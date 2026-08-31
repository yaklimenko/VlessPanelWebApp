package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"vlesspanel/model"
	"vlesspanel/xui"
)

// server/status парсится в структуру (включая xray, netIO, load).
func TestPanelAPIServerStatus(t *testing.T) {
	payload := `{"success":true,"obj":{
		"cpu":12.5,
		"mem":{"current":2147483648,"total":8589934592},
		"swap":{"current":0,"total":4294967296},
		"disk":{"current":53687091200,"total":268435456000},
		"netIO":{"up":1073741824,"down":2147483648},
		"xray":{"state":"running","version":"v25.10.31"},
		"tcpCount":42,
		"load":{"load1":0.5,"load3":0.4,"load15":0.2}
	}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/api/server/status" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("auth = %q", r.Header.Get("Authorization"))
		}
		w.Write([]byte(payload))
	}))
	defer srv.Close()

	api := NewPanelAPI()
	st, err := api.ServerStatus(model.Panel{URL: srv.URL, Token: "tok"})
	if err != nil {
		t.Fatal(err)
	}
	if st.CPU != 12.5 || st.TCPCount != 42 {
		t.Errorf("cpu/tcp = %v/%d", st.CPU, st.TCPCount)
	}
	if st.Mem.Total != 8589934592 || st.Swap.Total != 4294967296 {
		t.Errorf("mem/swap: %+v / %+v", st.Mem, st.Swap)
	}
	if st.NetIO.Up != 1073741824 || st.NetIO.Down != 2147483648 {
		t.Errorf("netIO: %+v", st.NetIO)
	}
	if st.Xray.State != "running" || st.Load.Load1 != 0.5 {
		t.Errorf("xray/load: %+v / %+v", st.Xray, st.Load)
	}
}

// server/history/{metric}/{bucket} — точки {t, v} парсятся; bucket передаётся в URL.
func TestPanelAPIServerHistory(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/panel/api/server/history/cpu/60" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Write([]byte(`{"success":true,"obj":[{"t":1700000000,"v":12.5},{"t":1700000060,"v":13.1}]}`))
	}))
	defer srv.Close()

	api := NewPanelAPI()
	pts, err := api.ServerHistory(model.Panel{URL: srv.URL, Token: "tok"}, "cpu", 60)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 2 || pts[0].T != 1700000000 || pts[0].V != 12.5 || pts[1].V != 13.1 {
		t.Errorf("points: %+v", pts)
	}
}

// Ошибка панели (HTTP 500) → ошибка, а не паника.
func TestPanelAPITelemetryError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`oops`))
	}))
	defer srv.Close()

	api := NewPanelAPI()
	if _, err := api.ServerStatus(model.Panel{URL: srv.URL, Token: "tok"}); err == nil {
		t.Fatal("ожидалась ошибка server/status")
	}
	if _, err := api.ServerHistory(model.Panel{URL: srv.URL, Token: "tok"}, "cpu", 60); err == nil {
		t.Fatal("ожидалась ошибка server/history")
	}
}

// success=false → ошибка с msg панели.
func TestPanelAPITelemetryUnsuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":false,"msg":"invalid bucket"}`))
	}))
	defer srv.Close()

	api := NewPanelAPI()
	_, err := api.ServerHistory(model.Panel{URL: srv.URL, Token: "tok"}, "cpu", 300)
	if err == nil {
		t.Fatal("ожидалась ошибка invalid bucket")
	}
}

// Защита: невалидный obj не роняет парсинг.
func TestPanelAPITelemetryBadObj(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"obj":"not-an-object"}`))
	}))
	defer srv.Close()

	api := NewPanelAPI()
	if _, err := api.ServerStatus(model.Panel{URL: srv.URL, Token: "tok"}); err == nil {
		t.Fatal("ожидалась ошибка разбора status")
	}
}

// Типы сериализуются в JSON (для DTO/ответов API).
func TestServerStatusJSONTags(t *testing.T) {
	st := &xui.ServerStatus{CPU: 1, TCPCount: 5}
	b, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("пустой JSON")
	}
}
