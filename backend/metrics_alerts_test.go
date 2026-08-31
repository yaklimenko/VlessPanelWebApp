package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"vlesspanel/dto"
	"vlesspanel/model"
	"vlesspanel/xui"
)

// fakeSender — фейк tgSender: копит отправленные сообщения, может эмулировать
// ошибку отправки (реальный Telegram не дёргаем).
type fakeSender struct {
	msgs []string
	err  error
}

func (f *fakeSender) SendMessage(text string) error {
	if f.err != nil {
		return f.err
	}
	f.msgs = append(f.msgs, text)
	return nil
}

func (f *fakeSender) count(prefix string) int {
	n := 0
	for _, m := range f.msgs {
		if strings.HasPrefix(m, prefix) {
			n++
		}
	}
	return n
}

// newAlertFixture — AlertManager на временной БД с фейк-отправителем и
// управляемыми часами (a.now). Возвращает также указатель на часы.
func newAlertFixture(t *testing.T, cfg AlertConfig) (*AlertManager, *MetricsDB, *fakeSender, *time.Time) {
	t.Helper()
	dir := t.TempDir()
	db, err := NewMetricsDB(filepath.Join(dir, "metrics.db"), log.New(os.Stderr, "test: ", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	sender := &fakeSender{}
	a := NewAlertManager(db, sender, cfg, log.New(os.Stderr, "test: ", 0))
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	// Зеркало панелей: panel_snapshots ссылается на panels(id) по FK.
	if _, err := db.db.Exec(`INSERT INTO panels (id, name, url) VALUES ('one', 'One', 'https://one:1')`); err != nil {
		t.Fatal(err)
	}
	return a, db, sender, &now
}

func alertCfg() AlertConfig {
	return AlertConfig{
		Enabled:           true,
		RAMThresholdPct:   85,
		LoadCores:         1,
		LoadFactor:        1.0,
		TrafficMultiplier: 3,
		TrafficWindow:     24 * time.Hour,
		TrafficMinSamples: 6,
		StaleTesterAfter:  15 * time.Minute,
		Cooldown:          6 * time.Hour,
	}
}

func snapMem(mem float64) SnapshotRecord {
	return SnapshotRecord{PanelID: "one", TS: 0, MemMax: &mem}
}

func testPanel() model.Panel { return model.Panel{ID: "one", Name: "One"} }

// --- RAM ---

func TestAlertRAMFiresAndDedup(t *testing.T) {
	a, _, sender, now := newAlertFixture(t, alertCfg())

	a.CheckPanel(testPanel(), snapMem(92)) // mem_max 92% > 85% → алерт
	if sender.count("🚨") != 1 || sender.count("✅") != 0 {
		t.Fatalf("после первого срабатывания: 🚨=%d ✅=%d, want 1/0",
			sender.count("🚨"), sender.count("✅"))
	}

	// Тот же цикл/cooldown — повторный алерт не шлётся.
	a.CheckPanel(testPanel(), snapMem(94))
	if sender.count("🚨") != 1 {
		t.Fatalf("повторный алерт в пределах cooldown отправлен: %d, want 1", sender.count("🚨"))
	}

	// Cooldown прошёл — условие всё ещё болит → повторный алерт.
	*now = now.Add(7 * time.Hour)
	a.CheckPanel(testPanel(), snapMem(93))
	if sender.count("🚨") != 2 {
		t.Fatalf("после cooldown алертов %d, want 2", sender.count("🚨"))
	}

	// Возврат в норму → OK-сообщение (тоже раз в cooldown).
	a.CheckPanel(testPanel(), snapMem(50))
	if sender.count("✅") != 1 {
		t.Fatalf("OK-сообщений %d, want 1", sender.count("✅"))
	}
	// В норме повторно — OK не дублируется.
	a.CheckPanel(testPanel(), snapMem(51))
	if sender.count("✅") != 1 {
		t.Fatalf("OK-сообщений после повторного норм-снапшота %d, want 1", sender.count("✅"))
	}
}

func TestAlertRAMBelowThreshold(t *testing.T) {
	a, _, sender, _ := newAlertFixture(t, alertCfg())

	a.CheckPanel(testPanel(), snapMem(50))
	a.CheckPanel(testPanel(), snapMem(84.9))
	if len(sender.msgs) != 0 {
		t.Fatalf("при RAM ниже порога отправлено %d сообщений, want 0", len(sender.msgs))
	}
}

func TestAlertRAMNoMemData(t *testing.T) {
	a, _, sender, _ := newAlertFixture(t, alertCfg())
	a.CheckPanel(testPanel(), SnapshotRecord{PanelID: "one"}) // без mem
	if len(sender.msgs) != 0 {
		t.Fatalf("без данных по RAM отправлено %d сообщений, want 0", len(sender.msgs))
	}
}

// --- Load ---

func TestAlertLoad(t *testing.T) {
	a, _, sender, _ := newAlertFixture(t, alertCfg())

	// load1 2.5 > 1 ядро × 1.0 → алерт.
	rec := SnapshotRecord{PanelID: "one", Load1Avg: f64(2.5)}
	a.CheckPanel(testPanel(), rec)
	if sender.count("🚨") != 1 {
		t.Fatalf("load-алерт не отправлен: %d", sender.count("🚨"))
	}

	// Многоядерная панель: 2.5 < 4 ядра × 1.0 → алерта нет.
	cfg := alertCfg()
	cfg.LoadCores = 4
	a2, _, sender2, _ := newAlertFixture(t, cfg)
	a2.CheckPanel(testPanel(), rec)
	if len(sender2.msgs) != 0 {
		t.Fatalf("при load < ядер отправлено %d сообщений, want 0", len(sender2.msgs))
	}
}

// --- Трафик ---

// seedTraffic — базлайн net_up/net_down по окнам, начиная с tsBase, шаг 5 мин.
func seedTraffic(t *testing.T, db *MetricsDB, tsBase int64, n int, up, down int64) {
	t.Helper()
	for i := 0; i < n; i++ {
		u, d := int64Ptr(up), int64Ptr(down)
		if err := db.InsertSnapshot(SnapshotRecord{
			PanelID: "one", TS: tsBase + int64(i)*300, NetUp: u, NetDown: d,
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAlertTrafficSpike(t *testing.T) {
	a, db, sender, now := newAlertFixture(t, alertCfg())

	// Базлайн: 12 окон (1 час) по 1000/500 байт за окно.
	seedTraffic(t, db, now.Add(-time.Hour).Unix(), 12, 1000, 500)

	// Текущее окно: up ×40, down ×2 → алерт по up.
	rec := SnapshotRecord{PanelID: "one", TS: now.Unix(), NetUp: int64Ptr(40000), NetDown: int64Ptr(1000)}
	a.CheckPanel(testPanel(), rec)
	if sender.count("🚨") != 1 || !strings.Contains(sender.msgs[0], "аномальный трафик") {
		t.Fatalf("traffic-алерт не отправлен: %v", sender.msgs)
	}
	if !strings.Contains(sender.msgs[0], "↑") || strings.Contains(sender.msgs[0], "↓") {
		t.Errorf("сообщение должно быть про up: %s", sender.msgs[0])
	}

	// Возврат к нормальному трафику → OK.
	rec2 := SnapshotRecord{PanelID: "one", TS: now.Add(5 * time.Minute).Unix(), NetUp: int64Ptr(1500), NetDown: int64Ptr(500)}
	a.CheckPanel(testPanel(), rec2)
	if sender.count("✅") != 1 {
		t.Fatalf("OK по трафику не отправлен: %d", sender.count("✅"))
	}
}

func TestAlertTrafficLittleHistory(t *testing.T) {
	a, db, sender, now := newAlertFixture(t, alertCfg())

	// Мало истории (2 окна < minSamples=6) — алерт не шлём, даже при ×40.
	seedTraffic(t, db, now.Add(-10*time.Minute).Unix(), 2, 1000, 500)
	rec := SnapshotRecord{PanelID: "one", TS: now.Unix(), NetUp: int64Ptr(40000), NetDown: int64Ptr(500)}
	a.CheckPanel(testPanel(), rec)
	if len(sender.msgs) != 0 {
		t.Fatalf("при малой истории отправлено %d сообщений, want 0: %v", len(sender.msgs), sender.msgs)
	}
}

func TestAlertTrafficNoSpike(t *testing.T) {
	a, db, sender, now := newAlertFixture(t, alertCfg())

	seedTraffic(t, db, now.Add(-time.Hour).Unix(), 12, 1000, 500)
	rec := SnapshotRecord{PanelID: "one", TS: now.Unix(), NetUp: int64Ptr(2000), NetDown: int64Ptr(600)} // ×2, ×1.2 < 3
	a.CheckPanel(testPanel(), rec)
	if len(sender.msgs) != 0 {
		t.Fatalf("без спайка отправлено %d сообщений, want 0: %v", len(sender.msgs), sender.msgs)
	}
}

// --- Панель недоступна ---

func TestAlertPanelDownAndRecovery(t *testing.T) {
	a, db, sender, now := newAlertFixture(t, alertCfg())

	// Панель должна хоть раз отдать данные, иначе первый сбой не алертится.
	seedTraffic(t, db, now.Add(-5*time.Minute).Unix(), 1, 100, 100)

	a.CheckPanelDown(testPanel())
	if sender.count("🚨") != 1 || sender.count("✅") != 0 {
		t.Fatalf("panel_down: 🚨=%d ✅=%d, want 1/0", sender.count("🚨"), sender.count("✅"))
	}
	// Повторный сбой в cooldown — дедуп.
	a.CheckPanelDown(testPanel())
	if sender.count("🚨") != 1 {
		t.Fatalf("повторный panel_down в cooldown: %d, want 1", sender.count("🚨"))
	}

	// Панель ожила (свежий снапшот) → OK.
	a.CheckPanel(testPanel(), snapMem(50))
	if sender.count("✅") != 1 {
		t.Fatalf("OK по панели не отправлен: %d", sender.count("✅"))
	}

	// Снова упала — прошло много времени → новый алерт.
	*now = now.Add(7 * time.Hour)
	a.CheckPanelDown(testPanel())
	if sender.count("🚨") != 2 {
		t.Fatalf("panel_down после восстановления: %d, want 2", sender.count("🚨"))
	}
}

func TestAlertPanelDownNoHistory(t *testing.T) {
	a, _, sender, _ := newAlertFixture(t, alertCfg())

	// Новая панель без единого снапшота: сбой НЕ алертится (ложный позитив).
	a.CheckPanelDown(testPanel())
	if len(sender.msgs) != 0 {
		t.Fatalf("panel_down на новой панели отправлен: %v", sender.msgs)
	}
}

// --- Тестеры ---

func setTesterHeartbeat(t *testing.T, db *MetricsDB, name string, at time.Time) {
	t.Helper()
	testers, err := db.ListTesters()
	if err != nil {
		t.Fatal(err)
	}
	for _, ts := range testers {
		if ts.Name == name {
			if err := db.TouchTesterHeartbeat(ts.ID, at); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatalf("тестер %s не найден", name)
}

func setTesterEnabled(t *testing.T, db *MetricsDB, name string, enabled bool) {
	t.Helper()
	e := 0
	if enabled {
		e = 1
	}
	if _, err := db.db.Exec(`UPDATE testers SET enabled = ? WHERE name = ?`, e, name); err != nil {
		t.Fatal(err)
	}
}

func TestAlertTesterStale(t *testing.T) {
	a, db, sender, now := newAlertFixture(t, alertCfg())

	// Heartbeat 1 час назад (> 15 мин) → алерт.
	setTesterHeartbeat(t, db, "vlesssubtest-minicloud", now.Add(-time.Hour))
	a.CheckTesters()
	if sender.count("🚨") != 1 || !strings.Contains(sender.msgs[0], "Тестер") {
		t.Fatalf("tester-stale не отправлен: %v", sender.msgs)
	}

	// Свежий heartbeat → OK (возврат в норму).
	setTesterHeartbeat(t, db, "vlesssubtest-minicloud", *now)
	a.CheckTesters()
	if sender.count("✅") != 1 {
		t.Fatalf("OK по тестеру не отправлен: %d", sender.count("✅"))
	}

	// Снова свежий — тишина.
	a.CheckTesters()
	if len(sender.msgs) != 2 {
		t.Fatalf("лишние сообщения по живому тестеру: %v", sender.msgs)
	}
}

func TestAlertTesterStaleDisabledAndNull(t *testing.T) {
	a, db, sender, now := newAlertFixture(t, alertCfg())

	// Отключённый тестер с протухшим heartbeat — молчим.
	setTesterHeartbeat(t, db, "vlesssubtest-minicloud", now.Add(-time.Hour))
	setTesterEnabled(t, db, "vlesssubtest-minicloud", false)
	a.CheckTesters()
	if len(sender.msgs) != 0 {
		t.Fatalf("отключённый тестер алертится: %v", sender.msgs)
	}

	// Включённый, но ни разу не контактировавший (heartbeat NULL) — молчим.
	setTesterEnabled(t, db, "vlesssubtest-minicloud", true)
	if _, err := db.db.Exec(`UPDATE testers SET last_heartbeat_at = NULL WHERE name = 'vlesssubtest-minicloud'`); err != nil {
		t.Fatal(err)
	}
	a.CheckTesters()
	if len(sender.msgs) != 0 {
		t.Fatalf("тестер без heartbeat алертится: %v", sender.msgs)
	}
}

// --- Ошибка отправки не роняет менеджер/коллектор ---

func TestAlertSendErrorDoesNotPanic(t *testing.T) {
	a, _, sender, now := newAlertFixture(t, alertCfg())
	sender.err = errors.New("network down")

	// Ошибка отправки — не паникуем, сообщение не «засчитано» (LastFiredAt не двигается).
	a.CheckPanel(testPanel(), snapMem(95))
	if len(sender.msgs) != 0 {
		t.Fatalf("при ошибке отправки сообщение записано: %v", sender.msgs)
	}

	// Канал починился — следующий цикл (после cooldown) алерт уходит.
	*now = now.Add(7 * time.Hour)
	sender.err = nil
	a.CheckPanel(testPanel(), snapMem(95))
	if sender.count("🚨") != 1 {
		t.Fatalf("алерт после восстановления отправки: %d, want 1", sender.count("🚨"))
	}
}

func TestAlertDisabled(t *testing.T) {
	cfg := alertCfg()
	cfg.Enabled = false
	a, db, sender, _ := newAlertFixture(t, cfg)

	seedTraffic(t, db, time.Now().Add(-time.Hour).Unix(), 12, 1000, 500)
	a.CheckPanel(testPanel(), snapMem(99))
	a.CheckPanelDown(testPanel())
	if len(sender.msgs) != 0 {
		t.Fatalf("при выключенных алертах отправлено: %v", sender.msgs)
	}
}

// --- Интеграция: коллектор дёргает алерты после записи снапшота ---

func TestCollectorAlertsIntegration(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC)
	base := time.Date(2026, 8, 31, 12, 5, 0, 0, time.UTC).Unix()

	// mem в истории 92/94 → mem_max 94 > 85 → алерт после цикла сбора.
	h := fullHistory(base)
	h["mem"] = []xui.HistoryPoint{{T: base + 0, V: 92}, {T: base + 60, V: 94}}
	tel := &fakeTelemetry{status: testStatus(), history: h}

	c, db, _ := newCollectorFixture(t, tel, nil, &fakeDaemon{})
	sender := &fakeSender{}
	cfg := alertCfg()
	cfg.LoadCores = 8 // load1 в fullHistory (1.5) ниже порога — проверяем только RAM
	a := NewAlertManager(db, sender, cfg, log.New(os.Stderr, "test: ", 0))
	c.alerts = a

	c.collectTelemetry(now)

	if sender.count("🚨") != 1 || !strings.Contains(sender.msgs[0], "RAM") {
		t.Fatalf("коллектор не отправил RAM-алерт: %v", sender.msgs)
	}
	// Трафик в тесте маленький и истории нет → спайк не оцениваем, panel_down не болел.
	if sender.count("✅") != 0 {
		t.Fatalf("неожиданные OK: %v", sender.msgs)
	}
}

// Панель недоступна → коллектор зовёт CheckPanelDown (снапшотов нет — но ранее
// были данные, поэтому алерт уходит).
func TestCollectorAlertsPanelDown(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC)
	base := time.Date(2026, 8, 31, 12, 5, 0, 0, time.UTC).Unix() // точки 12:05/12:06 > lastSeen

	// Сначала панель работает (пишем историю), потом падает.
	dir := t.TempDir()
	storage := NewStorage(filepath.Join(dir, "panels.json"), filepath.Join(dir, "agg"), dir)
	if _, err := storage.AddPanel(dto.CreatePanelRequest{Name: "One", URL: "https://one:1", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	db, err := NewMetricsDB(filepath.Join(dir, "metrics.db"), log.New(os.Stderr, "test: ", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	tel := &fakeTelemetry{status: testStatus(), history: fullHistory(base)}
	c := NewMetricsCollector(db, storage, &fakePanelClient{}, tel, &fakeDaemon{}, "http://vlesssubtest:7070",
		log.New(os.Stderr, "test: ", 0))
	sender := &fakeSender{}
	cfg := alertCfg()
	cfg.LoadCores = 8 // load1 в fullHistory ниже порога — не мешаем panel_down
	a := NewAlertManager(db, sender, cfg, log.New(os.Stderr, "test: ", 0))
	c.alerts = a

	c.collectTelemetry(now) // панель жива — снапшот записан
	if sender.count("🚨") != 0 {
		t.Fatalf("на живой панели алерты: %v", sender.msgs)
	}

	tel.err = errors.New("timeout")
	c.collectTelemetry(now.Add(5 * time.Minute)) // панель упала
	if sender.count("🚨") != 1 {
		t.Fatalf("panel_down от коллектора не отправлен: %v", sender.msgs)
	}
}

// --- Вспомогательные ---

func f64(v float64) *float64 { return &v }
