package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Handlers — HTTP transport: декодирует запрос, зовёт сервис, кодирует ответ.
type Handlers struct {
	auth          *TokenAuth
	panels        *PanelService
	subscriptions *SubscriptionService
	keySources    *KeySourceService
	sync          *SyncService
	tokens        *TokenService
	daemon        *DaemonService
}

// NewHandlers creates a new Handlers instance.
func NewHandlers(auth *TokenAuth, panels *PanelService, subscriptions *SubscriptionService,
	keySources *KeySourceService, sync *SyncService, tokens *TokenService, daemon *DaemonService) *Handlers {
	return &Handlers{
		auth:          auth,
		panels:        panels,
		subscriptions: subscriptions,
		keySources:    keySources,
		sync:          sync,
		tokens:        tokens,
		daemon:        daemon,
	}
}

// respondJSON writes a JSON response
func respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError writes an error JSON response
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}

// respondServiceError маппит ошибку сервиса в HTTP-ответ.
func respondServiceError(w http.ResponseWriter, err error) {
	var ae *AppError
	if errors.As(err, &ae) {
		respondError(w, ae.Status, ae.Message)
		return
	}
	switch {
	case errors.Is(err, ErrPanelNotFound), errors.Is(err, ErrKeySourceNotFound),
		errors.Is(err, ErrSubscriptionNotFound), errors.Is(err, ErrTokenNotFound),
		errors.Is(err, ErrClientNotFound), errors.Is(err, ErrInboundNotFound):
		respondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrPanelUnreachable):
		respondError(w, http.StatusBadGateway, "панель недоступна (таймаут 10 с)")
	case errors.Is(err, ErrInvalidSubscriptionName):
		respondError(w, http.StatusBadRequest, "имя может содержать только буквы, цифры, _ и -")
	default:
		log.Printf("internal error: %v", err)
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

// --- Panel Handlers ---

func (h *Handlers) ListPanels(w http.ResponseWriter, r *http.Request) {
	panels, err := h.panels.List()
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, panels)
}

func (h *Handlers) CreatePanel(w http.ResponseWriter, r *http.Request) {
	var req CreatePanelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	panel, err := h.panels.Create(req)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, panel)
}

func (h *Handlers) DeletePanel(w http.ResponseWriter, r *http.Request) {
	if err := h.panels.Delete(chi.URLParam(r, "id")); err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Client Handlers ---

func (h *Handlers) ListClients(w http.ResponseWriter, r *http.Request) {
	clients, err := h.panels.ListClients(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, clients)
}

func (h *Handlers) CreateClient(w http.ResponseWriter, r *http.Request) {
	var req CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.panels.CreateClient(chi.URLParam(r, "id"), req)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, resp)
}

func (h *Handlers) GetClientKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := h.panels.GetClientKeys(chi.URLParam(r, "id"), chi.URLParam(r, "email"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, keys)
}

func (h *Handlers) ListInbounds(w http.ResponseWriter, r *http.Request) {
	inbounds, err := h.panels.ListInbounds(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, inbounds)
}

func (h *Handlers) AttachInbound(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InboundID int `json:"inboundId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.panels.Attach(chi.URLParam(r, "id"), chi.URLParam(r, "email"), req.InboundID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handlers) DetachInbound(w http.ResponseWriter, r *http.Request) {
	var req struct {
		InboundID int `json:"inboundId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.panels.Detach(chi.URLParam(r, "id"), chi.URLParam(r, "email"), req.InboundID)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handlers) UpdateClient(w http.ResponseWriter, r *http.Request) {
	var req UpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.panels.UpdateClient(chi.URLParam(r, "id"), chi.URLParam(r, "email"), req)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// --- Subscription Handlers ---

func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := h.subscriptions.List()
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, subs)
}

func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.subscriptions.Create(req)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, resp)
}

func (h *Handlers) GetSubscription(w http.ResponseWriter, r *http.Request) {
	sub, err := h.subscriptions.Get(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, sub)
}

func (h *Handlers) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	var req UpdateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	res, err := h.subscriptions.Update(chi.URLParam(r, "id"), req)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	if res.Generate != nil {
		respondJSON(w, http.StatusOK, res.Generate)
		return
	}
	respondJSON(w, http.StatusOK, res.Subscription)
}

func (h *Handlers) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	if err := h.subscriptions.Delete(chi.URLParam(r, "id")); err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *Handlers) GetSubscriptionRaw(w http.ResponseWriter, r *http.Request) {
	content, err := h.subscriptions.Raw(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

func (h *Handlers) TestSubscription(w http.ResponseWriter, r *http.Request) {
	resp, err := h.subscriptions.Test(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handlers) RegenerateAllSubscriptions(w http.ResponseWriter, r *http.Request) {
	resp, err := h.subscriptions.RegenerateAll()
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// --- KeySource Handlers ---

func (h *Handlers) ListKeySources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.keySources.List()
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, sources)
}

func (h *Handlers) CreateKeySource(w http.ResponseWriter, r *http.Request) {
	var req CreateKeySourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.keySources.Create(req)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	status := http.StatusCreated
	if resp.Deduped {
		status = http.StatusOK
	}
	respondJSON(w, status, resp)
}

func (h *Handlers) GetKeySource(w http.ResponseWriter, r *http.Request) {
	ks, err := h.keySources.Get(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, ks)
}

func (h *Handlers) DeleteKeySource(w http.ResponseWriter, r *http.Request) {
	resp, err := h.keySources.Delete(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handlers) GetKeySourceKey(w http.ResponseWriter, r *http.Request) {
	resp, err := h.keySources.GetKey(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handlers) TestKeySource(w http.ResponseWriter, r *http.Request) {
	resp, err := h.keySources.Test(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handlers) GetKeySourceTraffic(w http.ResponseWriter, r *http.Request) {
	resp, err := h.keySources.Traffic(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// --- Sync ---

func (h *Handlers) SyncToAggregator(w http.ResponseWriter, r *http.Request) {
	resp, err := h.sync.Run(r.Context())
	if err != nil {
		var ae *AppError
		if errors.As(err, &ae) && resp.Status == "error" {
			respondJSON(w, ae.Status, resp)
			return
		}
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// --- Utility ---

func (h *Handlers) GetVlessSubTestStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, h.daemon.Status())
}

// --- Auth / Tokens ---

func (h *Handlers) GetAuthStatus(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]bool{"enabled": h.auth.Enabled()})
}

func (h *Handlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.tokens.List()
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, tokens)
}

func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	var req CreateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	resp, err := h.tokens.Create(req)
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, resp)
}

func (h *Handlers) DeleteToken(w http.ResponseWriter, r *http.Request) {
	resp, err := h.tokens.Delete(chi.URLParam(r, "id"))
	if err != nil {
		respondServiceError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}
