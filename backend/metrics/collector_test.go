package metrics

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vlesspanel/dto"
	"vlesspanel/model"
	"vlesspanel/xui"
)

// fakeTelemetry — фейк TelemetryClient: программируемый status и history.
type fakeTelemetry struct {
	status  *xui.ServerStatus
	history map[string][]xui.HistoryPoint // metric → точки
	err     error
}

func (f *fakeTelemetry) ServerStatus(model.Panel) (*xui.ServerStatus, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.status, nil
}

func (f *fakeTelemetry) ServerHistory(_ model.Panel, metric string, _ int) ([]xui.HistoryPoint, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.history[metric], nil
}

// fakePanelStore — фейк PanelStore: панели и подписки без файлового хранилища.
type fakePanelStore struct {
	panels []model.Panel
	subs   []model.Subscription
}

func (f *fakePanelStore) LoadPanels() ([]model.Panel, error)               { return f.panels, nil }
func (f *fakePanelStore) ListSubscriptions() ([]model.Subscription, error) { return f.subs, nil }

// fakePanelClient — фейк PanelAPI: только ListInbounds (нужен коллектору).
type fakePanelClient struct {
	PanelAPI
	inbounds []xui.XUIInbound
	err      error
}

func (f *fakePanelClient) ListInbounds(model.Panel) ([]xui.XUIInbound, error) {
	return f.inbounds, f.err
}

// fakeDaemon — фейк DaemonClient: Status + ListRuns (остальные методы не нужны).
type fakeDaemon struct {
	DaemonClient
	status dto.VlessSubTestStatus
	runs   []dto.DaemonRun
	err    error
}

func (f *fakeDaemon) Status() dto.VlessSubTestStatus { return f.status }

func (f *fakeDaemon) ListRuns(time.Time, time.Time) ([]dto.DaemonRun, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.runs, nil
}

func testStatus() *xui.ServerStatus {
	return &xui.ServerStatus{
		CPU:      10,
		Mem:      xui.MemStats{Current: 2 << 30, Total: 8 << 30},
		Swap:     xui.MemStats{Current: 1 << 30, Total: 4 << 30},
		Disk:     xui.MemStats{Current: 50 << 30, Total: 100 << 30},
		NetIO:    xui.NetIOStats{Up: 100000, Down: 200000},
		Xray:     xui.XrayState{State: "running"},
		TCPCount: 42,
		Load:     xui.LoadStats{Load1: 0.5, Load5: 0.4, Load15: 0.3},
	}
}

// newCollectorFixture — фейк PanelStore с панелью p1 (id "one") + коллектор на фейках.
func newCollectorFixture(t *testing.T, tel *fakeTelemetry, inbounds []xui.XUIInbound, daemon *fakeDaemon) (*MetricsCollector, *MetricsDB, *fakePanelStore) {
	t.Helper()
	dir := t.TempDir()
	storage := &fakePanelStore{panels: []model.Panel{{ID: "one", Name: "One", URL: "https://one:1", Token: "t"}}}

	db, err := NewMetricsDB(filepath.Join(dir, "metrics.db"), log.New(os.Stderr, "test: ", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	panelClient := &fakePanelClient{inbounds: inbounds}
	c := NewMetricsCollector(db, storage, panelClient, tel, daemon, "http://vlesssubtest:7070",
		log.New(os.Stderr, "test: ", 0))
	return c, db, storage
}

func fullHistory(base int64) map[string][]xui.HistoryPoint {
	return map[string][]xui.HistoryPoint{
		"cpu":     {{T: base + 0, V: 10}, {T: base + 60, V: 20}},
		"mem":     {{T: base + 0, V: 50}, {T: base + 60, V: 60}},
		"netUp":   {{T: base + 0, V: 1000}, {T: base + 60, V: 2000}},
		"netDown": {{T: base + 0, V: 500}, {T: base + 60, V: 500}},
		"online":  {{T: base + 0, V: 3}, {T: base + 60, V: 5}},
		"load1":   {{T: base + 0, V: 1.0}, {T: base + 60, V: 2.0}},
		"load5":   {{T: base + 0, V: 0.5}, {T: base + 60, V: 0.5}},
		"load15":  {{T: base + 0, V: 0.25}, {T: base + 60, V: 0.25}},
	}
}

// Первый цикл: из history считаем avg/max САМИ, поля без history — из status.
func TestCollectTelemetrySnapshot(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC) // окно 12:05
	base := time.Date(2026, 8, 31, 12, 5, 0, 0, time.UTC).Unix()

	tel := &fakeTelemetry{status: testStatus(), history: fullHistory(base)}
	c, db, _ := newCollectorFixture(t, tel, nil, &fakeDaemon{})

	c.collectTelemetry(now)

	rows, err := db.Snapshots("one", 0, now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("снапшотов %d, want 1", len(rows))
	}
	s := rows[0]
	if s.TS != base {
		t.Errorf("ts = %d, want %d (начало окна)", s.TS, base)
	}
	if mustFloat(s.CPUAvg) != 15 || mustFloat(s.CPUMax) != 20 {
		t.Errorf("cpu avg/max = %v/%v, want 15/20", s.CPUAvg, s.CPUMax)
	}
	if mustFloat(s.MemAvg) != 55 || mustFloat(s.MemMax) != 60 {
		t.Errorf("mem avg/max = %v/%v, want 55/60", s.MemAvg, s.MemMax)
	}
	if mustInt64(s.NetUp) != 3000*historyBucket || mustInt64(s.NetDown) != 1000*historyBucket {
		t.Errorf("net up/down = %d/%d, want %d/%d", mustInt64(s.NetUp), mustInt64(s.NetDown), 3000*historyBucket, 1000*historyBucket)
	}
	if mustInt(s.OnlineAvg) != 4 || mustInt(s.OnlineMax) != 5 {
		t.Errorf("online avg/max = %d/%d, want 4/5", mustInt(s.OnlineAvg), mustInt(s.OnlineMax))
	}
	// Из status: swap %, диск, кумулятивные счётчики, open_conns, xray.
	if mustFloat(s.SwapAvg) != 25 {
		t.Errorf("swap = %v, want 25%%", s.SwapAvg)
	}
	if mustInt64(s.NetTrafficSent) != 100000 || mustInt64(s.NetTrafficRecv) != 200000 {
		t.Errorf("net_traffic = %d/%d, want 100000/200000", mustInt64(s.NetTrafficSent), mustInt64(s.NetTrafficRecv))
	}
	if mustInt(s.OpenConnsMax) != 42 || s.XrayOK != 1 {
		t.Errorf("open_conns/xray = %d/%d, want 42/1", mustInt(s.OpenConnsMax), s.XrayOK)
	}
}

// Инкрементальный забор: второй цикл берёт только новые точки, строки не дублируются.
// Бакет определяется по ts точки, поэтому хвост 12:07/12:08 попадает в уже записанное
// окно 12:05 (строка перезаписывается INSERT OR REPLACE более полной агрегацией),
// а 12:10/12:11 — в новое окно 12:10.
func TestCollectTelemetryIncremental(t *testing.T) {
	now1 := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC)
	base1 := time.Date(2026, 8, 31, 12, 5, 0, 0, time.UTC).Unix()

	tel := &fakeTelemetry{status: testStatus(), history: fullHistory(base1)}
	c, db, _ := newCollectorFixture(t, tel, nil, &fakeDaemon{})

	c.collectTelemetry(now1)

	// Второй цикл: панель отдаёт полную историю (как настоящая). Новые точки:
	// 12:07/12:08 (cpu 100) → окно 12:05 (перезапись), 12:10/12:11 (cpu 200) → окно 12:10.
	base2 := time.Date(2026, 8, 31, 12, 10, 0, 0, time.UTC).Unix()
	h2 := map[string][]xui.HistoryPoint{}
	for m, pts := range fullHistory(base1) {
		h2[m] = append(append([]xui.HistoryPoint{}, pts...),
			xui.HistoryPoint{T: base1 + 120, V: 100}, xui.HistoryPoint{T: base1 + 180, V: 100},
			xui.HistoryPoint{T: base1 + 300, V: 200}, xui.HistoryPoint{T: base1 + 360, V: 200})
	}
	tel.history = h2
	c.collectTelemetry(time.Date(2026, 8, 31, 12, 12, 0, 0, time.UTC))

	rows, _ := db.Snapshots("one", 0, time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC).Unix())
	if len(rows) != 2 {
		t.Fatalf("снапшотов %d, want 2 (инкрементально, без дублей)", len(rows))
	}
	if rows[0].TS != base1 || rows[1].TS != base2 {
		t.Errorf("ts строк: %d, %d; want %d, %d", rows[0].TS, rows[1].TS, base1, base2)
	}
	// Окно 12:05 перезаписано новыми точками бакета (100,100 → avg 100), старые
	// точки (10,20) не пересчитаны — иначе avg был бы 57.5 (инкрементальность).
	if mustFloat(rows[0].CPUAvg) != 100 {
		t.Errorf("окно 12:05 cpu = %v, want 100 (только новые точки бакета)", rows[0].CPUAvg)
	}
	// Новое окно 12:10 — только его точки: cpu 200,200 → avg 200.
	if mustFloat(rows[1].CPUAvg) != 200 {
		t.Errorf("окно 12:10 cpu = %v, want 200", rows[1].CPUAvg)
	}
	// Свежее окно несёт поля из server/status.
	if mustFloat(rows[1].SwapAvg) != 25 {
		t.Errorf("окно 12:10 swap = %v, want 25%%", rows[1].SwapAvg)
	}
}

// Первый контакт с панелью: история за ~2-3 окна (11:55, 12:00, 12:05) → строка
// на каждое окно; у старых полей из server/status нет (nil, xray_ok=1), у самого
// свежего — есть. Повторный цикл с той же историей не дублирует строки.
func TestCollectTelemetryFirstContactFullHistory(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC) // окно 12:05
	base := time.Date(2026, 8, 31, 12, 5, 0, 0, time.UTC).Unix()

	ts := []int64{base - 600, base - 300, base, base + 60} // 11:55, 12:00, 12:05×2
	mk := func(vals ...float64) []xui.HistoryPoint {
		pts := make([]xui.HistoryPoint, 0, len(ts))
		for i, t := range ts {
			pts = append(pts, xui.HistoryPoint{T: t, V: vals[i]})
		}
		return pts
	}
	tel := &fakeTelemetry{status: testStatus(), history: map[string][]xui.HistoryPoint{
		"cpu":     mk(10, 20, 30, 40),
		"mem":     mk(50, 60, 70, 80),
		"netUp":   mk(1000, 2000, 3000, 4000),
		"netDown": mk(500, 500, 500, 500),
		"online":  mk(3, 5, 7, 9),
		"load1":   mk(1.0, 2.0, 3.0, 4.0),
		"load5":   mk(0.5, 0.6, 0.7, 0.8),
		"load15":  mk(0.25, 0.3, 0.35, 0.4),
	}}
	c, db, _ := newCollectorFixture(t, tel, nil, &fakeDaemon{})

	c.collectTelemetry(now)

	rows, err := db.Snapshots("one", 0, now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("снапшотов %d, want 3 (по строке на окно 11:55/12:00/12:05)", len(rows))
	}
	if rows[0].TS != base-600 || rows[1].TS != base-300 || rows[2].TS != base {
		t.Fatalf("ts строк: %d, %d, %d; want %d, %d, %d", rows[0].TS, rows[1].TS, rows[2].TS, base-600, base-300, base)
	}

	// Старые окна: агрегаты из истории, полей из status нет (nil, xray_ok=1).
	if mustFloat(rows[0].CPUAvg) != 10 || mustInt64(rows[0].NetUp) != 1000*historyBucket {
		t.Errorf("окно 11:55 cpu/net_up = %v/%d, want 10/%d", rows[0].CPUAvg, mustInt64(rows[0].NetUp), 1000*historyBucket)
	}
	if rows[0].SwapAvg != nil || rows[0].DiskUsed != nil || rows[0].DiskTotal != nil ||
		rows[0].NetTrafficSent != nil || rows[0].NetTrafficRecv != nil || rows[0].OpenConnsMax != nil {
		t.Errorf("окно 11:55: поля из status не nil: %+v", rows[0])
	}
	if rows[0].XrayOK != 1 {
		t.Errorf("окно 11:55 xray_ok = %d, want 1", rows[0].XrayOK)
	}
	if mustFloat(rows[1].CPUAvg) != 20 || rows[1].SwapAvg != nil {
		t.Errorf("окно 12:00 cpu/swap = %v/%v, want 20/nil", rows[1].CPUAvg, rows[1].SwapAvg)
	}

	// Свежее окно (12:05): две точки cpu 30,40 → avg 35/max 40 + поля из status.
	if mustFloat(rows[2].CPUAvg) != 35 || mustFloat(rows[2].CPUMax) != 40 {
		t.Errorf("окно 12:05 cpu avg/max = %v/%v, want 35/40", rows[2].CPUAvg, rows[2].CPUMax)
	}
	if mustInt64(rows[2].NetUp) != 7000*historyBucket || mustInt(rows[2].OnlineAvg) != 8 || mustInt(rows[2].OnlineMax) != 9 {
		t.Errorf("окно 12:05 net_up/online = %d/%d/%d, want %d/8/9",
			mustInt64(rows[2].NetUp), mustInt(rows[2].OnlineAvg), mustInt(rows[2].OnlineMax), 7000*historyBucket)
	}
	if mustFloat(rows[2].SwapAvg) != 25 || mustInt64(rows[2].DiskUsed) != 50<<30 || mustInt(rows[2].OpenConnsMax) != 42 {
		t.Errorf("окно 12:05 поля из status: swap=%v disk=%d conns=%d, want 25/%%/537…/42",
			rows[2].SwapAvg, mustInt64(rows[2].DiskUsed), mustInt(rows[2].OpenConnsMax))
	}

	// Повторный цикл с той же историей: новых точек нет (курсор lastTS на месте),
	// строки не дублируются.
	c.collectTelemetry(now.Add(5 * time.Minute)) // 12:12
	rows, _ = db.Snapshots("one", 0, now.Add(6*time.Minute).Unix())
	if len(rows) != 3 {
		t.Fatalf("после повторного цикла снапшотов %d, want 3 (без дублей)", len(rows))
	}
}

// Панель не отдаёт телеметрию: снапшот не пишется, повторных опросов нет (одна попытка).
func TestCollectTelemetryPanelDown(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC)
	tel := &fakeTelemetry{err: errors.New("timeout")}
	c, db, _ := newCollectorFixture(t, tel, nil, &fakeDaemon{})

	c.collectTelemetry(now)

	rows, err := db.Snapshots("one", 0, now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("при недоступной панели снапшотов %d, want 0", len(rows))
	}

	// Панель ожила — следующий цикл пишет данные (точки уже нового окна).
	tel.err = nil
	tel.status = testStatus()
	base2 := time.Date(2026, 8, 31, 12, 10, 0, 0, time.UTC).Unix()
	tel.history = fullHistory(base2)
	c.collectTelemetry(now.Add(5 * time.Minute)) // 12:12, окно 12:10

	rows, _ = db.Snapshots("one", 0, now.Add(10*time.Minute).Unix())
	if len(rows) != 1 {
		t.Fatalf("после восстановления снапшотов %d, want 1", len(rows))
	}
	if rows[0].TS != base2 {
		t.Errorf("ts восстановленного снапшота = %d, want %d", rows[0].TS, base2)
	}
}

// Дельты клиентов: первый срез — базлайн; второй — дельты; пустые/сброшенные не пишутся.
func TestCollectTrafficDeltas(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC)

	ib := func(up, down int64, stats []xui.XUIClientStats) []xui.XUIInbound {
		return []xui.XUIInbound{{ID: 1, Remark: "PL1", Up: up, Down: down, Total: up + down, ClientStats: stats}}
	}

	tel := &fakeTelemetry{status: testStatus()}
	first := ib(1000, 2000, []xui.XUIClientStats{
		{ID: 1, Email: "a@x", Up: 500, Down: 400},
		{ID: 3, Email: "c@x", Up: 100, Down: 100},
	})
	c, db, _ := newCollectorFixture(t, tel, first, &fakeDaemon{})

	// Первый срез: инбаунд пишется, клиентские дельты — нет (только базлайн).
	c.collectTelemetry(now)
	ibRows, err := db.InboundTrafficRows("one", 0, now.Unix())
	if err != nil {
		t.Fatal(err)
	}
	if len(ibRows) != 1 || ibRows[0].Total != 3000 {
		t.Fatalf("инбаунд-срезы: %+v", ibRows)
	}
	clRows, _ := db.ClientTrafficRows("one", 0, now.Unix())
	if len(clRows) != 0 {
		t.Fatalf("на первом срезе клиентских дельт %d, want 0", len(clRows))
	}

	// Второй срез: a@x нарос, b@x не менялся, у c@x счётчик сброшен (negative delta).
	next := now.Add(5 * time.Minute)
	c.panelsAPI = &fakePanelClient{inbounds: ib(5000, 3000, []xui.XUIClientStats{
		{ID: 1, Email: "a@x", Up: 1500, Down: 800}, // +1000/+400
		{ID: 2, Email: "b@x", Up: 10, Down: 10},    // без изменений
		{ID: 3, Email: "c@x", Up: 0, Down: 0},      // сброс счётчика → 0/0 → пропуск
	})}
	c.collectTelemetry(next)

	clRows, _ = db.ClientTrafficRows("one", 0, next.Unix())
	if len(clRows) != 1 {
		t.Fatalf("клиентских дельт %d, want 1 (только a@x)", len(clRows))
	}
	cr := clRows[0]
	if cr.Email != "a@x" || cr.UpDelta != 1000 || cr.DownDelta != 400 {
		t.Errorf("дельта a@x: %+v, want +1000/+400", cr)
	}
	if cr.TS != time.Date(2026, 8, 31, 12, 10, 0, 0, time.UTC).Unix() {
		t.Errorf("ts дельты = %d, want окно 12:10", cr.TS)
	}
}

// Забор прогонов: маппинг в test_runs + test_key_results, дедуп при повторе.
func TestCollectRuns(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 15, 0, 0, time.UTC)
	started := time.Date(2026, 8, 31, 6, 15, 0, 0, time.UTC)
	finished := started.Add(90 * time.Second)

	results, _ := json.Marshal([]dto.DaemonKeyResult{
		{KeyIdx: 0, IP: "1.1.1.1", Remark: "PL1-Olga", Status: "OK",
			AvgSpeedKbps: 7839.6, StabilityPct: 93.33, Reconnects: 1, LatencyMs: 84.33,
			TotalDownloadedMB: 143.55, SessionsOK: 14, SessionsFail: 1, DurationSec: 181},
		{KeyIdx: 1, IP: "2.2.2.2", Remark: "PL2-Olga", Status: "FAILED", Reason: "conn refused"},
	})
	daemon := &fakeDaemon{runs: []dto.DaemonRun{{
		ID: "2026-08-31T06:15:00.000000000Z", Kind: "test",
		SubscriptionURL: "https://80.87.202.236/sub/Olga",
		StartedAt:       started, FinishedAt: finished,
		Total: 2, OK: 1, Failed: 1,
		Results: results,
	}}}

	c, db, _ := newCollectorFixture(t, &fakeTelemetry{status: testStatus()}, nil, daemon)
	c.lastRuns = started.Add(-time.Hour)

	c.collectRuns(now)

	runs, err := db.TestRuns(now.AddDate(0, 0, -1), now, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("прогонов %d, want 1", len(runs))
	}
	r := runs[0]
	if r.SubscriptionID != "Olga" || r.Status != "partial" || r.Total != 2 || r.OKCount != 1 || r.FailCount != 1 {
		t.Errorf("прогон: %+v", r)
	}
	if r.StartedAt != "2026-08-31T06:15:00.000000000Z" {
		t.Errorf("started_at = %q", r.StartedAt)
	}

	keys, _ := db.TestKeyResults(r.ID)
	if len(keys) != 2 {
		t.Fatalf("key results %d, want 2", len(keys))
	}
	if keys[0].Status != "OK" || keys[1].Status != "FAIL" {
		t.Errorf("статусы: %+v, %+v", keys[0].Status, keys[1].Status)
	}
	// Все метрики демона сохраняются.
	k0 := keys[0]
	if k0.AvgSpeedKbps != 7839.6 || k0.StabilityPct != 93.33 || k0.Reconnects != 1 {
		t.Errorf("метрики скорости: %+v", k0)
	}
	if k0.TotalDownloadedMB != 143.55 || k0.SessionsOK != 14 || k0.SessionsFail != 1 || k0.DurationSec != 181 {
		t.Errorf("метрики сессий/объёма: %+v", k0)
	}
	if k0.LatencyMs == nil || *k0.LatencyMs != 84 {
		t.Errorf("latency_ms: %v, want 84", k0.LatencyMs)
	}
	if k0.IP != "1.1.1.1" || k0.Label != "PL1-Olga" {
		t.Errorf("ip/label: %+v", k0)
	}

	// Повторный забор (после рестарта) — дедуп, дублей нет.
	c.lastRuns = started.Add(-time.Hour)
	c.collectRuns(now)
	runs, _ = db.TestRuns(now.AddDate(0, 0, -1), now, "", "")
	if len(runs) != 1 {
		t.Fatalf("после повтора прогонов %d, want 1", len(runs))
	}
}

// Прогон, упавший целиком (error) → статус failed, per-key результатов нет.
func TestCollectRunsFailedRun(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 15, 0, 0, time.UTC)
	started := time.Date(2026, 8, 31, 6, 15, 0, 0, time.UTC)
	daemon := &fakeDaemon{runs: []dto.DaemonRun{{
		ID: "x", Kind: "test",
		SubscriptionURL: "https://80.87.202.236/sub/Olga",
		StartedAt:       started, FinishedAt: started.Add(time.Minute),
		Error: "subscription unreachable",
	}}}
	c, db, _ := newCollectorFixture(t, &fakeTelemetry{status: testStatus()}, nil, daemon)
	c.lastRuns = started.Add(-time.Hour)

	c.collectRuns(now)

	runs, _ := db.TestRuns(now.AddDate(0, 0, -1), now, "", "")
	if len(runs) != 1 || runs[0].Status != "failed" {
		t.Fatalf("прогон: %+v", runs)
	}
	if runs[0].Error == nil || *runs[0].Error != "subscription unreachable" {
		t.Errorf("error = %v", runs[0].Error)
	}
}

// Если демон лежал — курсор lastRuns не двигается, окно не пропадает.
func TestCollectRunsDaemonDown(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 15, 0, 0, time.UTC)
	daemon := &fakeDaemon{err: errors.New("connection refused")}
	c, db, _ := newCollectorFixture(t, &fakeTelemetry{status: testStatus()}, nil, daemon)
	c.lastRuns = now.Add(-6 * time.Hour)

	c.collectRuns(now)

	if !c.lastRuns.Equal(now.Add(-6 * time.Hour)) {
		t.Errorf("lastRuns сдвинут при недоступном демоне: %v", c.lastRuns)
	}
	runs, _ := db.TestRuns(now.AddDate(0, 0, -1), now, "", "")
	if len(runs) != 0 {
		t.Errorf("прогонов %d, want 0", len(runs))
	}
}

// Имя подписки из subscription_url (последний сегмент пути, URL-декод).
func TestSubscriptionNameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://80.87.202.236/sub/Olga":           "Olga",
		"https://80.87.202.236/sub/Olga%20Petrova": "Olga Petrova",
		"http://x/sub/":                            "http://x/sub/",
		"not-a-url":                                "not-a-url",
	}
	for in, want := range cases {
		if got := subscriptionNameFromURL(in); got != want {
			t.Errorf("subscriptionNameFromURL(%q) = %q, want %q", in, got, want)
		}
	}
}

// matchSubKeyID связывает красный ключ с SubKey по фрагменту vless-ссылки.
func TestMatchSubKeyID(t *testing.T) {
	keys := []model.SubKey{
		{ID: "k-1", Link: "vless://uuid@host:443?type=tcp#PL1-Olga"},
		{ID: "k-2", Link: "vless://uuid@host:443?type=tcp#PL2-Olga"},
	}
	id := matchSubKeyID(keys, "PL2-Olga")
	if id == nil || *id != "k-2" {
		t.Errorf("match = %v, want k-2", id)
	}
	if id := matchSubKeyID(keys, "unknown"); id != nil {
		t.Errorf("match unknown = %v, want nil", id)
	}
}

// nextRunsTick: сдвиг на 15 минут, интервал 6 часов, первый опрос не в полночь.
func TestNextRunsTick(t *testing.T) {
	// 00:10 → 00:15 того же дня
	got := nextRunsTick(time.Date(2026, 8, 31, 0, 10, 0, 0, time.UTC))
	want := time.Date(2026, 8, 31, 0, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("nextRunsTick(00:10) = %v, want %v", got, want)
	}
	// 00:20 → 06:15
	got = nextRunsTick(time.Date(2026, 8, 31, 0, 20, 0, 0, time.UTC))
	want = time.Date(2026, 8, 31, 6, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("nextRunsTick(00:20) = %v, want %v", got, want)
	}
	// 18:16 → 00:15 следующего дня
	got = nextRunsTick(time.Date(2026, 8, 31, 18, 16, 0, 0, time.UTC))
	want = time.Date(2026, 9, 1, 0, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("nextRunsTick(18:16) = %v, want %v", got, want)
	}
	// ровно 06:15 → 12:15
	got = nextRunsTick(time.Date(2026, 8, 31, 6, 15, 0, 0, time.UTC))
	want = time.Date(2026, 8, 31, 12, 15, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("nextRunsTick(06:15) = %v, want %v", got, want)
	}
}
