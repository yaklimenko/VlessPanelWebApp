package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Handlers groups all HTTP handlers
type Handlers struct {
	storage  *Storage
	panelAPI *PanelAPI
	config   Config
}

// NewHandlers creates a new Handlers instance
func NewHandlers(storage *Storage, panelAPI *PanelAPI, config Config) *Handlers {
	return &Handlers{
		storage:  storage,
		panelAPI: panelAPI,
		config:   config,
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

// --- Panel Handlers ---

// ListPanels returns all panels
func (h *Handlers) ListPanels(w http.ResponseWriter, r *http.Request) {
	panels, err := h.storage.LoadPanels()
	if err != nil {
		log.Printf("Error loading panels: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to load panels")
		return
	}

	respondJSON(w, http.StatusOK, panels)
}

// CreatePanel adds a new panel
func (h *Handlers) CreatePanel(w http.ResponseWriter, r *http.Request) {
	var req CreatePanelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" || req.URL == "" || req.Token == "" {
		respondError(w, http.StatusBadRequest, "name, url, and token are required")
		return
	}

	panel, err := h.storage.AddPanel(req)
	if err != nil {
		log.Printf("Error creating panel: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to create panel")
		return
	}

	respondJSON(w, http.StatusCreated, panel)
}

// DeletePanel removes a panel
func (h *Handlers) DeletePanel(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.storage.DeletePanel(id); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- Client Handlers ---

// ListClients returns all clients for a panel
func (h *Handlers) ListClients(w http.ResponseWriter, r *http.Request) {
	panelID := chi.URLParam(r, "id")

	panels, err := h.storage.LoadPanels()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load panels")
		return
	}

	var panel *Panel
	for i, p := range panels {
		if p.ID == panelID {
			panel = &panels[i]
			break
		}
	}

	if panel == nil {
		respondError(w, http.StatusNotFound, "Panel not found")
		return
	}

	clients, err := h.panelAPI.ListClients(*panel)
	if err != nil {
		log.Printf("Error listing clients for panel %s: %v", panelID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list clients: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, clients)
}

// CreateClient creates a new client on a panel
func (h *Handlers) CreateClient(w http.ResponseWriter, r *http.Request) {
	panelID := chi.URLParam(r, "id")

	var req CreateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Email == "" || req.InboundID == 0 {
		respondError(w, http.StatusBadRequest, "email and inboundId are required")
		return
	}

	panels, err := h.storage.LoadPanels()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load panels")
		return
	}

	var panel *Panel
	for i, p := range panels {
		if p.ID == panelID {
			panel = &panels[i]
			break
		}
	}

	if panel == nil {
		respondError(w, http.StatusNotFound, "Panel not found")
		return
	}

	if err := h.panelAPI.CreateClient(*panel, req.InboundID, req.Email); err != nil {
		log.Printf("Error creating client on panel %s: %v", panelID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create client: %v", err))
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"status": "created", "email": req.Email})
}

// GetClientKeys returns VLESS keys for a client
func (h *Handlers) GetClientKeys(w http.ResponseWriter, r *http.Request) {
	panelID := chi.URLParam(r, "id")
	email := chi.URLParam(r, "email")

	panels, err := h.storage.LoadPanels()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load panels")
		return
	}

	var panel *Panel
	for i, p := range panels {
		if p.ID == panelID {
			panel = &panels[i]
			break
		}
	}

	if panel == nil {
		respondError(w, http.StatusNotFound, "Panel not found")
		return
	}

	keys, err := h.panelAPI.GetClientKeys(*panel, email)
	if err != nil {
		log.Printf("Error getting keys for client %s on panel %s: %v", email, panelID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to get keys: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, keys)
}

// ListInbounds returns all inbounds for a panel
func (h *Handlers) ListInbounds(w http.ResponseWriter, r *http.Request) {
	panelID := chi.URLParam(r, "id")

	panels, err := h.storage.LoadPanels()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load panels")
		return
	}

	var panel *Panel
	for i, p := range panels {
		if p.ID == panelID {
			panel = &panels[i]
			break
		}
	}

	if panel == nil {
		respondError(w, http.StatusNotFound, "Panel not found")
		return
	}

	inbounds, err := h.panelAPI.ListInbounds(*panel)
	if err != nil {
		log.Printf("Error listing inbounds for panel %s: %v", panelID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to list inbounds: %v", err))
		return
	}

	// Return simplified inbounds
	type SimpleInbound struct {
		ID       int    `json:"id"`
		Remark   string `json:"remark"`
		Port     int    `json:"port"`
		Protocol string `json:"protocol"`
	}

	simple := make([]SimpleInbound, 0, len(inbounds))
	for _, ib := range inbounds {
		simple = append(simple, SimpleInbound{
			ID:       ib.ID,
			Remark:   ib.Remark,
			Port:     ib.Port,
			Protocol: ib.Protocol,
		})
	}

	respondJSON(w, http.StatusOK, simple)
}

// --- Subscription Handlers ---

// ListSubscriptions returns all subscriptions
func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := h.storage.ListSubscriptions()
	if err != nil {
		log.Printf("Error listing subscriptions: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to list subscriptions")
		return
	}

	respondJSON(w, http.StatusOK, subs)
}

// CreateSubscription creates a new subscription
func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}

	sub, err := h.storage.CreateSubscription(req)
	if err != nil {
		log.Printf("Error creating subscription: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to create subscription")
		return
	}

	respondJSON(w, http.StatusCreated, sub)
}

// GetSubscription returns a single subscription
func (h *Handlers) GetSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sub, err := h.storage.GetSubscription(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, sub)
}

// UpdateSubscription updates a subscription
func (h *Handlers) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	sub, err := h.storage.UpdateSubscription(id, req)
	if err != nil {
		log.Printf("Error updating subscription: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to update subscription")
		return
	}

	respondJSON(w, http.StatusOK, sub)
}

// DeleteSubscription removes a subscription
func (h *Handlers) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.storage.DeleteSubscription(id); err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// GetSubscriptionRaw returns raw file content
func (h *Handlers) GetSubscriptionRaw(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	content, err := h.storage.GetSubscriptionRaw(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
}

// TestSubscription runs VlessSubTest on a subscription
func (h *Handlers) TestSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sub, err := h.storage.GetSubscription(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	if len(sub.Keys) == 0 {
		respondError(w, http.StatusBadRequest, "Subscription has no keys to test")
		return
	}

	// Build the URL parameter for vlesssubtest
	// Format: url=vless://...|vless://...|vless://...
	urls := make([]string, 0, len(sub.Keys))
	for _, k := range sub.Keys {
		urls = append(urls, k.Link)
	}
	urlParam := strings.Join(urls, "|")

	cmd := exec.Command(h.config.VlessSubTestPath, "url="+urlParam)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("VlessSubTest error: %v, output: %s", err, string(output))
	}

	result := strings.TrimSpace(string(output))
	if result == "" {
		result = "VlessSubTest completed with no output"
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"result":   result,
		"status":   "ok",
		"original": strings.Join(urls, "\n"),
	})
}

// GetVlessSubTestStatus returns the status of VlessSubTest binary
func (h *Handlers) GetVlessSubTestStatus(w http.ResponseWriter, r *http.Request) {
	cmd := exec.Command(h.config.VlessSubTestPath, "--help")
	output, err := cmd.CombinedOutput()

	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"path":      h.config.VlessSubTestPath,
			"error":     err.Error(),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"available": true,
		"path":      h.config.VlessSubTestPath,
		"help":      strings.TrimSpace(string(output)),
	})
}

// sanitizeFilename ensures a safe filename
func sanitizeFilename(name string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	return reg.ReplaceAllString(name, "_")
}

// ensure interfaces
var _ http.Handler = chi.NewRouter()
