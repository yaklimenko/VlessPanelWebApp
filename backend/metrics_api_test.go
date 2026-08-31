package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"vlesspanel/dto"
	"vlesspanel/model"

	"github.com/go-chi/chi/v5"
)

func getMetrics(t *testing.T, h *MetricsHandlers, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	switch {
	case strings.HasPrefix(path, "/snaps"):
		h.Snapshots(rr, req)
	case strings.HasPrefix(path, "/traf"):
		h.Traffic(rr, req)
	case strings.HasPrefix(path, "/runs"):
		h.TestRuns(rr, req)
	case strings.HasPrefix(path, "/avai"):
		h.Availability(rr, req)
	default:
		t.Fatalf("unknown path %q", path)
	}
	return rr
}

func seedSnapshots(t *testing.T, db *MetricsDB, panelID string, n int, stepSec int64, baseTS int64) {
	t.Helper()
	if err := db.SyncPanels([]model.Panel{{ID: panelID, Name: "One", URL: "https://one:1"}}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < n; i++ {
		cpu := float64(i)
		netUp := int64(i * 100)
		conns := 10 + i
		rec := SnapshotRecord{
			PanelID: panelID, TS: baseTS + int64(i)*stepSec,
			CPUAvg: &cpu, CPUMax: &cpu, NetUp: &netUp, NetDown: &netUp,
			NetTrafficSent: &netUp, NetTrafficRecv: &netUp,
			OpenConnsMax: &conns, XrayOK: 1,
		}
		if err := db.InsertSnapshot(rec); err != nil {
			t.Fatal(err)
		}
	}
}

// GET /api/metrics/snapshots: диапазон + агрегация в бакеты.
func TestMetricsSnapshotsHandler(t *testing.T) {
	db := newTestMetricsDB(t)
	// 4 строки с шагом 5 минут, все внутри одного часового бакета:
	// час_старт - 20/15/10/5 минут (детерминированно, не зависит от времени суток).
	hourStart := time.Now().UTC().Truncate(time.Hour)
	seedSnapshots(t, db, "p1", 4, 300, hourStart.Add(-20*time.Minute).Unix())

	h := NewMetricsHandlers(db)
	rr := getMetrics(t, h, "/snaps?range=24h&panelId=p1")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body.String())
	}

	var resp dto.MetricsSnapshotResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Range != "24h" || resp.BucketSeconds != 300 || len(resp.Points) != 4 {
		t.Fatalf("resp: range=%s bucket=%d points=%d", resp.Range, resp.BucketSeconds, len(resp.Points))
	}
	// Точки по возрастанию ts.
	if resp.Points[0].TS > resp.Points[1].TS {
		t.Errorf("точки не отсортированы")
	}
	if resp.Points[0].CPUAvg == nil || *resp.Points[0].CPUAvg != 0 {
		t.Errorf("первая точка cpu=%v, want 0", resp.Points[0].CPUAvg)
	}

	// 7d с часовыми бакетами: 4 строки внутри одного часа → 1 бакет, avg=1.5.
	rr = getMetrics(t, h, "/snaps?range=7d&panelId=p1")
	var resp7 dto.MetricsSnapshotResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp7); err != nil {
		t.Fatal(err)
	}
	if resp7.BucketSeconds != 3600 || len(resp7.Points) != 1 {
		t.Fatalf("7d: bucket=%d points=%d, want 3600/1", resp7.BucketSeconds, len(resp7.Points))
	}
	p := resp7.Points[0]
	if p.CPUAvg == nil || *p.CPUAvg != 1.5 {
		t.Errorf("7d cpuAvg = %v, want 1.5", p.CPUAvg)
	}
	if p.CPUMax == nil || *p.CPUMax != 3 {
		t.Errorf("7d cpuMax = %v, want 3", p.CPUMax)
	}
	if p.NetUp == nil || *p.NetUp != 600 {
		t.Errorf("7d netUp = %v, want 600 (сумма 0+100+200+300)", p.NetUp)
	}
	// Кумулятивный счётчик — значение последней строки бакета.
	if p.NetTrafficSent == nil || *p.NetTrafficSent != 300 {
		t.Errorf("7d netTrafficSent = %v, want 300", p.NetTrafficSent)
	}
}

// panelId обязателен.
func TestMetricsSnapshotsRequirePanel(t *testing.T) {
	db := newTestMetricsDB(t)
	h := NewMetricsHandlers(db)
	rr := getMetrics(t, h, "/snaps?range=24h")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

// GET /api/metrics/traffic: дельты инбаундов считаются на чтении.
func TestMetricsTrafficHandlerInbound(t *testing.T) {
	db := newTestMetricsDB(t)
	// Базовый срез — на 5 минут в прошлом от выровненного окна, чтобы все
	// срезы гарантированно попадали в [from, now] и в один 300-сек бакет.
	base := time.Now().UTC().Truncate(300*time.Second).Unix() - 300
	if err := db.SyncPanels([]model.Panel{{ID: "p1", Name: "One", URL: "https://one:1"}}); err != nil {
		t.Fatal(err)
	}
	// Срезы одного инбаунда в одном 300-сек бакете: 1000→3000 (дельта 2000),
	// затем 3000→3300 (дельта 300). Итого за бакет 2300.
	for i, up := range []int64{1000, 3000, 3300} {
		if err := db.InsertInboundTraffic(InboundTrafficRecord{
			PanelID: "p1", InboundID: 1, Remark: "PL1", TS: base + int64(i)*60, Up: up, Down: up, Total: up * 2,
		}); err != nil {
			t.Fatal(err)
		}
	}

	h := NewMetricsHandlers(db)
	rr := getMetrics(t, h, "/traf?range=24h&panelId=p1&groupBy=inbound")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rr.Code, rr.Body.String())
	}
	var resp dto.MetricsTrafficResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	points, ok := resp.Points.([]interface{})
	if !ok {
		t.Fatalf("points type %T", resp.Points)
	}
	if len(points) != 1 {
		t.Fatalf("точек %d, want 1 (первый срез — база)", len(points))
	}
	b, _ := json.Marshal(points[0])
	var pt dto.InboundTrafficPoint
	if err := json.Unmarshal(b, &pt); err != nil {
		t.Fatal(err)
	}
	// Срезы 1000,3000,3300 в одном бакете → дельты 2000+300=2300.
	if pt.UpDelta != 2300 || pt.DownDelta != 2300 || pt.Remark != "PL1" {
		t.Errorf("точка: %+v, want up/down 2300", pt)
	}
}

// GET /api/metrics/traffic?groupBy=client: сумма delta-записей по бакету.
func TestMetricsTrafficHandlerClient(t *testing.T) {
	db := newTestMetricsDB(t)
	base := time.Now().UTC().Truncate(300*time.Second).Unix() - 300
	if err := db.SyncPanels([]model.Panel{{ID: "p1", Name: "One", URL: "https://one:1"}}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []ClientTrafficRecord{
		{PanelID: "p1", InboundID: 1, Email: "a@x", TS: base, UpDelta: 100, DownDelta: 50},
		{PanelID: "p1", InboundID: 1, Email: "a@x", TS: base + 60, UpDelta: 200, DownDelta: 100},
		{PanelID: "p1", InboundID: 1, Email: "b@x", TS: base, UpDelta: 10, DownDelta: 5},
	} {
		if err := db.InsertClientTraffic(d); err != nil {
			t.Fatal(err)
		}
	}

	h := NewMetricsHandlers(db)
	rr := getMetrics(t, h, "/traf?range=24h&panelId=p1&groupBy=client")
	var resp dto.MetricsTrafficResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	points, _ := resp.Points.([]interface{})
	if len(points) != 2 {
		t.Fatalf("точек %d, want 2 (a@x и b@x в одном бакете)", len(points))
	}
	b, _ := json.Marshal(points[0])
	var pt dto.ClientTrafficPoint
	if err := json.Unmarshal(b, &pt); err != nil {
		t.Fatal(err)
	}
	// a@x в бакете base: 100+200=300 up.
	if pt.Email == "a@x" && pt.TS == base && pt.UpDelta != 300 {
		t.Errorf("a@x base up = %d, want 300", pt.UpDelta)
	}
}

// GET /api/metrics/test-runs: список прогонов с именем тестера.
func TestMetricsTestRunsHandler(t *testing.T) {
	db := newTestMetricsDB(t)
	testers, _ := db.ListTesters()
	now := time.Now().UTC()
	run := &TestRun{
		TesterID: testers[0].ID, SubscriptionID: "Olga", Status: "ok",
		Total: 2, OKCount: 2,
		StartedAt:  now.Add(-time.Hour).UTC().Format(runTimeFormat),
		FinishedAt: now.Add(-time.Hour).UTC().Format(runTimeFormat),
	}
	id, _, err := db.InsertTestRun(run, []TestKeyResult{
		{Label: "PL1-Olga", Status: "OK", TestedAt: run.StartedAt},
		{Label: "PL2-Olga", Status: "OK", TestedAt: run.StartedAt},
	})
	if err != nil {
		t.Fatal(err)
	}

	h := NewMetricsHandlers(db)
	rr := getMetrics(t, h, "/runs?range=24h")
	var resp dto.MetricsTestRunsResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Runs) != 1 {
		t.Fatalf("runs = %d, want 1", len(resp.Runs))
	}
	r := resp.Runs[0]
	if r.SubscriptionID != "Olga" || r.TesterName != "vlesssubtest-minicloud" || r.OKCount != 2 {
		t.Errorf("run: %+v", r)
	}
	if len(r.Results) != 0 {
		t.Errorf("в списке не должно быть per-key результатов, got %d", len(r.Results))
	}

	// Detail: /runs/{id}
	req := httptest.NewRequest(http.MethodGet, "/api/metrics/test-runs/"+itoa(id), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", itoa(id))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr2 := httptest.NewRecorder()
	h.TestRunDetail(rr2, req)
	if rr2.Code != http.StatusOK {
		t.Fatalf("detail status = %d: %s", rr2.Code, rr2.Body.String())
	}
	var det dto.MetricsTestRun
	if err := json.Unmarshal(rr2.Body.Bytes(), &det); err != nil {
		t.Fatal(err)
	}
	if len(det.Results) != 2 || det.Results[0].Status != "OK" {
		t.Errorf("detail results: %+v", det.Results)
	}

	// Несуществующий id → 404.
	req = httptest.NewRequest(http.MethodGet, "/api/metrics/test-runs/99999", nil)
	rctx = chi.NewRouteContext()
	rctx.URLParams.Add("id", "99999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rr3 := httptest.NewRecorder()
	h.TestRunDetail(rr3, req)
	if rr3.Code != http.StatusNotFound {
		t.Fatalf("несуществующий id: status %d, want 404", rr3.Code)
	}
}

// GET /api/metrics/availability: последний снапшот по панели.
func TestMetricsAvailabilityHandler(t *testing.T) {
	db := newTestMetricsDB(t)
	now := time.Now().UTC()
	seedSnapshots(t, db, "p1", 1, 300, now.Unix())

	h := NewMetricsHandlers(db)
	rr := getMetrics(t, h, "/avai")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var out []dto.MetricsAvailability
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].PanelID != "p1" || out[0].LastSnapshotTS != now.Unix() {
		t.Errorf("availability: %+v", out)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
