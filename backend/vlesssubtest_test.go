package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestListRuns_FromToRFC3339 проверяет, что ListRuns шлёт демону from/to
// в формате RFC3339 (с Z): демон (VlessSubTest parseTimeParam) принимает
// только YYYY-MM-DD или RFC3339/RFC3339Nano, формат без timezone даёт 400.
func TestListRuns_FromToRFC3339(t *testing.T) {
	var gotFrom, gotTo string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		gotFrom = q.Get("from")
		gotTo = q.Get("to")
		if gotFrom == "" || gotTo == "" {
			t.Errorf("missing from/to in request: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"runs":[]}`))
	}))
	defer srv.Close()

	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)

	c := NewVlessSubTestClient(srv.URL)
	if _, err := c.ListRuns(from, to); err != nil {
		t.Fatalf("ListRuns: %v", err)
	}

	// Формат должен быть RFC3339 с Z: "2026-08-31T00:00:00Z".
	for name, got := range map[string]string{"from": gotFrom, "to": gotTo} {
		ts, err := time.Parse(time.RFC3339, got)
		if err != nil {
			t.Errorf("%s=%q не парсится как RFC3339: %v", name, got, err)
			continue
		}
		if !strings.HasSuffix(got, "Z") {
			t.Errorf("%s=%q: ожидается timezone-суффикс Z (RFC3339)", name, got)
		}
		// Значение должно совпадать с тем, что мы передали (UTC).
		if !ts.Equal(from) && name == "from" {
			t.Errorf("from=%q, ожидалось %v", got, from)
		}
		if !ts.Equal(to) && name == "to" {
			t.Errorf("to=%q, ожидалось %v", got, to)
		}
	}
}

// TestListRuns_ParseRFC3339Nano проверяет, что ответ демона со временами
// в RFC3339Nano (наносекунды + Z) корректно разбирается в DaemonRun.
func TestListRuns_ParseRFC3339Nano(t *testing.T) {
	body := `{
		"total": 2,
		"runs": [
			{
				"id": "run-1",
				"kind": "test",
				"subscription_url": "https://example.com/sub/Olga",
				"started_at": "2026-08-31T16:00:00.099051015Z",
				"finished_at": "2026-08-31T16:00:12.345678901Z",
				"duration_sec": 12,
				"total": 5,
				"ok": 5,
				"failed": 0,
				"results": [
					{"key_idx": 0, "ip": "1.2.3.4", "remark": "PL1-TCP", "status": "OK", "youtube": "OK", "instagram": "OK"}
				]
			},
			{
				"id": "run-2",
				"kind": "probe",
				"subscription_url": "https://example.com/sub/Probe",
				"started_at": "2026-08-31T18:00:00.5Z",
				"finished_at": "2026-08-31T18:00:09Z",
				"duration_sec": 9,
				"total": 2,
				"ok": 1,
				"degraded": 1,
				"failed": 0,
				"results": [
					{"key_idx": 0, "ip": "5.6.7.8", "remark": "NL2-xHTTP", "status": "DEGRADED", "reason": "slow", "youtube": "SLOW", "instagram": "OK"}
				]
			}
		]
	}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	defer srv.Close()

	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)

	c := NewVlessSubTestClient(srv.URL)
	runs, err := c.ListRuns(from, to)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs)=%d, ожидалось 2", len(runs))
	}

	r0 := runs[0]
	if r0.ID != "run-1" {
		t.Errorf("ID=%q, ожидалось run-1", r0.ID)
	}
	wantStart, _ := time.Parse(time.RFC3339Nano, "2026-08-31T16:00:00.099051015Z")
	if !r0.StartedAt.Equal(wantStart) {
		t.Errorf("StartedAt=%v, ожидалось %v", r0.StartedAt, wantStart)
	}
	wantFinish, _ := time.Parse(time.RFC3339Nano, "2026-08-31T16:00:12.345678901Z")
	if !r0.FinishedAt.Equal(wantFinish) {
		t.Errorf("FinishedAt=%v, ожидалось %v", r0.FinishedAt, wantFinish)
	}

	// Results — сырой JSON, должны разбираться в []DaemonKeyResult.
	var keys []DaemonKeyResult
	if err := json.Unmarshal(r0.Results, &keys); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	if len(keys) != 1 || keys[0].Status != "OK" || keys[0].Remark != "PL1-TCP" {
		t.Errorf("results разобраны неверно: %+v", keys)
	}

	r1 := runs[1]
	wantStart1, _ := time.Parse(time.RFC3339Nano, "2026-08-31T18:00:00.5Z")
	if !r1.StartedAt.Equal(wantStart1) {
		t.Errorf("r1 StartedAt=%v, ожидалось %v", r1.StartedAt, wantStart1)
	}
	wantFinish1, _ := time.Parse(time.RFC3339Nano, "2026-08-31T18:00:09Z")
	if !r1.FinishedAt.Equal(wantFinish1) {
		t.Errorf("r1 FinishedAt=%v, ожидалось %v", r1.FinishedAt, wantFinish1)
	}
}

// TestListRuns_Non200 проверяет, что ответ 400 (например invalid time)
// оборачивается в ErrDaemonParse.
func TestListRuns_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"invalid time \"2026-08-31T00:00:00\" (use YYYY-MM-DD or RFC3339)"}`))
	}))
	defer srv.Close()

	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC)

	c := NewVlessSubTestClient(srv.URL)
	_, err := c.ListRuns(from, to)
	if err == nil {
		t.Fatal("ожидалась ошибка при HTTP 400")
	}
	if !errors.Is(err, ErrDaemonParse) {
		t.Errorf("ошибка %v, ожидалась обёртка ErrDaemonParse", err)
	}
}

// TestListRuns_QueryParams проверяет, что запрос к демону содержит
// detail=1 и limit=1000 (нужные для забора per-key деталей).
func TestListRuns_QueryParams(t *testing.T) {
	var q url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"total":0,"runs":[]}`))
	}))
	defer srv.Close()

	from := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	c := NewVlessSubTestClient(srv.URL)
	if _, err := c.ListRuns(from, to); err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if q.Get("detail") != "1" {
		t.Errorf("detail=%q, ожидалось 1", q.Get("detail"))
	}
	if q.Get("limit") != "1000" {
		t.Errorf("limit=%q, ожидалось 1000", q.Get("limit"))
	}
}
