package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vlesspanel/dto"
	"vlesspanel/model"

	"github.com/go-chi/chi/v5"
)

// newTestHandlers собирает Handlers на реальном Storage + фейках (auth выключен).
func newTestHandlers(t *testing.T) (*Handlers, *Storage) {
	t.Helper()
	s := newTestStorage(t)
	panelAPI := &fakePanelClient{}
	daemon := &fakeDaemon{}
	auth := NewTokenAuth("")
	panels := NewPanelService(s, panelAPI)
	subscriptions := NewSubscriptionService(s, panelAPI, NewSyncState(), daemon)
	keySources := NewKeySourceService(s, panelAPI, NewSyncState(), daemon)
	syncSvc := NewSyncService(NewSyncState(), &fakeSyncer{})
	tokens := NewTokenService(s, auth)
	return NewHandlers(auth, panels, subscriptions, keySources, syncSvc, tokens, daemon, ""), s
}

// panelRouter — минимальный chi-роутер с PUT /panels/{id} (как в main.go).
func panelRouter(h *Handlers) http.Handler {
	r := chi.NewRouter()
	r.Put("/panels/{id}", h.UpdatePanel)
	return r
}

func doPutPanel(t *testing.T, router http.Handler, id, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/panels/"+id, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestUpdatePanelHandlerSuccess(t *testing.T) {
	h, s := newTestHandlers(t)
	panel, err := s.AddPanel(dto.CreatePanelRequest{Name: "Old Name", URL: "https://x:1", Token: "t"})
	if err != nil {
		t.Fatal(err)
	}

	w := doPutPanel(t, panelRouter(h), panel.ID, `{"name":"New Name"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var updated model.Panel
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != panel.ID || updated.Name != "New Name" {
		t.Fatalf("updated = %+v", updated)
	}

	// Сохранено в storage, URL/Token не тронуты.
	got, err := s.GetPanel(panel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "New Name" || got.URL != "https://x:1" || got.Token != "t" {
		t.Fatalf("persisted = %+v", got)
	}
}

func TestUpdatePanelHandlerEmptyName(t *testing.T) {
	h, s := newTestHandlers(t)
	panel, err := s.AddPanel(dto.CreatePanelRequest{Name: "Old", URL: "https://x:1", Token: "t"})
	if err != nil {
		t.Fatal(err)
	}

	for _, body := range []string{`{"name":""}`, `{"name":"   "}`} {
		w := doPutPanel(t, panelRouter(h), panel.ID, body)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body %s: status = %d, want 400", body, w.Code)
		}
	}

	// Имя не изменилось.
	got, err := s.GetPanel(panel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Old" {
		t.Fatalf("name changed: %q", got.Name)
	}
}

func TestUpdatePanelHandlerNotFound(t *testing.T) {
	h, _ := newTestHandlers(t)
	w := doPutPanel(t, panelRouter(h), "ghost", `{"name":"New"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
}

func TestUpdatePanelHandlerBadBody(t *testing.T) {
	h, s := newTestHandlers(t)
	panel, err := s.AddPanel(dto.CreatePanelRequest{Name: "Old", URL: "https://x:1", Token: "t"})
	if err != nil {
		t.Fatal(err)
	}

	w := doPutPanel(t, panelRouter(h), panel.ID, `{invalid json`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
}
