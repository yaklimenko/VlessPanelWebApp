package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// KeySourceService — use cases для KeySource (источников ключей).
type KeySourceService struct {
	storage   *Storage
	panelAPI  *PanelAPI
	sync      *SyncState
	daemonURL string
}

func NewKeySourceService(storage *Storage, panelAPI *PanelAPI, sync *SyncState, daemonURL string) *KeySourceService {
	return &KeySourceService{storage: storage, panelAPI: panelAPI, sync: sync, daemonURL: daemonURL}
}

// panelSnapshot caches clients of one panel for a request batch.
type panelSnapshot struct {
	clients  []Client
	inbounds []XUIInbound
	err      error
}

// loadPanelSnapshot fetches one panel's clients+inbounds in parallel.
func (s *KeySourceService) loadPanelSnapshot(panel Panel) *panelSnapshot {
	clients, inbounds, err := s.panelAPI.ListClientsAndInbounds(panel)
	return &panelSnapshot{clients: clients, inbounds: inbounds, err: err}
}

// loadSnapshots fetches clients+inbounds for all panels referenced by the
// given KeySources in parallel (one goroutine per panel).
func (s *KeySourceService) loadSnapshots(panelMap map[string]Panel, sources []KeySource) map[string]*panelSnapshot {
	needed := make(map[string]bool)
	for _, ks := range sources {
		if ks.Type == "panel" {
			if _, ok := panelMap[ks.PanelID]; ok {
				needed[ks.PanelID] = true
			}
		}
	}

	snapshot := make(map[string]*panelSnapshot, len(needed))
	var wg sync.WaitGroup
	var mu sync.Mutex
	for pid := range needed {
		panel := panelMap[pid]
		wg.Add(1)
		go func(p Panel) {
			defer wg.Done()
			snap := s.loadPanelSnapshot(p)
			mu.Lock()
			snapshot[p.ID] = snap
			mu.Unlock()
		}(panel)
	}
	wg.Wait()
	return snapshot
}

// List возвращает все KeySource с derived-статусами.
func (s *KeySourceService) List() ([]KeySource, error) {
	sources, err := s.storage.LoadKeySources()
	if err != nil {
		return nil, errInternal("Failed to load key sources")
	}

	panels, _ := s.storage.LoadPanels()
	panelMap := make(map[string]Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}

	usage := map[string]int{}
	if subs, err := s.storage.ListSubscriptions(); err == nil {
		for _, sub := range subs {
			for _, k := range sub.Keys {
				if k.KeySourceID != nil {
					usage[*k.KeySourceID]++
				}
			}
		}
	}

	snapshot := s.loadSnapshots(panelMap, sources)
	resp := make([]KeySource, 0, len(sources))
	for i := range sources {
		ks := sources[i]
		s.enrichKeySource(&ks, panelMap, snapshot, usage)
		resp = append(resp, ks)
	}
	return resp, nil
}

// Get возвращает один KeySource с derived-статусом.
func (s *KeySourceService) Get(id string) (*KeySource, error) {
	ks, err := s.storage.GetKeySource(id)
	if err != nil {
		return nil, err
	}
	panels, _ := s.storage.LoadPanels()
	panelMap := make(map[string]Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}
	s.enrichKeySource(ks, panelMap, map[string]*panelSnapshot{}, map[string]int{})
	return ks, nil
}

// Create создаёт KeySource (panel triplet или manual vless link) с дедупом.
func (s *KeySourceService) Create(req CreateKeySourceRequest) (CreateKeySourceResponse, error) {
	req.Type = strings.ToLower(strings.TrimSpace(req.Type))
	switch req.Type {
	case "panel":
		if req.PanelID == "" || req.ClientEmail == "" || req.InboundID == 0 {
			return CreateKeySourceResponse{}, errBadRequest("panelId, clientEmail и inboundId обязательны для type=panel")
		}
	case "manual":
		req.VlessLink = strings.TrimSpace(req.VlessLink)
		if !strings.HasPrefix(req.VlessLink, "vless://") {
			return CreateKeySourceResponse{}, errBadRequest("vlessLink должен начинаться с vless://")
		}
	default:
		return CreateKeySourceResponse{}, errBadRequest("type должен быть panel или manual")
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

	existing, deduped, err := s.storage.AddKeySource(ks)
	if err != nil {
		return CreateKeySourceResponse{}, errInternal("Failed to save key source")
	}
	if deduped {
		return CreateKeySourceResponse{KeySource: existing, Deduped: true}, nil
	}
	return CreateKeySourceResponse{KeySource: &ks, Deduped: false}, nil
}

// Delete удаляет KeySource и каскадно чистит ссылки из подписок.
func (s *KeySourceService) Delete(id string) (DeleteKeySourceResponse, error) {
	ks, err := s.storage.GetKeySource(id)
	if err != nil {
		return DeleteKeySourceResponse{}, err
	}

	subs, err := s.storage.ListSubscriptions()
	if err != nil {
		return DeleteKeySourceResponse{}, errInternal("Failed to load subscriptions")
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
			_ = s.storage.RemoveSubscriptionFile(sub.Name)
		} else {
			sub.Status = "active"
			if err := s.storage.WriteSubscriptionFile(sub.Name, newKeys); err != nil {
				log.Printf("DeleteKeySource: rewriting file for %s: %v", sub.Name, err)
			}
		}
		if err := s.storage.UpsertSubMeta(sub); err != nil {
			log.Printf("DeleteKeySource: upsert meta for %s: %v", sub.Name, err)
		}
		affected = append(affected, sub.Name)
	}
	if len(affected) > 0 {
		s.markSyncNeeded()
	}

	if err := s.storage.DeleteKeySource(id); err != nil {
		return DeleteKeySourceResponse{}, errInternal(err.Error())
	}

	label := ks.Label
	if label == "" {
		label = id
	}
	return DeleteKeySourceResponse{
		Status:              "deleted",
		Label:               label,
		UsedInSubscriptions: len(affected),
		Subscriptions:       affected,
	}, nil
}

// GetKey возвращает свежий ключ (для manual — хранимый link), обновляя кэш.
func (s *KeySourceService) GetKey(id string) (KeySourceKeyResponse, error) {
	ks, err := s.storage.GetKeySource(id)
	if err != nil {
		return KeySourceKeyResponse{}, err
	}

	if ks.Type == "manual" {
		return KeySourceKeyResponse{Key: VLESSKey{Label: ks.Label, Link: ks.VlessLink}}, nil
	}

	panel, err := s.storage.GetPanel(ks.PanelID)
	if err != nil {
		return KeySourceKeyResponse{}, errNotFound("панель KeySource не найдена (удалена?)")
	}

	key, err := s.panelAPI.GetClientKeyForInbound(panel, ks.ClientEmail, ks.InboundID)
	if err != nil {
		log.Printf("GetKeySourceKey %s: %v", id, err)
		if errors.Is(err, ErrPanelUnreachable) {
			return KeySourceKeyResponse{}, errBadGateway("панель недоступна (таймаут 10 с)")
		}
		return KeySourceKeyResponse{}, errNotFound(err.Error())
	}

	ks.CachedKey = &CachedKey{Link: key.Link, FetchedAt: nowStr()}
	ks.UpdatedAt = nowStr()
	if err := s.storage.UpdateKeySource(*ks); err != nil {
		log.Printf("GetKeySourceKey: cache update failed: %v", err)
	}

	return KeySourceKeyResponse{Key: key, Source: ks}, nil
}

// Test прогоняет test-single через демон и сохраняет lastTest.
func (s *KeySourceService) Test(id string) (KeySourceTestResponse, error) {
	ks, err := s.storage.GetKeySource(id)
	if err != nil {
		return KeySourceTestResponse{}, err
	}

	link := ""
	if ks.Type == "manual" {
		link = ks.VlessLink
	} else {
		panel, err := s.storage.GetPanel(ks.PanelID)
		if err != nil {
			return KeySourceTestResponse{}, errNotFound("панель KeySource не найдена (удалена?)")
		}
		key, err := s.panelAPI.GetClientKeyForInbound(panel, ks.ClientEmail, ks.InboundID)
		if err != nil {
			lastTest := &KeySourceTest{Status: "fail", At: nowStr(), Error: err.Error()}
			ks.LastTest = lastTest
			ks.UpdatedAt = nowStr()
			_ = s.storage.UpdateKeySource(*ks)
			return KeySourceTestResponse{}, errBadGateway("не удалось получить ключ: " + err.Error())
		}
		link = key.Link
		ks.CachedKey = &CachedKey{Link: key.Link, FetchedAt: nowStr()}
	}

	if strings.TrimSpace(link) == "" {
		return KeySourceTestResponse{}, errBadRequest("у KeySource нет ключа для теста")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	daemonURL := strings.TrimRight(s.daemonURL, "/") + "/test-single"
	reqBody, _ := json.Marshal(TestSingleRequest{Vless: link, Timeout: 10})

	start := time.Now()
	resp, err := client.Post(daemonURL, "application/json", bytes.NewReader(reqBody))
	ms := int(time.Since(start).Milliseconds())
	if err != nil {
		lastTest := &KeySourceTest{Status: "fail", At: nowStr(), Error: "демон тестов недоступен: " + err.Error()}
		ks.LastTest = lastTest
		ks.UpdatedAt = nowStr()
		_ = s.storage.UpdateKeySource(*ks)
		return KeySourceTestResponse{Result: nil, LastTest: lastTest, Error: lastTest.Error}, nil
	}

	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var single TestSingleResponse
	if err := json.Unmarshal(body, &single); err != nil {
		lastTest := &KeySourceTest{Status: "fail", At: nowStr(), Error: "не удалось разобрать ответ демона"}
		ks.LastTest = lastTest
		ks.UpdatedAt = nowStr()
		_ = s.storage.UpdateKeySource(*ks)
		return KeySourceTestResponse{Result: nil, LastTest: lastTest, Error: lastTest.Error}, nil
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
	_ = s.storage.UpdateKeySource(*ks)

	return KeySourceTestResponse{Result: &single, LastTest: lastTest}, nil
}

// Traffic возвращает up/down/expiry для panel-KeySource.
func (s *KeySourceService) Traffic(id string) (KeySourceTrafficResponse, error) {
	ks, err := s.storage.GetKeySource(id)
	if err != nil {
		return KeySourceTrafficResponse{}, err
	}
	if ks.Type == "manual" {
		return KeySourceTrafficResponse{}, errBadRequest("manual KeySource не имеет трафика")
	}

	panel, err := s.storage.GetPanel(ks.PanelID)
	if err != nil {
		return KeySourceTrafficResponse{}, errNotFound("панель KeySource не найдена (удалена?)")
	}

	client, err := s.panelAPI.GetClientStats(panel, ks.ClientEmail)
	if err != nil {
		return KeySourceTrafficResponse{}, errNotFound(err.Error())
	}

	return KeySourceTrafficResponse{
		Up:         client.Up,
		Down:       client.Down,
		ExpiryTime: client.ExpiryTime,
		Enable:     client.Enable,
	}, nil
}

// enrichKeySource fills derived fields (status, traffic, names) for a KeySource.
func (s *KeySourceService) enrichKeySource(ks *KeySource, panelMap map[string]Panel, snapshot map[string]*panelSnapshot, usage map[string]int) {
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
		snap = s.loadPanelSnapshot(panel)
		snapshot[ks.PanelID] = snap
	}

	if snap.err != nil {
		ks.Status = "panel_unreachable"
		ks.Error = "панель недоступна (таймаут 10 с)"
		return
	}

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

// markSyncNeeded поднимает флаг «нужна сверка с агрегатором».
func (s *KeySourceService) markSyncNeeded() {
	s.sync.Mark()
}
