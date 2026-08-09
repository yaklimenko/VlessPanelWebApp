package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

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

	if err := h.panelAPI.CreateClient(*panel, req.InboundID, req.Email, req.ExpiryDate); err != nil {
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
		Enable   bool   `json:"enable"`
	}

	simple := make([]SimpleInbound, 0, len(inbounds))
	for _, ib := range inbounds {
		simple = append(simple, SimpleInbound{
			ID:       ib.ID,
			Remark:   ib.Remark,
			Port:     ib.Port,
			Protocol: ib.Protocol,
			Enable:   ib.Enable,
		})
	}

	respondJSON(w, http.StatusOK, simple)
}

// AttachInbound attaches an existing client to an additional inbound
func (h *Handlers) AttachInbound(w http.ResponseWriter, r *http.Request) {
	panelID := chi.URLParam(r, "id")
	email := chi.URLParam(r, "email")

	var req struct {
		InboundID int `json:"inboundId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.InboundID == 0 {
		respondError(w, http.StatusBadRequest, "inboundId is required")
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

	if err := h.panelAPI.AttachClient(*panel, email, req.InboundID); err != nil {
		log.Printf("Error attaching client %s to inbound %d on panel %s: %v", email, req.InboundID, panelID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to attach inbound: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "attached", "email": email})
}

// DetachInbound detaches a client from an inbound
func (h *Handlers) DetachInbound(w http.ResponseWriter, r *http.Request) {
	panelID := chi.URLParam(r, "id")
	email := chi.URLParam(r, "email")

	var req struct {
		InboundID int `json:"inboundId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.InboundID == 0 {
		respondError(w, http.StatusBadRequest, "inboundId is required")
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

	if err := h.panelAPI.DetachClient(*panel, email, req.InboundID); err != nil {
		log.Printf("Error detaching client %s from inbound %d on panel %s: %v", email, req.InboundID, panelID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to detach inbound: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "detached", "email": email})
}

// UpdateClient updates client fields (expiry date, etc.)
func (h *Handlers) UpdateClient(w http.ResponseWriter, r *http.Request) {
	panelID := chi.URLParam(r, "id")
	email := chi.URLParam(r, "email")

	var req UpdateClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
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

	var expiryTime int64
	if req.ExpiryDate != "" {
		t, err := time.Parse("2006-01-02", req.ExpiryDate)
		if err != nil {
			respondError(w, http.StatusBadRequest, "Invalid expiryDate format (expected YYYY-MM-DD)")
			return
		}
		expiryTime = t.Unix() * 1000
	}

	if err := h.panelAPI.UpdateClient(*panel, email, expiryTime); err != nil {
		log.Printf("Error updating client %s on panel %s: %v", email, panelID, err)
		respondError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to update client: %v", err))
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "updated", "email": email})
}

// --- Subscription Handlers ---

// ListSubscriptions returns all subscriptions (with KeySource refs and sync status)
func (h *Handlers) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	subs, err := h.storage.ListSubscriptions()
	if err != nil {
		log.Printf("Error listing subscriptions: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to list subscriptions")
		return
	}

	// Enrich sync status concurrently (each active sub = one HEAD to aggregator).
	var wg sync.WaitGroup
	for i := range subs {
		if subs[i].Status == "active" {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
			h.enrichSync(&subs[i])
			}(i)
		}
	}
	wg.Wait()

	respondJSON(w, http.StatusOK, subs)
}

// CreateSubscription creates a new subscription from KeySources (or legacy keys)
// and generates the file. Expired/missing/unreachable KeySources are skipped and
// reported. Returns partial-success report.
func (h *Handlers) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	var req CreateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		respondError(w, http.StatusBadRequest, "name is required")
		return
	}
	if !validSubscriptionName(name) {
		respondError(w, http.StatusBadRequest, "имя может содержать только буквы, цифры, _ и -")
		return
	}

	// Duplicate name check (case-insensitive, both files and metadata).
	if h.subscriptionNameTaken(name) {
		respondError(w, http.StatusConflict, fmt.Sprintf("подписка с именем «%s» уже существует", name))
		return
	}

	report := &GenerationReport{Items: []GenerationReportItem{}}

	// Resolve keys: from keySourceIds (new mode) or legacy keys array.
	// Manual sources are included always (per spec: «manual — включается всегда»).
	var keys []SubKey
	if len(req.KeySourceIDs) > 0 {
		var err error
		keys, err = h.resolveKeySources(req.KeySourceIDs, report, true)
		if err != nil {
			log.Printf("Error resolving key sources for subscription %s: %v", name, err)
			respondError(w, http.StatusInternalServerError, "Не удалось извлечь ключи: "+err.Error())
			return
		}
	} else if len(req.Keys) > 0 {
		keys = req.Keys
		report.Items = append(report.Items, GenerationReportItem{Kind: "manual", Label: "manual (legacy)"})
	}

	status := "active"
	if len(keys) == 0 {
		status = "draft"
	}
	if status == "active" {
		if err := h.storage.WriteSubscriptionFile(name, keys); err != nil {
			log.Printf("Error writing subscription file: %v", err)
			respondError(w, http.StatusInternalServerError, "Failed to write subscription file")
			return
		}
	}

	now := nowStr()
	meta := Subscription{
		ID:        name,
		Name:      name,
		Status:    status,
		Keys:      keys,
		UpdatedAt: now,
	}
	if err := h.storage.UpsertSubMeta(meta); err != nil {
		log.Printf("Error saving subscription meta: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to save subscription")
		return
	}

	meta.FileMtime = h.storage.SubscriptionFileMtime(name)
	h.enrichSync(&meta)

	report.Included = countKind(report.Items, "ok") + countKind(report.Items, "manual")
	report.Skipped = countKind(report.Items, "skip")

	respondJSON(w, http.StatusCreated, SubscriptionGenerateResponse{
		Subscription: meta,
		Report:       report.Items,
		Included:     report.Included,
		Skipped:      report.Skipped,
	})
}

// GetSubscription returns a single subscription
func (h *Handlers) GetSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sub, err := h.storage.GetSubscription(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	h.enrichSync(sub)
	respondJSON(w, http.StatusOK, sub)
}

// UpdateSubscription updates a subscription.
// Modes: regenerate (fresh keys from KeySources), addKeySourceIds (append refs),
// removeKeyId (delete one key + rewrite file), legacy keys array (full replace),
// name rename.
func (h *Handlers) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateSubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	sub, err := h.storage.GetSubscription(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// Ensure metadata exists (creates it for legacy file-only subscriptions).
	meta := *sub
	if meta.ID == "" {
		meta.ID = id
	}
	meta.Name = id

	// ─── Rename ───
	if req.Name != "" && req.Name != id {
		if !validSubscriptionName(req.Name) {
			respondError(w, http.StatusBadRequest, "имя может содержать только буквы, цифры, _ и -")
			return
		}
		if h.subscriptionNameTaken(req.Name) {
			respondError(w, http.StatusConflict, fmt.Sprintf("подписка с именем «%s» уже существует", req.Name))
			return
		}
		if h.storage.SubscriptionFileExists(id) {
			if err := h.storage.RenameSubscriptionFile(id, req.Name); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		if err := h.storage.DeleteSubMeta(id); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		meta.Name = req.Name
		meta.ID = req.Name
		meta.UpdatedAt = nowStr()
		if err := h.storage.UpsertSubMeta(meta); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		meta.FileMtime = h.storage.SubscriptionFileMtime(meta.Name)
		h.enrichSync(&meta)
		respondJSON(w, http.StatusOK, meta)
		return
	}

	// ─── Regenerate: re-fetch fresh keys from KeySources, keep manual ───
	if req.Regenerate {
		report := &GenerationReport{Items: []GenerationReportItem{}}
		keys, err := h.regenerateKeys(meta.Keys, report)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Не удалось перегенерировать: "+err.Error())
			return
		}

		meta.Keys = keys
		meta.UpdatedAt = nowStr()
		if len(keys) == 0 {
			meta.Status = "draft"
			_ = h.storage.RemoveSubscriptionFile(meta.Name)
		} else {
			meta.Status = "active"
			if err := h.storage.WriteSubscriptionFile(meta.Name, keys); err != nil {
				respondError(w, http.StatusInternalServerError, "Failed to write subscription file")
				return
			}
		}
		if err := h.storage.UpsertSubMeta(meta); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		report.Included = countKind(report.Items, "ok") + countKind(report.Items, "manual")
		report.Skipped = countKind(report.Items, "skip")
		meta.FileMtime = h.storage.SubscriptionFileMtime(meta.Name)
		h.enrichSync(&meta)
		respondJSON(w, http.StatusOK, SubscriptionGenerateResponse{
			Subscription: meta,
			Report:       report.Items,
			Included:     report.Included,
			Skipped:      report.Skipped,
		})
		return
	}

	// ─── Add KeySource refs (no file write → badge «требуется синк») ───
	if len(req.AddKeySourceIDs) > 0 {
		for _, ksID := range req.AddKeySourceIDs {
			if keySourceInKeys(meta.Keys, ksID) {
				continue // already added
			}
			if _, err := h.storage.GetKeySource(ksID); err != nil {
				respondError(w, http.StatusBadRequest, "KeySource не найден: "+ksID)
				return
			}
			sid := ksID
			meta.Keys = append(meta.Keys, SubKey{ID: "k-" + randID(), KeySourceID: &sid})
		}
		meta.UpdatedAt = nowStr()
		if err := h.storage.UpsertSubMeta(meta); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		meta.FileMtime = h.storage.SubscriptionFileMtime(meta.Name)
		h.enrichSync(&meta)
		respondJSON(w, http.StatusOK, meta)
		return
	}

	// ─── Remove one key + rewrite file ───
	if req.RemoveKeyID != "" {
		newKeys := make([]SubKey, 0, len(meta.Keys))
		for _, k := range meta.Keys {
			if k.ID != req.RemoveKeyID {
				newKeys = append(newKeys, k)
			}
		}
		meta.Keys = newKeys
		meta.UpdatedAt = nowStr()
		if len(newKeys) == 0 {
			meta.Status = "draft"
			_ = h.storage.RemoveSubscriptionFile(meta.Name)
		} else {
			meta.Status = "active"
			if err := h.storage.WriteSubscriptionFile(meta.Name, newKeys); err != nil {
				respondError(w, http.StatusInternalServerError, "Failed to write subscription file")
				return
			}
		}
		if err := h.storage.UpsertSubMeta(meta); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		meta.FileMtime = h.storage.SubscriptionFileMtime(meta.Name)
		h.enrichSync(&meta)
		respondJSON(w, http.StatusOK, meta)
		return
	}

	// ─── Legacy mode: full key replacement ───
	if req.Keys != nil {
		meta.Keys = req.Keys
		meta.UpdatedAt = nowStr()
		if len(req.Keys) == 0 {
			meta.Status = "draft"
			_ = h.storage.RemoveSubscriptionFile(meta.Name)
		} else {
			meta.Status = "active"
			if err := h.storage.WriteSubscriptionFile(meta.Name, req.Keys); err != nil {
				respondError(w, http.StatusInternalServerError, "Failed to write subscription file")
				return
			}
		}
		if err := h.storage.UpsertSubMeta(meta); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		meta.FileMtime = h.storage.SubscriptionFileMtime(meta.Name)
		h.enrichSync(&meta)
		respondJSON(w, http.StatusOK, meta)
		return
	}

	// No-op
	meta.FileMtime = h.storage.SubscriptionFileMtime(meta.Name)
	h.enrichSync(&meta)
	respondJSON(w, http.StatusOK, meta)
}

// DeleteSubscription removes a subscription (file + metadata)
func (h *Handlers) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if !h.storage.SubscriptionFileExists(id) {
		if _, ok := h.storage.GetSubMeta(id); !ok {
			respondError(w, http.StatusNotFound, fmt.Sprintf("subscription %s not found", id))
			return
		}
	}

	if err := h.storage.RemoveSubscriptionFile(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.storage.DeleteSubMeta(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
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

// TestSubscription runs VlessSubTest daemon on a subscription
func (h *Handlers) TestSubscription(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	sub, err := h.storage.GetSubscription(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	// Only test keys with actual links (draft/ungenerated panel keys are skipped).
	var testKeys []SubKey
	for _, k := range sub.Keys {
		if strings.TrimSpace(k.Link) != "" {
			testKeys = append(testKeys, k)
		}
	}
	if len(testKeys) == 0 {
		respondError(w, http.StatusBadRequest, "Subscription has no keys to test")
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	daemonURL := strings.TrimRight(h.config.VlessSubTestDaemonURL, "/") + "/test-single"

	var results []TestSingleResponse
	okCount := 0

	for i, key := range testKeys {
		reqBody, _ := json.Marshal(TestSingleRequest{
			Vless:   key.Link,
			Timeout: 10,
		})

		resp, err := client.Post(daemonURL, "application/json", bytes.NewReader(reqBody))
		if err != nil {
			results = append(results, TestSingleResponse{
				KeyIdx: i,
				Status: "ERROR",
				IP:     err.Error(),
			})
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var single TestSingleResponse
		if err := json.Unmarshal(body, &single); err != nil {
			results = append(results, TestSingleResponse{
				KeyIdx: i,
				Status: "ERROR",
				IP:     "failed to parse response",
			})
			continue
		}

		single.KeyIdx = i
		if single.Status == "OK" {
			okCount++
		}
		results = append(results, single)
	}

	respondJSON(w, http.StatusOK, TestSubscriptionResponse{
		Total:   len(testKeys),
		OK:      okCount,
		Results: results,
	})
}

// GetVlessSubTestStatus returns the status of VlessSubTest daemon
func (h *Handlers) GetVlessSubTestStatus(w http.ResponseWriter, r *http.Request) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(strings.TrimRight(h.config.VlessSubTestDaemonURL, "/") + "/test")

	if err != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"available": false,
			"daemonURL": h.config.VlessSubTestDaemonURL,
			"error":     err.Error(),
		})
		return
	}
	resp.Body.Close()

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"available": true,
		"daemonURL": h.config.VlessSubTestDaemonURL,
	})
}

// ─── KeySource Handlers ───

// panelSnapshot caches clients of one panel for a request batch.
type panelSnapshot struct {
	clients  []Client
	inbounds []XUIInbound
	err      error
}

// ListKeySources returns all KeySources with derived statuses, caches, traffic.
func (h *Handlers) ListKeySources(w http.ResponseWriter, r *http.Request) {
	sources, err := h.storage.LoadKeySources()
	if err != nil {
		log.Printf("Error loading key sources: %v", err)
		respondError(w, http.StatusInternalServerError, "Failed to load key sources")
		return
	}

	panels, _ := h.storage.LoadPanels()
	panelMap := make(map[string]Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}

	// Usage count per KeySource across subscriptions.
	usage := map[string]int{}
	if subs, err := h.storage.ListSubscriptions(); err == nil {
		for _, s := range subs {
			for _, k := range s.Keys {
				if k.KeySourceID != nil {
					usage[*k.KeySourceID]++
				}
			}
		}
	}

	snapshot := map[string]*panelSnapshot{}
	resp := make([]KeySource, 0, len(sources))
	for i := range sources {
		ks := sources[i]
		h.enrichKeySource(&ks, panelMap, snapshot, usage)
		resp = append(resp, ks)
	}

	respondJSON(w, http.StatusOK, resp)
}

// CreateKeySource creates a KeySource (panel triplet or manual vless link),
// with dedup within the same type.
func (h *Handlers) CreateKeySource(w http.ResponseWriter, r *http.Request) {
	var req CreateKeySourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	switch req.Type {
	case "panel":
		if req.PanelID == "" || req.ClientEmail == "" || req.InboundID == 0 {
			respondError(w, http.StatusBadRequest, "panelId, clientEmail и inboundId обязательны для type=panel")
			return
		}
	case "manual":
		req.VlessLink = strings.TrimSpace(req.VlessLink)
		if !strings.HasPrefix(req.VlessLink, "vless://") {
			respondError(w, http.StatusBadRequest, "vlessLink должен начинаться с vless://")
			return
		}
	default:
		respondError(w, http.StatusBadRequest, "type должен быть panel или manual")
		return
	}

	// Dedup within type.
	var existing *KeySource
	if req.Type == "panel" {
		existing = h.storage.FindDuplicatePanel(req.PanelID, req.ClientEmail, req.InboundID)
	} else {
		existing = h.storage.FindDuplicateManual(req.VlessLink)
	}
	if existing != nil {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"keySource": existing,
			"deduped":   true,
		})
		return
	}

	now := nowStr()
	ks := KeySource{
		ID:          "ks-" + randID(),
		Type:        req.Type,
		PanelID:     req.PanelID,
		ClientEmail: req.ClientEmail,
		InboundID:   req.InboundID,
		VlessLink:   req.VlessLink,
		Label:       req.Label,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if ks.Type == "manual" && ks.Label == "" {
		ks.Label = labelFromVless(ks.VlessLink)
	}

	sources, err := h.storage.LoadKeySources()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load key sources")
		return
	}
	sources = append(sources, ks)
	if err := h.storage.SaveKeySources(sources); err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to save key source")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"keySource": ks,
		"deduped":   false,
	})
}

// GetKeySource returns a single KeySource with derived status.
func (h *Handlers) GetKeySource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ks, err := h.storage.GetKeySource(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	panels, _ := h.storage.LoadPanels()
	panelMap := make(map[string]Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}
	h.enrichKeySource(ks, panelMap, map[string]*panelSnapshot{}, map[string]int{})
	respondJSON(w, http.StatusOK, ks)
}

// DeleteKeySource removes a KeySource; SubKeys referencing it are removed from
// subscriptions (and their files rewritten). Reports usage count.
func (h *Handlers) DeleteKeySource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ks, err := h.storage.GetKeySource(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	subs, err := h.storage.ListSubscriptions()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load subscriptions")
		return
	}

	affected := []string{}
	for i := range subs {
		sub := subs[i]
		removed := false
		newKeys := make([]SubKey, 0, len(sub.Keys))
		for _, k := range sub.Keys {
			if k.KeySourceID != nil && *k.KeySourceID == id {
				removed = true
				continue
			}
			newKeys = append(newKeys, k)
		}
		if !removed {
			continue
		}

		sub.Keys = newKeys
		sub.UpdatedAt = nowStr()
		if len(newKeys) == 0 {
			sub.Status = "draft"
			_ = h.storage.RemoveSubscriptionFile(sub.Name)
		} else {
			sub.Status = "active"
			if err := h.storage.WriteSubscriptionFile(sub.Name, newKeys); err != nil {
				log.Printf("DeleteKeySource: rewriting file for %s: %v", sub.Name, err)
			}
		}
		if err := h.storage.UpsertSubMeta(sub); err != nil {
			log.Printf("DeleteKeySource: upsert meta for %s: %v", sub.Name, err)
		}
		affected = append(affected, sub.Name)
	}

	if err := h.storage.DeleteKeySource(id); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	label := ks.Label
	if label == "" {
		label = id
	}
	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status":               "deleted",
		"label":                label,
		"usedInSubscriptions":  len(affected),
		"subscriptions":        affected,
	})
}

// GetKeySourceKey returns a fresh key from the panel (or the stored manual
// link), updating the cache. Force refresh is the default behaviour.
func (h *Handlers) GetKeySourceKey(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ks, err := h.storage.GetKeySource(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	if ks.Type == "manual" {
		respondJSON(w, http.StatusOK, map[string]interface{}{
			"key": VLESSKey{Label: ks.Label, Link: ks.VlessLink},
		})
		return
	}

	panels, err := h.storage.LoadPanels()
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load panels")
		return
	}
	var panel *Panel
	for i := range panels {
		if panels[i].ID == ks.PanelID {
			panel = &panels[i]
			break
		}
	}
	if panel == nil {
		respondError(w, http.StatusNotFound, "панель KeySource не найдена (удалена?)")
		return
	}

	key, err := h.panelAPI.GetClientKeyForInbound(*panel, ks.ClientEmail, ks.InboundID)
	if err != nil {
		log.Printf("GetKeySourceKey %s: %v", id, err)
		msg := err.Error()
		if strings.Contains(msg, "Client.Timeout") || strings.Contains(msg, "context deadline") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "EOF") {
			respondError(w, http.StatusBadGateway, "панель недоступна (таймаут 10 с)")
		} else {
			respondError(w, http.StatusNotFound, msg)
		}
		return
	}

	ks.CachedKey = &CachedKey{Link: key.Link, FetchedAt: nowStr()}
	ks.UpdatedAt = nowStr()
	if err := h.storage.UpdateKeySource(*ks); err != nil {
		log.Printf("GetKeySourceKey: cache update failed: %v", err)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"key": key, "source": ks})
}

// TestKeySource runs a test-single against the daemon and stores lastTest.
func (h *Handlers) TestKeySource(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ks, err := h.storage.GetKeySource(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	link := ""
	if ks.Type == "manual" {
		link = ks.VlessLink
	} else {
		// Fetch fresh key so the test reflects reality.
		panels, _ := h.storage.LoadPanels()
		var panel *Panel
		for i := range panels {
			if panels[i].ID == ks.PanelID {
				panel = &panels[i]
				break
			}
		}
		if panel == nil {
			respondError(w, http.StatusNotFound, "панель KeySource не найдена (удалена?)")
			return
		}
		key, err := h.panelAPI.GetClientKeyForInbound(*panel, ks.ClientEmail, ks.InboundID)
		if err != nil {
			lastTest := &KeySourceTest{Status: "fail", At: nowStr(), Error: err.Error()}
			ks.LastTest = lastTest
			ks.UpdatedAt = nowStr()
			_ = h.storage.UpdateKeySource(*ks)
			respondError(w, http.StatusBadGateway, "не удалось получить ключ: "+err.Error())
			return
		}
		link = key.Link
		ks.CachedKey = &CachedKey{Link: key.Link, FetchedAt: nowStr()}
	}

	if strings.TrimSpace(link) == "" {
		respondError(w, http.StatusBadRequest, "у KeySource нет ключа для теста")
		return
	}

	client := &http.Client{Timeout: 30 * time.Second}
	daemonURL := strings.TrimRight(h.config.VlessSubTestDaemonURL, "/") + "/test-single"
	reqBody, _ := json.Marshal(TestSingleRequest{Vless: link, Timeout: 10})

	start := time.Now()
	resp, err := client.Post(daemonURL, "application/json", bytes.NewReader(reqBody))
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		lastTest := &KeySourceTest{Status: "fail", At: nowStr(), Error: "демон тестов недоступен: " + err.Error()}
		ks.LastTest = lastTest
		ks.UpdatedAt = nowStr()
		_ = h.storage.UpdateKeySource(*ks)
		respondJSON(w, http.StatusOK, map[string]interface{}{"result": nil, "lastTest": lastTest, "error": lastTest.Error})
		return
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var single TestSingleResponse
	if err := json.Unmarshal(body, &single); err != nil {
		lastTest := &KeySourceTest{Status: "fail", At: nowStr(), Error: "не удалось разобрать ответ демона"}
		ks.LastTest = lastTest
		ks.UpdatedAt = nowStr()
		_ = h.storage.UpdateKeySource(*ks)
		respondJSON(w, http.StatusOK, map[string]interface{}{"result": nil, "lastTest": lastTest, "error": lastTest.Error})
		return
	}

	status := "fail"
	if single.Status == "OK" {
		status = "ok"
	}
	lastTest := &KeySourceTest{Status: status, At: nowStr(), Ms: ms}
	if status == "fail" {
		lastTest.Error = "ключ не прошёл тест демона"
	}
	ks.LastTest = lastTest
	ks.UpdatedAt = nowStr()
	_ = h.storage.UpdateKeySource(*ks)

	respondJSON(w, http.StatusOK, map[string]interface{}{"result": single, "lastTest": lastTest})
}

// GetKeySourceTraffic returns up/down stats for a panel KeySource.
func (h *Handlers) GetKeySourceTraffic(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ks, err := h.storage.GetKeySource(id)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}
	if ks.Type == "manual" {
		respondError(w, http.StatusBadRequest, "manual KeySource не имеет трафика")
		return
	}

	panels, _ := h.storage.LoadPanels()
	var panel *Panel
	for i := range panels {
		if panels[i].ID == ks.PanelID {
			panel = &panels[i]
			break
		}
	}
	if panel == nil {
		respondError(w, http.StatusNotFound, "панель KeySource не найдена (удалена?)")
		return
	}

	client, err := h.panelAPI.GetClientStats(*panel, ks.ClientEmail)
	if err != nil {
		respondError(w, http.StatusNotFound, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"up":         client.Up,
		"down":       client.Down,
		"expiryTime": client.ExpiryTime,
		"enable":     client.Enable,
	})
}

// enrichKeySource fills derived fields (status, traffic, names) for a KeySource.
func (h *Handlers) enrichKeySource(ks *KeySource, panelMap map[string]Panel, snapshot map[string]*panelSnapshot, usage map[string]int) {
	ks.UsedInSubs = usage[ks.ID]

	if ks.Type == "manual" {
		ks.Status = "manual"
		ks.KeyAvailable = strings.TrimSpace(ks.VlessLink) != ""
		return
	}

	panel, ok := panelMap[ks.PanelID]
	if !ok {
		ks.Status = "missing"
		ks.Error = "панель удалена — ключ не извлечётся"
		return
	}
	ks.PanelName = panel.Name

	snap := snapshot[ks.PanelID]
	if snap == nil {
		clients, cerr := h.panelAPI.ListClients(panel)
		inbounds, ierr := h.panelAPI.ListInbounds(panel)
		var snapErr error
		if cerr != nil {
			snapErr = cerr
		} else if ierr != nil {
			snapErr = ierr
		}
		snap = &panelSnapshot{clients: clients, inbounds: inbounds, err: snapErr}
		snapshot[ks.PanelID] = snap
	}

	if snap.err != nil {
		ks.Status = "panel_unreachable"
		ks.Error = "панель недоступна (таймаут 10 с)"
		return
	}

	// Inbound remark/port.
	for _, ib := range snap.inbounds {
		if ib.ID == ks.InboundID {
			ks.InboundRemark = ib.Remark
			ks.InboundPort = ib.Port
			break
		}
	}

	var client *Client
	for i := range snap.clients {
		if snap.clients[i].Email == ks.ClientEmail {
			client = &snap.clients[i]
			break
		}
	}
	if client == nil {
		ks.Status = "missing"
		ks.Error = "клиент не найден на панели — ключ не извлечётся"
		return
	}
	ks.ClientEnabled = client.Enable

	if client.ExpiryTime > 0 {
		ks.ExpiryTime = client.ExpiryTime
		ks.ExpireDate = time.UnixMilli(client.ExpiryTime).UTC().Format("2006-01-02")
		if time.Now().UnixMilli() > client.ExpiryTime {
			ks.Status = "expired"
			ks.Error = "срок действия ключа истёк — не включается при генерации"
			return
		}
	}

	attached := false
	for _, id := range client.InboundIDs {
		if id == ks.InboundID {
			attached = true
			break
		}
	}
	if !attached {
		ks.Status = "missing"
		ks.Error = "инбаунд не найден у клиента — ключ не извлечётся"
		return
	}

	ks.Status = "ok"
	ks.Traffic = &KeySourceTraffic{Up: client.Up, Down: client.Down}
	ks.KeyAvailable = ks.CachedKey != nil && ks.CachedKey.Link != ""
}

// resolveKeySources fetches fresh keys for a list of KeySource IDs.
// includeManual=false → manual sources are skipped (creation flow);
// includeManual=true → manual sources included (regeneration keeps them).
func (h *Handlers) resolveKeySources(ids []string, report *GenerationReport, includeManual bool) ([]SubKey, error) {
	panels, _ := h.storage.LoadPanels()
	panelMap := make(map[string]Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}

	keys := []SubKey{}
	for _, id := range ids {
		ks, err := h.storage.GetKeySource(id)
		if err != nil {
			report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: id, Why: "KeySource не найден"})
			continue
		}

		label := ksLabelFor(ks, panelMap)

		if ks.Type == "manual" {
			if !includeManual {
				report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: "manual-ключи добавляются отдельно"})
				continue
			}
			keys = append(keys, SubKey{ID: "k-" + randID(), Link: ks.VlessLink, KeySourceID: strPtr(ks.ID)})
			report.Items = append(report.Items, GenerationReportItem{Kind: "manual", Label: label})
			continue
		}

		panel, ok := panelMap[ks.PanelID]
		if !ok {
			report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: "панель удалена"})
			continue
		}

		start := time.Now()
		key, err := h.panelAPI.GetClientKeyForInbound(panel, ks.ClientEmail, ks.InboundID)
		ms := int(time.Since(start).Milliseconds())

		if err != nil {
			msg := err.Error()
			if strings.Contains(msg, "клиент") && strings.Contains(msg, "не найден") {
				report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: "клиент не найден на панели — пропущен"})
			} else if strings.Contains(msg, "инбаунд") && strings.Contains(msg, "не найден") {
				report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: "инбаунд не найден на панели — пропущен"})
			} else if strings.Contains(msg, "Client.Timeout") || strings.Contains(msg, "context deadline") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "EOF") || strings.Contains(msg, "tls") {
				report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: "панель недоступна (таймаут 10 с)"})
			} else {
				report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: msg})
			}
			continue
		}

		// Update cache for display.
		ks.CachedKey = &CachedKey{Link: key.Link, FetchedAt: nowStr()}
		ks.UpdatedAt = nowStr()
		_ = h.storage.UpdateKeySource(*ks)

		keys = append(keys, SubKey{ID: "k-" + randID(), Link: key.Link, KeySourceID: strPtr(ks.ID)})
		report.Items = append(report.Items, GenerationReportItem{Kind: "ok", Label: label, Ms: ms})
	}
	return keys, nil
}

// regenerateKeys refreshes panel keys of a subscription, keeping manual keys.
// Skipped panel keys are dropped from the subscription (reported).
func (h *Handlers) regenerateKeys(keys []SubKey, report *GenerationReport) ([]SubKey, error) {
	panels, _ := h.storage.LoadPanels()
	panelMap := make(map[string]Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}

	out := []SubKey{}
	for _, k := range keys {
		if k.KeySourceID == nil {
			out = append(out, k)
			report.Items = append(report.Items, GenerationReportItem{Kind: "manual", Label: "manual (legacy)"})
			continue
		}

		ks, err := h.storage.GetKeySource(*k.KeySourceID)
		if err != nil {
			report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: *k.KeySourceID, Why: "KeySource не найден — ключ удалён"})
			continue
		}
		label := ksLabelFor(ks, panelMap)

		if ks.Type == "manual" {
			out = append(out, SubKey{ID: k.ID, Link: ks.VlessLink, KeySourceID: k.KeySourceID})
			report.Items = append(report.Items, GenerationReportItem{Kind: "manual", Label: label})
			continue
		}

		panel, ok := panelMap[ks.PanelID]
		if !ok {
			report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: "панель удалена — ключ удалён"})
			continue
		}

		start := time.Now()
		key, err := h.panelAPI.GetClientKeyForInbound(panel, ks.ClientEmail, ks.InboundID)
		ms := int(time.Since(start).Milliseconds())
		if err != nil {
			msg := err.Error()
			why := msg
			if strings.Contains(msg, "клиент") && strings.Contains(msg, "не найден") {
				why = "клиент не найден на панели — не включён"
			} else if strings.Contains(msg, "инбаунд") && strings.Contains(msg, "не найден") {
				why = "инбаунд не найден на панели — не включён"
			} else if strings.Contains(msg, "Client.Timeout") || strings.Contains(msg, "context deadline") || strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "EOF") || strings.Contains(msg, "tls") {
				why = "панель недоступна (таймаут 10 с)"
			}
			report.Items = append(report.Items, GenerationReportItem{Kind: "skip", Label: label, Why: why})
			continue
		}

		ks.CachedKey = &CachedKey{Link: key.Link, FetchedAt: nowStr()}
		ks.UpdatedAt = nowStr()
		_ = h.storage.UpdateKeySource(*ks)

		out = append(out, SubKey{ID: k.ID, Link: key.Link, KeySourceID: k.KeySourceID})
		report.Items = append(report.Items, GenerationReportItem{Kind: "ok", Label: label, Ms: ms})
	}
	return out, nil
}

// ─── Sync ───

// SyncToAggregator runs the rsync script (same mechanism as before).
func (h *Handlers) SyncToAggregator(w http.ResponseWriter, r *http.Request) {
	script := h.config.SyncScript
	if _, err := os.Stat(script); err != nil {
		respondError(w, http.StatusNotImplemented, "sync script not found: "+script)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("SyncToAggregator failed: %v\n%s", err, string(out))
		respondJSON(w, http.StatusBadGateway, map[string]interface{}{
			"status": "error",
			"error":  err.Error(),
			"output": tailString(string(out), 2000),
		})
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"status": "synced",
		"output": tailString(string(out), 2000),
	})
}

// enrichSync computes the sync status for a subscription by comparing the
// aggregator's content (GET /sub/{name}, base64) with the local file.
// The aggregator does not emit Last-Modified (nginx → FastAPI), so we compare
// content instead of timestamps — more accurate anyway.
func (h *Handlers) enrichSync(sub *Subscription) {
	if sub == nil {
		return
	}
	if sub.Status != "active" {
		synced := false
		sub.Synced = &synced
		sub.AggrLastModified = ""
		return
	}

	contentMatch, checkedAt, err := h.aggregatorMatchesLocal(sub.Name)
	if err != nil || !contentMatch {
		sub.AggrLastModified = checkedAt
	}
	if err != nil {
		// Aggregator unreachable → unknown.
		sub.Synced = nil
		return
	}
	sub.AggrLastModified = checkedAt

	synced := contentMatch
	if sub.UpdatedAt != "" && sub.FileMtime != "" && sub.UpdatedAt > sub.FileMtime {
		synced = false // metadata changed after the file was written → needs regenerate
	}
	sub.Synced = &synced
}

// aggregatorMatchesLocal fetches /sub/{name} from the aggregator and compares
// its (base64-decoded) content with the local subscription file.
func (h *Handlers) aggregatorMatchesLocal(name string) (bool, string, error) {
	client := &http.Client{Timeout: 6 * time.Second}
	u := strings.TrimRight(h.config.AggregatorURL, "/") + "/sub/" + url.PathEscape(name)
	resp, err := client.Get(u)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	checkedAt := ""
	if d := resp.Header.Get("Date"); d != "" {
		if t, err := http.ParseTime(d); err == nil {
			checkedAt = t.UTC().Format("2006-01-02T15:04:05Z07:00")
		}
	}

	if resp.StatusCode == http.StatusNotFound {
		return false, checkedAt, nil
	}
	if resp.StatusCode >= 400 {
		return false, checkedAt, fmt.Errorf("aggregator HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, checkedAt, err
	}

	local, err := h.storage.GetSubscriptionRaw(name)
	if err != nil {
		return false, checkedAt, nil
	}

	normLocal := strings.TrimRight(local, "\n")

	// Try base64 (aggregator returns base64 of the file).
	clean := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' {
			return -1
		}
		return r
	}, string(body))
	if decoded, err := base64.StdEncoding.DecodeString(clean); err == nil {
		if strings.TrimRight(string(decoded), "\n") == normLocal {
			return true, checkedAt, nil
		}
	}
	// Fallback: plain comparison.
	if strings.TrimRight(string(body), "\n") == normLocal {
		return true, checkedAt, nil
	}
	return false, checkedAt, nil
}

// subscriptionNameTaken checks name duplication (case-insensitive) in files and meta.
func (h *Handlers) subscriptionNameTaken(name string) bool {
	if h.storage.SubscriptionFileExists(name) {
		return true
	}
	if _, ok := h.storage.GetSubMeta(name); ok {
		return true
	}
	return false
}

// ksLabelFor builds a human-readable label for a KeySource.
func ksLabelFor(ks *KeySource, panelMap map[string]Panel) string {
	if ks.Label != "" {
		return ks.Label
	}
	if ks.Type == "manual" {
		return "manual"
	}
	p := panelMap[ks.PanelID]
	return fmt.Sprintf("%s · %s", p.Name, ks.ClientEmail)
}

func keySourceInKeys(keys []SubKey, ksID string) bool {
	for _, k := range keys {
		if k.KeySourceID != nil && *k.KeySourceID == ksID {
			return true
		}
	}
	return false
}

func countKind(items []GenerationReportItem, kind string) int {
	n := 0
	for _, it := range items {
		if it.Kind == kind {
			n++
		}
	}
	return n
}

func strPtr(s string) *string { return &s }

// nowStr returns the current time as RFC3339 UTC.
func nowStr() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// randID returns a short random hex id.
func randID() string {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

// validSubscriptionName restricts subscription names to safe file characters.
func validSubscriptionName(name string) bool {
	if name == "" || len(name) > 60 {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

// labelFromVless derives a label from the fragment of a vless link.
func labelFromVless(link string) string {
	if u, err := url.Parse(link); err == nil && u.Fragment != "" {
		return u.Fragment
	}
	return "manual"
}

func tailString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// sanitizeFilename ensures a safe filename
func sanitizeFilename(name string) string {
	reg := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
	return reg.ReplaceAllString(name, "_")
}

// ensure interfaces
var _ http.Handler = chi.NewRouter()
