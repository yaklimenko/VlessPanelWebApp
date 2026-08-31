package main

import (
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	"vlesspanel/model"
)

func newTestMetricsDB(t *testing.T) *MetricsDB {
	t.Helper()
	db, err := NewMetricsDB(filepath.Join(t.TempDir(), "metrics.db"), log.New(os.Stderr, "test: ", 0))
	if err != nil {
		t.Fatalf("NewMetricsDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func mustFloat(f *float64) float64 {
	if f == nil {
		return -1
	}
	return *f
}

func mustInt64(i *int64) int64 {
	if i == nil {
		return -1
	}
	return *i
}

func mustInt(i *int) int {
	if i == nil {
		return -1
	}
	return *i
}

// Схема должна создаваться идемпотентно: все таблицы и индексы из заметки.
func TestMetricsDBSchemaCreated(t *testing.T) {
	db := newTestMetricsDB(t)

	tables := []string{"testers", "test_runs", "test_key_results", "panels",
		"panel_snapshots", "inbound_traffic", "client_traffic"}
	for _, name := range tables {
		var cnt int
		err := db.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&cnt)
		if err != nil || cnt != 1 {
			t.Errorf("таблица %s не создана (cnt=%d, err=%v)", name, cnt, err)
		}
	}

	indexes := []string{"idx_runs_sub", "idx_runs_tester", "idx_keys_run", "idx_snap"}
	for _, name := range indexes {
		var cnt int
		err := db.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&cnt)
		if err != nil || cnt != 1 {
			t.Errorf("индекс %s не создан (cnt=%d, err=%v)", name, cnt, err)
		}
	}

	// Повторная миграция не должна падать.
	if err := db.migrate(); err != nil {
		t.Fatalf("повторная миграция: %v", err)
	}
}

// Bootstrap: при пустой testers вставляется демон minicloud ровно один раз.
func TestMetricsDBBootstrapTester(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metrics.db")
	db, err := NewMetricsDB(path, nil)
	if err != nil {
		t.Fatal(err)
	}

	testers, err := db.ListTesters()
	if err != nil {
		t.Fatal(err)
	}
	if len(testers) != 1 {
		t.Fatalf("ожидался 1 тестер, got %d", len(testers))
	}
	tt := testers[0]
	if tt.Name != "vlesssubtest-minicloud" || tt.BaseURL != "http://vlesssubtest:7070" ||
		tt.Location != "minicloud" || tt.Enabled != 1 || tt.Priority != 0 {
		t.Errorf("неожиданный тестер: %+v", tt)
	}

	// Повторный старт на той же БД не должен дублировать.
	db.Close()
	db2, err := NewMetricsDB(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	testers, _ = db2.ListTesters()
	if len(testers) != 1 {
		t.Fatalf("после повторного старта тестеров %d, ожидался 1", len(testers))
	}
}

// SyncPanels — зеркало panels.json: upsert по id, изменение имени подхватывается.
func TestMetricsDBSyncPanels(t *testing.T) {
	db := newTestMetricsDB(t)

	panels := []model.Panel{{ID: "p1", Name: "One", URL: "https://one:1"}, {ID: "p2", Name: "Two", URL: "https://two:2"}}
	if err := db.SyncPanels(panels); err != nil {
		t.Fatal(err)
	}

	panels[0].Name = "One-renamed"
	if err := db.SyncPanels(panels); err != nil {
		t.Fatal(err)
	}

	list, err := db.ListPanels()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("panels = %d, want 2", len(list))
	}
	byID := map[string]model.Panel{}
	for _, p := range list {
		byID[p.ID] = p
	}
	if byID["p1"].Name != "One-renamed" {
		t.Errorf("имя не обновилось: %q", byID["p1"].Name)
	}
}

// Вставка снапшота + чтение; INSERT OR REPLACE по UNIQUE(panel_id, ts).
func TestMetricsDBInsertSnapshot(t *testing.T) {
	db := newTestMetricsDB(t)
	if err := db.SyncPanels([]model.Panel{{ID: "p1", Name: "One", URL: "https://one:1"}}); err != nil {
		t.Fatal(err)
	}

	cpu, mem := 12.5, 30.0
	netUp := int64(1000)
	discUsed, discTotal := int64(50), int64(100)
	online := 3
	conns := 42
	err := db.InsertSnapshot(SnapshotRecord{
		PanelID: "p1", TS: 1000,
		CPUAvg: &cpu, CPUMax: &cpu, MemAvg: &mem, MemMax: &mem,
		NetUp: &netUp, NetTrafficSent: &netUp,
		DiskUsed: &discUsed, DiskTotal: &discTotal,
		OnlineAvg: &online, OnlineMax: &online, OpenConnsMax: &conns, XrayOK: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.Snapshots("p1", 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(rows))
	}
	s := rows[0]
	if mustFloat(s.CPUAvg) != 12.5 || mustInt64(s.NetUp) != 1000 || mustInt(s.OpenConnsMax) != 42 {
		t.Errorf("неверные значения снапшота: %+v", s)
	}

	// Тот же ts → замена, не дубль.
	cpu2 := 99.0
	if err := db.InsertSnapshot(SnapshotRecord{PanelID: "p1", TS: 1000, CPUAvg: &cpu2, CPUMax: &cpu2, XrayOK: 1}); err != nil {
		t.Fatal(err)
	}
	rows, _ = db.Snapshots("p1", 0, 2000)
	if len(rows) != 1 {
		t.Fatalf("после replace snapshots = %d, want 1", len(rows))
	}
	if mustFloat(rows[0].CPUAvg) != 99.0 {
		t.Errorf("replace не сработал: cpu=%v", rows[0].CPUAvg)
	}
}

// InsertTestRun пишет прогон + результаты; повторная вставка — дедуп, тот же id.
func TestMetricsDBInsertTestRunDedup(t *testing.T) {
	db := newTestMetricsDB(t)
	testers, _ := db.ListTesters()
	testerID := testers[0].ID

	run := &TestRun{
		TesterID:       testerID,
		SubscriptionID: "Olga",
		Status:         "partial",
		Total:          2,
		OKCount:        1,
		FailCount:      1,
		StartedAt:      "2026-08-31T12:15:00.000000000Z",
		FinishedAt:     "2026-08-31T12:16:00.000000000Z",
	}
	keys := []TestKeyResult{
		{Label: "PL1-Olga", Status: "OK", IP: "1.2.3.4", TestedAt: "2026-08-31T12:15:00.000000000Z"},
		{Label: "PL2-Olga", Status: "FAIL", TestedAt: "2026-08-31T12:15:00.000000000Z"},
	}

	id1, inserted, err := db.InsertTestRun(run, keys)
	if err != nil || !inserted {
		t.Fatalf("первая вставка: inserted=%v err=%v", inserted, err)
	}
	id2, inserted, err := db.InsertTestRun(run, keys)
	if err != nil || inserted {
		t.Fatalf("повторная вставка должна быть дедупом: inserted=%v err=%v", inserted, err)
	}
	if id1 != id2 {
		t.Errorf("id разошлись: %d vs %d", id1, id2)
	}

	kr, err := db.TestKeyResults(id1)
	if err != nil {
		t.Fatal(err)
	}
	if len(kr) != 2 {
		t.Fatalf("key results = %d, want 2", len(kr))
	}
	if kr[0].Status != "OK" || kr[1].Status != "FAIL" {
		t.Errorf("статусы ключей: %+v", kr)
	}

	got, ok, err := db.TestRunByID(id1)
	if err != nil || !ok {
		t.Fatalf("TestRunByID: ok=%v err=%v", ok, err)
	}
	if got.SubscriptionID != "Olga" || got.Status != "partial" || got.FailCount != 1 {
		t.Errorf("прогон: %+v", got)
	}
}

// Retention: телеметрия и прогоны старше 3 месяцев чистятся, свежие остаются.
func TestMetricsDBCleanupRetention(t *testing.T) {
	db := newTestMetricsDB(t)
	if err := db.SyncPanels([]model.Panel{{ID: "p1", Name: "One", URL: "https://one:1"}}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	oldTS := now.AddDate(0, -4, 0).Unix() // 4 месяца назад
	freshTS := now.Add(-time.Hour).Unix() // час назад

	cpu := 1.0
	old := SnapshotRecord{PanelID: "p1", TS: oldTS, CPUAvg: &cpu, CPUMax: &cpu, XrayOK: 1}
	fresh := SnapshotRecord{PanelID: "p1", TS: freshTS, CPUAvg: &cpu, CPUMax: &cpu, XrayOK: 1}
	if err := db.InsertSnapshot(old); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertSnapshot(fresh); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertInboundTraffic(InboundTrafficRecord{PanelID: "p1", InboundID: 1, TS: oldTS, Up: 1, Down: 1, Total: 2}); err != nil {
		t.Fatal(err)
	}

	testers, _ := db.ListTesters()
	oldRun := &TestRun{
		TesterID: testers[0].ID, SubscriptionID: "Old", Status: "ok",
		StartedAt:  now.AddDate(0, -4, 0).UTC().Format(runTimeFormat),
		FinishedAt: now.AddDate(0, -4, 0).UTC().Format(runTimeFormat),
	}
	oldRunID, _, err := db.InsertTestRun(oldRun, []TestKeyResult{{Label: "k", Status: "OK", TestedAt: oldRun.StartedAt}})
	if err != nil {
		t.Fatal(err)
	}
	freshRun := &TestRun{
		TesterID: testers[0].ID, SubscriptionID: "Fresh", Status: "ok",
		StartedAt:  now.Add(-time.Hour).UTC().Format(runTimeFormat),
		FinishedAt: now.Add(-time.Hour).UTC().Format(runTimeFormat),
	}
	if _, _, err := db.InsertTestRun(freshRun, nil); err != nil {
		t.Fatal(err)
	}

	if err := db.CleanupRetention(now); err != nil {
		t.Fatal(err)
	}

	snap, _ := db.Snapshots("p1", 0, now.Unix())
	if len(snap) != 1 || snap[0].TS != freshTS {
		t.Errorf("после чистки снапшотов %d, ожидался 1 свежий (ts=%d)", len(snap), freshTS)
	}
	ib, _ := db.InboundTrafficRows("p1", 0, now.Unix())
	if len(ib) != 0 {
		t.Errorf("старый инбаунд-срез не удалён: %d", len(ib))
	}
	runs, _ := db.TestRuns(now.AddDate(0, -5, 0), now, "", "")
	if len(runs) != 1 || runs[0].SubscriptionID != "Fresh" {
		t.Errorf("после чистки прогонов %d, ожидался 1 свежий", len(runs))
	}
	// Каскад: прогон и его key results удалены вместе.
	kr, _ := db.TestKeyResults(oldRunID)
	if len(kr) != 0 {
		t.Errorf("key results старого прогона не каскадировались: %d", len(kr))
	}
	if _, ok, _ := db.TestRunByID(oldRunID); ok {
		t.Errorf("старый прогон не удалён")
	}
}

// Availability: последний ts снапшота по панелям.
func TestMetricsDBPanelAvailability(t *testing.T) {
	db := newTestMetricsDB(t)
	if err := db.SyncPanels([]model.Panel{
		{ID: "p1", Name: "One", URL: "https://one:1"},
		{ID: "p2", Name: "Two", URL: "https://two:2"},
	}); err != nil {
		t.Fatal(err)
	}
	cpu := 1.0
	if err := db.InsertSnapshot(SnapshotRecord{PanelID: "p1", TS: 500, CPUAvg: &cpu, CPUMax: &cpu, XrayOK: 1}); err != nil {
		t.Fatal(err)
	}

	av, err := db.PanelAvailability()
	if err != nil {
		t.Fatal(err)
	}
	if len(av) != 2 {
		t.Fatalf("availability = %d, want 2", len(av))
	}
	if av[0].PanelID == "p1" && av[0].LastSnapshotTS != 500 {
		t.Errorf("p1 last ts = %d, want 500", av[0].LastSnapshotTS)
	}
}
