package main

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

// newCollectorFixture — storage с панелью p1 + коллектор на фейках.
func newCollectorFixture(t *testing.T, tel *fakeTelemetry, inbounds []xui.XUIInbound, daemon *fakeDaemon) (*MetricsCollector, *MetricsDB, *Storage) {
	t.Helper()
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
	if mustInt64(s.NetUp) != 3000 || mustInt64(s.NetDown) != 1000 {
		t.Errorf("net up/down = %d/%d, want 3000/1000", mustInt64(s.NetUp), mustInt64(s.NetDown))
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
func TestCollectTelemetryIncremental(t *testing.T) {
	now1 := time.Date(2026, 8, 31, 12, 7, 0, 0, time.UTC)
	base1 := time.Date(2026, 8, 31, 12, 5, 0, 0, time.UTC).Unix()

	tel := &fakeTelemetry{status: testStatus(), history: fullHistory(base1)}
	c, db, _ := newCollectorFixture(t, tel, nil, &fakeDaemon{})

	c.collectTelemetry(now1)

	// Второй цикл: панель отдаёт полную историю (как настоящая), новые точки — 12:07..12:11.
	base2 := time.Date(2026, 8, 31, 12, 10, 0, 0, time.UTC).Unix()
	h2 := map[string][]xui.HistoryPoint{}
	for m, pts := range fullHistory(base1) {
		h2[m] = append(append([]xui.HistoryPoint{}, pts...), xui.HistoryPoint{T: base1 + 120, V: 100}, xui.HistoryPoint{T: base1 + 180, V: 100})
	}
	// Точки из окон 12:05/12:10 не должны задваивать значения предыдущей строки.
	tel.history = h2
	c.collectTelemetry(time.Date(2026, 8, 31, 12, 12, 0, 0, time.UTC))

	rows, _ := db.Snapshots("one", 0, time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC).Unix())
	if len(rows) != 2 {
		t.Fatalf("снапшотов %d, want 2 (инкрементально)", len(rows))
	}
	if rows[0].TS != base1 || rows[1].TS != base2 {
		t.Errorf("ts строк: %d, %d; want %d, %d", rows[0].TS, rows[1].TS, base1, base2)
	}
	// Первая строка не тронута (была записана первым циклом).
	if mustFloat(rows[0].CPUAvg) != 15 {
		t.Errorf("первая строка перезаписана: cpu=%v", rows[0].CPUAvg)
	}
	// Вторая строка — только новые точки (12:10..12:11): cpu 100,100 → avg 100.
	if mustFloat(rows[1].CPUAvg) != 100 {
		t.Errorf("вторая строка cpu = %v, want 100", rows[1].CPUAvg)
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

	results, _ := json.Marshal([]DaemonKeyResult{
		{KeyIdx: 0, IP: "1.1.1.1", Remark: "PL1-Olga", Status: "OK", Youtube: "OK", Instagram: "OK"},
		{KeyIdx: 1, IP: "2.2.2.2", Remark: "PL2-Olga", Status: "FAILED", Youtube: "FAIL", Instagram: "FAIL"},
	})
	daemon := &fakeDaemon{runs: []DaemonRun{{
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
	if keys[0].YouTube != "OK" || keys[1].YouTube != "FAIL" {
		t.Errorf("youtube: %q, %q", keys[0].YouTube, keys[1].YouTube)
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
	daemon := &fakeDaemon{runs: []DaemonRun{{
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
