package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"vlesspanel/model"

	_ "modernc.org/sqlite" // pure-Go SQLite (Docker собирается с CGO_ENABLED=0)
)

// MetricsDB — SQLite-хранилище раздела статистики (/data/metrics.db).
// Схема — ровно по заметке «Раздел статистики — БД.md» (таблицы и поля ТОЧНО).
//
// Обязательства:
//   - миграции — CREATE TABLE/INDEX IF NOT EXISTS при старте (простота);
//   - bootstrap — при пустой таблице testers вставляется демон minicloud;
//   - retention — чистка строк старше 3 месяцев по ts (при старте + раз в сутки);
//   - ts хранится как unix (UTC); прогоны — RFC3339 UTC фиксированной ширины
//     (лексографический порядок == хронологический, для чистки и сортировки).
type MetricsDB struct {
	db   *sql.DB
	path string
	log  *log.Logger
}

// runTimeFormat — формат хранения started_at/finished_at в test_runs и
// tested_at в test_key_results. Фиксированная ширина наносекунд (как ключи
// bbolt у демона), чтобы строковое сравнение совпадало с хронологией.
const runTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

// retentionMonths — глубина хранения телеметрии и прогонов.
const retentionMonths = 3

// NewMetricsDB открывает (создаёт) БД, накатывает схему и делает bootstrap.
func NewMetricsDB(path string, logger *log.Logger) (*MetricsDB, error) {
	if path == "" {
		path = filepath.Join(".", "metrics.db")
	}
	if logger == nil {
		logger = log.Default()
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("metrics db dir: %w", err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening metrics db: %w", err)
	}
	// Один коннект + WAL: коллектор пишет, API читает — без SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics db WAL: %w", err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics db busy_timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("metrics db foreign_keys: %w", err)
	}

	m := &MetricsDB{db: db, path: path, log: logger}
	if err := m.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	if err := m.bootstrap(); err != nil {
		db.Close()
		return nil, err
	}
	return m, nil
}

// Close закрывает БД.
func (m *MetricsDB) Close() error { return m.db.Close() }

// migrate создаёт схему. Идемпотентно (IF NOT EXISTS).
func (m *MetricsDB) migrate() error {
	stmts := []string{
		// 📋 Реестр тестировщиков (демонов VlessSubTest)
		`CREATE TABLE IF NOT EXISTS testers (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			name             TEXT NOT NULL UNIQUE,
			base_url         TEXT NOT NULL,
			location         TEXT,
			enabled          INTEGER NOT NULL DEFAULT 1,
			weight           INTEGER NOT NULL DEFAULT 1,
			priority         INTEGER NOT NULL DEFAULT 0,
			last_heartbeat_at TEXT,
			created_at       TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at       TEXT
		)`,

		// 🧪 Прогоны тестов подписок (история)
		`CREATE TABLE IF NOT EXISTS test_runs (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			tester_id       INTEGER NOT NULL REFERENCES testers(id),
			subscription_id TEXT NOT NULL,
			status          TEXT NOT NULL DEFAULT 'running',
			total           INTEGER NOT NULL DEFAULT 0,
			ok_count        INTEGER NOT NULL DEFAULT 0,
			fail_count      INTEGER NOT NULL DEFAULT 0,
			error           TEXT,
			started_at      TEXT NOT NULL DEFAULT (datetime('now')),
			finished_at     TEXT
		)`,

		// 🔑 Результаты по отдельным ключам внутри прогона
		`CREATE TABLE IF NOT EXISTS test_key_results (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id      INTEGER NOT NULL REFERENCES test_runs(id) ON DELETE CASCADE,
			key_id      TEXT,
			label       TEXT,
			status      TEXT NOT NULL,
			ip          TEXT,
			youtube     TEXT,
			instagram   TEXT,
			latency_ms  INTEGER,
			tested_at   TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Панели (зеркало panels.json, чтобы metrics.db была самодостаточной)
		`CREATE TABLE IF NOT EXISTS panels (
			id       TEXT PRIMARY KEY,
			name     TEXT NOT NULL,
			url      TEXT NOT NULL,
			added_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,

		// Системные метрики панели: одно окно (5 мин) = одна строка, avg+max
		`CREATE TABLE IF NOT EXISTS panel_snapshots (
			id             INTEGER PRIMARY KEY AUTOINCREMENT,
			panel_id       TEXT NOT NULL REFERENCES panels(id),
			ts             INTEGER NOT NULL,
			cpu_avg        REAL, cpu_max REAL,
			mem_avg        REAL, mem_max REAL,
			swap_avg       REAL,
			load1_avg      REAL, load5_avg REAL, load15_avg REAL,
			net_up         INTEGER,
			net_down       INTEGER,
			net_traffic_sent INTEGER,
			net_traffic_recv INTEGER,
			disk_used      INTEGER, disk_total INTEGER,
			online_avg     INTEGER, online_max INTEGER,
			open_conns_max INTEGER,
			xray_ok        INTEGER NOT NULL DEFAULT 1,
			UNIQUE(panel_id, ts)
		)`,

		// Трафик по инбаундам: срезы счётчиков (дельты считаем на чтении)
		`CREATE TABLE IF NOT EXISTS inbound_traffic (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			panel_id   TEXT NOT NULL REFERENCES panels(id),
			inbound_id INTEGER NOT NULL,
			remark     TEXT,
			ts         INTEGER NOT NULL,
			up         INTEGER NOT NULL,
			down       INTEGER NOT NULL,
			total      INTEGER NOT NULL,
			UNIQUE(panel_id, inbound_id, ts)
		)`,

		// Трафик по клиентам: ТОЛЬКО изменения (delta-записи), пустые клиенты не мусорят
		`CREATE TABLE IF NOT EXISTS client_traffic (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			panel_id   TEXT NOT NULL REFERENCES panels(id),
			inbound_id INTEGER NOT NULL,
			email      TEXT NOT NULL,
			ts         INTEGER NOT NULL,
			up_delta   INTEGER NOT NULL,
			down_delta INTEGER NOT NULL,
			UNIQUE(panel_id, inbound_id, email, ts)
		)`,

		// Индексы
		`CREATE INDEX IF NOT EXISTS idx_runs_sub    ON test_runs(subscription_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_runs_tester ON test_runs(tester_id, started_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_keys_run    ON test_key_results(run_id)`,
		`CREATE INDEX IF NOT EXISTS idx_snap        ON panel_snapshots(panel_id, ts DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_inb_traffic ON inbound_traffic(panel_id, ts)`,
		`CREATE INDEX IF NOT EXISTS idx_cli_traffic ON client_traffic(panel_id, ts)`,
	}

	for _, s := range stmts {
		if _, err := m.db.Exec(s); err != nil {
			return fmt.Errorf("metrics db migrate: %w\nstmt: %s", err, s)
		}
	}
	return nil
}

// bootstrap — при первом запуске (таблица testers пуста) вставляет текущего
// демона minicloud. Никакого env-сидинга: реестр тестеров живёт в БД.
func (m *MetricsDB) bootstrap() error {
	_, err := m.db.Exec(`INSERT INTO testers (name, base_url, location, enabled, priority)
		SELECT 'vlesssubtest-minicloud', 'http://vlesssubtest:7070', 'minicloud', 1, 0
		WHERE NOT EXISTS (SELECT 1 FROM testers)`)
	if err != nil {
		return fmt.Errorf("metrics db bootstrap: %w", err)
	}
	return nil
}

// --- Панели (зеркало panels.json) ---

// SyncPanels зеркалит panels.json в таблицу panels (upsert по id).
// Удалённые из panels.json панели сохраняются — их история не должна исчезать.
func (m *MetricsDB) SyncPanels(panels []model.Panel) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, p := range panels {
		if _, err := tx.Exec(`INSERT INTO panels (id, name, url) VALUES (?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, url = excluded.url`,
			p.ID, p.Name, p.URL); err != nil {
			return fmt.Errorf("sync panel %s: %w", p.ID, err)
		}
	}
	return tx.Commit()
}

// ListPanels возвращает зеркало панелей из БД (id, name, url).
func (m *MetricsDB) ListPanels() ([]model.Panel, error) {
	rows, err := m.db.Query(`SELECT id, name, url FROM panels ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var panels []model.Panel
	for rows.Next() {
		var p model.Panel
		if err := rows.Scan(&p.ID, &p.Name, &p.URL); err != nil {
			return nil, err
		}
		panels = append(panels, p)
	}
	return panels, rows.Err()
}

// --- Тестеры ---

// Tester — запись реестра тестировщиков.
type Tester struct {
	ID              int64
	Name            string
	BaseURL         string
	Location        string
	Enabled         int
	Weight          int
	Priority        int
	LastHeartbeatAt *string
	CreatedAt       string
	UpdatedAt       *string
}

// ListTesters возвращает всех тестеров (по priority, затем name).
func (m *MetricsDB) ListTesters() ([]Tester, error) {
	rows, err := m.db.Query(`SELECT id, name, base_url, location, enabled, weight, priority,
		last_heartbeat_at, created_at, updated_at FROM testers ORDER BY priority, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Tester
	for rows.Next() {
		var t Tester
		if err := rows.Scan(&t.ID, &t.Name, &t.BaseURL, &t.Location, &t.Enabled,
			&t.Weight, &t.Priority, &t.LastHeartbeatAt, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TouchTesterHeartbeat обновляет last_heartbeat_at при успешном контакте.
func (m *MetricsDB) TouchTesterHeartbeat(id int64, at time.Time) error {
	_, err := m.db.Exec(`UPDATE testers SET last_heartbeat_at = ?, updated_at = ? WHERE id = ?`,
		at.UTC().Format(runTimeFormat), nowStr(), id)
	return err
}

// --- Снапшоты панелей ---

// SnapshotRecord — одна строка panel_snapshots (окно 5 минут).
type SnapshotRecord struct {
	PanelID string
	TS      int64
	CPUAvg  *float64
	CPUMax  *float64
	MemAvg  *float64
	MemMax  *float64
	SwapAvg *float64

	Load1Avg  *float64
	Load5Avg  *float64
	Load15Avg *float64

	NetUp          *int64
	NetDown        *int64
	NetTrafficSent *int64
	NetTrafficRecv *int64
	DiskUsed       *int64
	DiskTotal      *int64
	OnlineAvg      *int
	OnlineMax      *int
	OpenConnsMax   *int
	XrayOK         int
}

// InsertSnapshot пишет строку снапшота (INSERT OR REPLACE по UNIQUE(panel_id, ts)).
func (m *MetricsDB) InsertSnapshot(s SnapshotRecord) error {
	_, err := m.db.Exec(`INSERT OR REPLACE INTO panel_snapshots (
		panel_id, ts, cpu_avg, cpu_max, mem_avg, mem_max, swap_avg,
		load1_avg, load5_avg, load15_avg,
		net_up, net_down, net_traffic_sent, net_traffic_recv,
		disk_used, disk_total, online_avg, online_max, open_conns_max, xray_ok)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		s.PanelID, s.TS,
		s.CPUAvg, s.CPUMax, s.MemAvg, s.MemMax, s.SwapAvg,
		s.Load1Avg, s.Load5Avg, s.Load15Avg,
		s.NetUp, s.NetDown, s.NetTrafficSent, s.NetTrafficRecv,
		s.DiskUsed, s.DiskTotal, s.OnlineAvg, s.OnlineMax, s.OpenConnsMax, s.XrayOK)
	if err != nil {
		return fmt.Errorf("insert snapshot %s@%d: %w", s.PanelID, s.TS, err)
	}
	return nil
}

// MaxSnapshotTS возвращает максимальный ts снапшота панели (для инкрементального
// забора после рестарта). ok=false — данных ещё нет.
func (m *MetricsDB) MaxSnapshotTS(panelID string) (int64, bool) {
	var ts int64
	err := m.db.QueryRow(`SELECT MAX(ts) FROM panel_snapshots WHERE panel_id = ?`, panelID).Scan(&ts)
	if err != nil || ts == 0 {
		return 0, false
	}
	return ts, true
}

// SnapshotRow — строка panel_snapshots при чтении (сырые, 5-минутные).
type SnapshotRow struct {
	PanelID string
	TS      int64
	CPUAvg  *float64
	CPUMax  *float64
	MemAvg  *float64
	MemMax  *float64
	SwapAvg *float64

	Load1Avg  *float64
	Load5Avg  *float64
	Load15Avg *float64

	NetUp          *int64
	NetDown        *int64
	NetTrafficSent *int64
	NetTrafficRecv *int64
	DiskUsed       *int64
	DiskTotal      *int64
	OnlineAvg      *int
	OnlineMax      *int
	OpenConnsMax   *int
	XrayOK         int
}

// Snapshots читает сырые снапшоты панели за [from, to] (включительно),
// упорядоченные по ts.
func (m *MetricsDB) Snapshots(panelID string, from, to int64) ([]SnapshotRow, error) {
	rows, err := m.db.Query(`SELECT panel_id, ts,
		cpu_avg, cpu_max, mem_avg, mem_max, swap_avg,
		load1_avg, load5_avg, load15_avg,
		net_up, net_down, net_traffic_sent, net_traffic_recv,
		disk_used, disk_total, online_avg, online_max, open_conns_max, xray_ok
		FROM panel_snapshots WHERE panel_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts`, panelID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SnapshotRow
	for rows.Next() {
		var s SnapshotRow
		if err := rows.Scan(&s.PanelID, &s.TS,
			&s.CPUAvg, &s.CPUMax, &s.MemAvg, &s.MemMax, &s.SwapAvg,
			&s.Load1Avg, &s.Load5Avg, &s.Load15Avg,
			&s.NetUp, &s.NetDown, &s.NetTrafficSent, &s.NetTrafficRecv,
			&s.DiskUsed, &s.DiskTotal, &s.OnlineAvg, &s.OnlineMax, &s.OpenConnsMax, &s.XrayOK); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Availability — последний ts по каждой панели (для сигнала «панель не отдаёт
// телеметрию»: свежих снапшотов нет → есть проблема).
type Availability struct {
	PanelID        string
	Name           string
	LastSnapshotTS int64
}

// PanelAvailability возвращает последний ts снапшота по каждой панели.
func (m *MetricsDB) PanelAvailability() ([]Availability, error) {
	rows, err := m.db.Query(`SELECT p.id, p.name, COALESCE(MAX(s.ts), 0)
		FROM panels p LEFT JOIN panel_snapshots s ON s.panel_id = p.id
		GROUP BY p.id ORDER BY p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Availability
	for rows.Next() {
		var a Availability
		if err := rows.Scan(&a.PanelID, &a.Name, &a.LastSnapshotTS); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// --- Трафик инбаундов и клиентов ---

// InboundTrafficRecord — срез счётчиков инбаунда.
type InboundTrafficRecord struct {
	PanelID   string
	InboundID int
	Remark    string
	TS        int64
	Up        int64
	Down      int64
	Total     int64
}

// InsertInboundTraffic пишет срез счётчиков инбаунда (INSERT OR REPLACE).
func (m *MetricsDB) InsertInboundTraffic(r InboundTrafficRecord) error {
	_, err := m.db.Exec(`INSERT OR REPLACE INTO inbound_traffic
		(panel_id, inbound_id, remark, ts, up, down, total)
		VALUES (?,?,?,?,?,?,?)`,
		r.PanelID, r.InboundID, r.Remark, r.TS, r.Up, r.Down, r.Total)
	if err != nil {
		return fmt.Errorf("insert inbound traffic %s/%d@%d: %w", r.PanelID, r.InboundID, r.TS, err)
	}
	return nil
}

// InboundTrafficRows читает срезы инбаундов панели за [from, to],
// упорядоченные по (inbound_id, ts) — удобно для дельт на чтении.
func (m *MetricsDB) InboundTrafficRows(panelID string, from, to int64) ([]InboundTrafficRecord, error) {
	rows, err := m.db.Query(`SELECT panel_id, inbound_id, remark, ts, up, down, total
		FROM inbound_traffic WHERE panel_id = ? AND ts >= ? AND ts <= ?
		ORDER BY inbound_id, ts`, panelID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []InboundTrafficRecord
	for rows.Next() {
		var r InboundTrafficRecord
		if err := rows.Scan(&r.PanelID, &r.InboundID, &r.Remark, &r.TS, &r.Up, &r.Down, &r.Total); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ClientTrafficRecord — delta-запись по клиенту.
type ClientTrafficRecord struct {
	PanelID   string
	InboundID int
	Email     string
	TS        int64
	UpDelta   int64
	DownDelta int64
}

// InsertClientTraffic пишет delta-запись клиента (INSERT OR REPLACE).
func (m *MetricsDB) InsertClientTraffic(r ClientTrafficRecord) error {
	_, err := m.db.Exec(`INSERT OR REPLACE INTO client_traffic
		(panel_id, inbound_id, email, ts, up_delta, down_delta)
		VALUES (?,?,?,?,?,?)`,
		r.PanelID, r.InboundID, r.Email, r.TS, r.UpDelta, r.DownDelta)
	if err != nil {
		return fmt.Errorf("insert client traffic %s/%d/%s@%d: %w", r.PanelID, r.InboundID, r.Email, r.TS, err)
	}
	return nil
}

// ClientTrafficRows читает delta-записи клиентов панели за [from, to].
func (m *MetricsDB) ClientTrafficRows(panelID string, from, to int64) ([]ClientTrafficRecord, error) {
	rows, err := m.db.Query(`SELECT panel_id, inbound_id, email, ts, up_delta, down_delta
		FROM client_traffic WHERE panel_id = ? AND ts >= ? AND ts <= ?
		ORDER BY ts`, panelID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ClientTrafficRecord
	for rows.Next() {
		var r ClientTrafficRecord
		if err := rows.Scan(&r.PanelID, &r.InboundID, &r.Email, &r.TS, &r.UpDelta, &r.DownDelta); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- Прогоны тестов ---

// TestRun — строка test_runs.
type TestRun struct {
	ID             int64
	TesterID       int64
	SubscriptionID string
	Status         string
	Total          int
	OKCount        int
	FailCount      int
	Error          *string
	StartedAt      string
	FinishedAt     string
}

// TestKeyResult — строка test_key_results.
type TestKeyResult struct {
	ID        int64
	RunID     int64
	KeyID     *string
	Label     string
	Status    string
	IP        string
	YouTube   string
	Instagram string
	LatencyMs *int
	TestedAt  string
}

// InsertTestRun пишет прогон + per-key результаты в одной транзакции.
// Дедупликация: (tester_id, subscription_id, started_at) уже есть → ничего
// не пишем, возвращаем (existingID, false). Это делает повторный забор
// после рестарта идемпотентным.
func (m *MetricsDB) InsertTestRun(run *TestRun, keys []TestKeyResult) (int64, bool, error) {
	tx, err := m.db.Begin()
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var existing int64
	err = tx.QueryRow(`SELECT id FROM test_runs
		WHERE tester_id = ? AND subscription_id = ? AND started_at = ?`,
		run.TesterID, run.SubscriptionID, run.StartedAt).Scan(&existing)
	if err == nil {
		return existing, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}

	res, err := tx.Exec(`INSERT INTO test_runs
		(tester_id, subscription_id, status, total, ok_count, fail_count, error, started_at, finished_at)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		run.TesterID, run.SubscriptionID, run.Status, run.Total, run.OKCount, run.FailCount,
		run.Error, run.StartedAt, run.FinishedAt)
	if err != nil {
		return 0, false, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}

	for _, k := range keys {
		if _, err := tx.Exec(`INSERT INTO test_key_results
			(run_id, key_id, label, status, ip, youtube, instagram, latency_ms, tested_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			runID, k.KeyID, k.Label, k.Status, k.IP, k.YouTube, k.Instagram, k.LatencyMs, k.TestedAt); err != nil {
			return 0, false, err
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return runID, true, nil
}

// TestRuns — список прогонов за [from, to] (по started_at), новые сверху.
// Если testerID != "" — только этого тестера; если subscriptionID != "" — только
// этой подписки.
func (m *MetricsDB) TestRuns(from, to time.Time, testerID string, subscriptionID string) ([]TestRun, error) {
	q := `SELECT id, tester_id, subscription_id, status, total, ok_count, fail_count,
		error, started_at, finished_at FROM test_runs WHERE started_at >= ? AND started_at <= ?`
	args := []interface{}{from.UTC().Format(runTimeFormat), to.UTC().Format(runTimeFormat)}
	if testerID != "" {
		q += ` AND tester_id = ?`
		args = append(args, testerID)
	}
	if subscriptionID != "" {
		q += ` AND subscription_id = ?`
		args = append(args, subscriptionID)
	}
	q += ` ORDER BY started_at DESC`

	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestRun
	for rows.Next() {
		var r TestRun
		if err := rows.Scan(&r.ID, &r.TesterID, &r.SubscriptionID, &r.Status, &r.Total,
			&r.OKCount, &r.FailCount, &r.Error, &r.StartedAt, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// TestRunByID возвращает прогон по id (ok=false — не найден).
func (m *MetricsDB) TestRunByID(id int64) (*TestRun, bool, error) {
	var r TestRun
	err := m.db.QueryRow(`SELECT id, tester_id, subscription_id, status, total, ok_count,
		fail_count, error, started_at, finished_at FROM test_runs WHERE id = ?`, id).
		Scan(&r.ID, &r.TesterID, &r.SubscriptionID, &r.Status, &r.Total,
			&r.OKCount, &r.FailCount, &r.Error, &r.StartedAt, &r.FinishedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return &r, true, nil
}

// TestKeyResults возвращает per-key результаты прогона.
func (m *MetricsDB) TestKeyResults(runID int64) ([]TestKeyResult, error) {
	rows, err := m.db.Query(`SELECT id, run_id, key_id, label, status, ip, youtube,
		instagram, latency_ms, tested_at FROM test_key_results WHERE run_id = ? ORDER BY id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TestKeyResult
	for rows.Next() {
		var k TestKeyResult
		if err := rows.Scan(&k.ID, &k.RunID, &k.KeyID, &k.Label, &k.Status, &k.IP,
			&k.YouTube, &k.Instagram, &k.LatencyMs, &k.TestedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// --- Retention ---

// CleanupRetention удаляет строки старше retentionMonths (по ts / started_at).
// Телеметрия — по unix ts, прогоны — по RFC3339-строке (лексографически
// корректно, т.к. формат фиксированной ширины). Вызывать при старте и раз в сутки.
func (m *MetricsDB) CleanupRetention(now time.Time) error {
	cutoff := now.AddDate(0, -retentionMonths, 0)
	cutoffUnix := cutoff.Unix()
	cutoffStr := cutoff.UTC().Format(runTimeFormat)

	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, s := range []struct {
		stmt string
		arg  interface{}
	}{
		{`DELETE FROM panel_snapshots WHERE ts < ?`, cutoffUnix},
		{`DELETE FROM inbound_traffic WHERE ts < ?`, cutoffUnix},
		{`DELETE FROM client_traffic WHERE ts < ?`, cutoffUnix},
		{`DELETE FROM test_key_results WHERE run_id IN (SELECT id FROM test_runs WHERE started_at < ?)`, cutoffStr},
		{`DELETE FROM test_runs WHERE started_at < ?`, cutoffStr},
	} {
		if _, err := tx.Exec(s.stmt, s.arg); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	m.log.Printf("metrics: retention cleanup done (cutoff %s)", cutoff.UTC().Format(time.RFC3339))
	return nil
}
