package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"vlesspanel/dto"
	"vlesspanel/model"
)

// SubscriptionService — use cases для подписок (генерация/обновление/синк-статус).
type SubscriptionService struct {
	storage  Repository
	panelAPI PanelClient
	sync     *SyncState
	daemon   VlessSubTestClient
}

func NewSubscriptionService(storage Repository, panelAPI PanelClient, sync *SyncState, daemon VlessSubTestClient) *SubscriptionService {
	return &SubscriptionService{storage: storage, panelAPI: panelAPI, sync: sync, daemon: daemon}
}

// enrichSync проставляет sync-статус подписки из глобального флага.
func (s *SubscriptionService) enrichSync(sub *model.Subscription) {
	if sub == nil {
		return
	}
	if sub.Status != "active" {
		synced := false
		sub.Synced = &synced
		sub.AggrLastModified = ""
		return
	}
	synced := !s.sync.Needed()
	sub.Synced = &synced
	sub.AggrLastModified = ""
}

// subscriptionNameTaken проверяет дубликат имени (files + meta).
func (s *SubscriptionService) subscriptionNameTaken(name string) bool {
	if s.storage.SubscriptionFileExists(name) {
		return true
	}
	if _, ok := s.storage.GetSubMeta(name); ok {
		return true
	}
	return false
}

func (s *SubscriptionService) List() ([]model.Subscription, error) {
	subs, err := s.storage.ListSubscriptions()
	if err != nil {
		return nil, errInternal(msgListSubsFailed)
	}
	for i := range subs {
		s.enrichSync(&subs[i])
	}
	return subs, nil
}

func (s *SubscriptionService) Get(id string) (*model.Subscription, error) {
	sub, err := s.storage.GetSubscription(id)
	if err != nil {
		return nil, err
	}
	s.enrichSync(sub)
	return sub, nil
}

func (s *SubscriptionService) Raw(id string) (string, error) {
	if !validSubscriptionName(id) {
		return "", errBadRequest(msgNameInvalid)
	}
	return s.storage.GetSubscriptionRaw(id)
}

// Create создаёт подписку из KeySources (или legacy keys) и генерирует файл.
func (s *SubscriptionService) Create(req dto.CreateSubscriptionRequest) (dto.SubscriptionGenerateResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return dto.SubscriptionGenerateResponse{}, errBadRequest(msgNameRequired)
	}
	if !validSubscriptionName(name) {
		return dto.SubscriptionGenerateResponse{}, errBadRequest(msgNameInvalid)
	}
	if s.subscriptionNameTaken(name) {
		return dto.SubscriptionGenerateResponse{}, errConflict(fmt.Sprintf(msgNameTaken, name))
	}

	report := &model.GenerationReport{Items: []model.GenerationReportItem{}}

	keys := []model.SubKey{}
	if len(req.KeySourceIDs) > 0 {
		var err error
		keys, err = s.resolveKeySources(req.KeySourceIDs, report, true)
		if err != nil {
			log.Printf("resolve key sources: %v", err)
			return dto.SubscriptionGenerateResponse{}, errInternal(msgResolveKeysFailed)
		}
	} else if len(req.Keys) > 0 {
		keys = req.Keys
		report.Items = append(report.Items, model.GenerationReportItem{Kind: "manual", Label: msgRptManualLegacy})
	}

	status := "active"
	if len(keys) == 0 {
		status = "draft"
	}
	if status == "active" {
		if err := s.storage.WriteSubscriptionFile(name, keys); err != nil {
			return dto.SubscriptionGenerateResponse{}, errInternal(msgWriteSubFileFailed)
		}
		s.sync.Mark()
	}

	now := nowStr()
	meta := model.Subscription{ID: name, Name: name, Status: status, Keys: keys, UpdatedAt: now}
	if err := s.storage.UpsertSubMeta(meta); err != nil {
		return dto.SubscriptionGenerateResponse{}, errInternal(msgSaveSubFailed)
	}

	meta.FileMtime = s.storage.SubscriptionFileMtime(name)
	s.enrichSync(&meta)

	report.Included = countKind(report.Items, "ok") + countKind(report.Items, "manual")
	report.Skipped = countKind(report.Items, "skip")

	return dto.SubscriptionGenerateResponse{
		Subscription: meta,
		Report:       report.Items,
		Included:     report.Included,
		Skipped:      report.Skipped,
	}, nil
}

// Update обновляет подписку (режимы: rename / regenerate / addKeySourceIds /
// removeKeyId / legacy keys). Возвращает Subscription, либо Generate-отчёт
// для режима regenerate.
func (s *SubscriptionService) Update(id string, req dto.UpdateSubscriptionRequest) (dto.UpdateSubscriptionResult, error) {
	sub, err := s.storage.GetSubscription(id)
	if err != nil {
		return dto.UpdateSubscriptionResult{}, err
	}

	meta := *sub
	if meta.ID == "" {
		meta.ID = id
	}
	meta.Name = id

	// ─── Rename ───
	if req.Name != "" && req.Name != id {
		if !validSubscriptionName(req.Name) {
			return dto.UpdateSubscriptionResult{}, errBadRequest(msgNameInvalid)
		}
		if s.subscriptionNameTaken(req.Name) {
			return dto.UpdateSubscriptionResult{}, errConflict(fmt.Sprintf(msgNameTaken, req.Name))
		}
		if s.storage.SubscriptionFileExists(id) {
			if err := s.storage.RenameSubscriptionFile(id, req.Name); err != nil {
				log.Printf("storage error: %v", err)
				return dto.UpdateSubscriptionResult{}, errInternal(msgInternal)
			}
			s.sync.Mark()
		}
		if err := s.storage.DeleteSubMeta(id); err != nil {
			log.Printf("storage error: %v", err)
			return dto.UpdateSubscriptionResult{}, errInternal(msgInternal)
		}
		meta.Name = req.Name
		meta.ID = req.Name
		meta.UpdatedAt = nowStr()
		if err := s.storage.UpsertSubMeta(meta); err != nil {
			log.Printf("storage error: %v", err)
			return dto.UpdateSubscriptionResult{}, errInternal(msgInternal)
		}
		meta.FileMtime = s.storage.SubscriptionFileMtime(meta.Name)
		s.enrichSync(&meta)
		return dto.UpdateSubscriptionResult{Subscription: meta}, nil
	}

	// ─── Regenerate ───
	if req.Regenerate {
		report := &model.GenerationReport{Items: []model.GenerationReportItem{}}
		keys, err := s.regenerateKeys(meta.Keys, report)
		if err != nil {
			log.Printf("regenerate keys: %v", err)
			return dto.UpdateSubscriptionResult{}, errInternal(msgRegenerateFailed)
		}

		meta.Keys = keys
		meta.UpdatedAt = nowStr()
		if len(keys) == 0 {
			meta.Status = "draft"
			_ = s.storage.RemoveSubscriptionFile(meta.Name)
		} else {
			meta.Status = "active"
			if err := s.storage.WriteSubscriptionFile(meta.Name, keys); err != nil {
				return dto.UpdateSubscriptionResult{}, errInternal(msgWriteSubFileFailed)
			}
		}
		s.sync.Mark()
		if err := s.storage.UpsertSubMeta(meta); err != nil {
			log.Printf("storage error: %v", err)
			return dto.UpdateSubscriptionResult{}, errInternal(msgInternal)
		}
		report.Included = countKind(report.Items, "ok") + countKind(report.Items, "manual")
		report.Skipped = countKind(report.Items, "skip")
		meta.FileMtime = s.storage.SubscriptionFileMtime(meta.Name)
		s.enrichSync(&meta)
		return dto.UpdateSubscriptionResult{Generate: &dto.SubscriptionGenerateResponse{
			Subscription: meta,
			Report:       report.Items,
			Included:     report.Included,
			Skipped:      report.Skipped,
		}}, nil
	}

	// ─── Add KeySource refs (no file write) ───
	if len(req.AddKeySourceIDs) > 0 {
		for _, ksID := range req.AddKeySourceIDs {
			if keySourceInKeys(meta.Keys, ksID) {
				continue
			}
			if _, err := s.storage.GetKeySource(ksID); err != nil {
				return dto.UpdateSubscriptionResult{}, errBadRequest(msgKSNotFoundPrefix + ksID)
			}
			sid := ksID
			meta.Keys = append(meta.Keys, model.SubKey{ID: "k-" + randID(), KeySourceID: &sid})
		}
		meta.UpdatedAt = nowStr()
		if err := s.storage.UpsertSubMeta(meta); err != nil {
			log.Printf("storage error: %v", err)
			return dto.UpdateSubscriptionResult{}, errInternal(msgInternal)
		}
		meta.FileMtime = s.storage.SubscriptionFileMtime(meta.Name)
		s.enrichSync(&meta)
		return dto.UpdateSubscriptionResult{Subscription: meta}, nil
	}

	// ─── Remove one key + rewrite file ───
	if req.RemoveKeyID != "" {
		newKeys := make([]model.SubKey, 0, len(meta.Keys))
		for _, k := range meta.Keys {
			if k.ID != req.RemoveKeyID {
				newKeys = append(newKeys, k)
			}
		}
		meta.Keys = newKeys
		meta.UpdatedAt = nowStr()
		if len(newKeys) == 0 {
			meta.Status = "draft"
			_ = s.storage.RemoveSubscriptionFile(meta.Name)
		} else {
			meta.Status = "active"
			if err := s.storage.WriteSubscriptionFile(meta.Name, newKeys); err != nil {
				return dto.UpdateSubscriptionResult{}, errInternal(msgWriteSubFileFailed)
			}
		}
		s.sync.Mark()
		if err := s.storage.UpsertSubMeta(meta); err != nil {
			log.Printf("storage error: %v", err)
			return dto.UpdateSubscriptionResult{}, errInternal(msgInternal)
		}
		meta.FileMtime = s.storage.SubscriptionFileMtime(meta.Name)
		s.enrichSync(&meta)
		return dto.UpdateSubscriptionResult{Subscription: meta}, nil
	}

	// ─── Legacy mode: full key replacement ───
	if req.Keys != nil {
		meta.Keys = req.Keys
		meta.UpdatedAt = nowStr()
		if len(req.Keys) == 0 {
			meta.Status = "draft"
			_ = s.storage.RemoveSubscriptionFile(meta.Name)
		} else {
			meta.Status = "active"
			if err := s.storage.WriteSubscriptionFile(meta.Name, req.Keys); err != nil {
				return dto.UpdateSubscriptionResult{}, errInternal(msgWriteSubFileFailed)
			}
		}
		s.sync.Mark()
		if err := s.storage.UpsertSubMeta(meta); err != nil {
			log.Printf("storage error: %v", err)
			return dto.UpdateSubscriptionResult{}, errInternal(msgInternal)
		}
		meta.FileMtime = s.storage.SubscriptionFileMtime(meta.Name)
		s.enrichSync(&meta)
		return dto.UpdateSubscriptionResult{Subscription: meta}, nil
	}

	// No-op
	meta.FileMtime = s.storage.SubscriptionFileMtime(meta.Name)
	s.enrichSync(&meta)
	return dto.UpdateSubscriptionResult{Subscription: meta}, nil
}

// Delete удаляет подписку (файл + мета).
func (s *SubscriptionService) Delete(id string) error {
	if !validSubscriptionName(id) {
		return errBadRequest(msgNameInvalid)
	}
	if !s.storage.SubscriptionFileExists(id) {
		if _, ok := s.storage.GetSubMeta(id); !ok {
			return errNotFound(fmt.Sprintf(msgSubNotFound, id))
		}
	}
	if err := s.storage.RemoveSubscriptionFile(id); err != nil {
		return errInternal(err.Error())
	}
	s.sync.Mark()
	if err := s.storage.DeleteSubMeta(id); err != nil {
		return errInternal(err.Error())
	}
	return nil
}

// Test прогоняет все ключи подписки через vlesssubtest.
func (s *SubscriptionService) Test(id string) (dto.TestSubscriptionResponse, error) {
	sub, err := s.storage.GetSubscription(id)
	if err != nil {
		return dto.TestSubscriptionResponse{}, err
	}

	var testKeys []model.SubKey
	for _, k := range sub.Keys {
		if strings.TrimSpace(k.Link) != "" {
			testKeys = append(testKeys, k)
		}
	}
	if len(testKeys) == 0 {
		return dto.TestSubscriptionResponse{}, errBadRequest(msgNoKeysToTest)
	}

	var results []dto.TestSingleResponse
	okCount := 0

	for i, key := range testKeys {
		single, err := s.daemon.TestSingle(key.Link, 10)
		if err != nil {
			ip := "failed to parse response"
			if errors.Is(err, ErrDaemonUnreachable) {
				ip = rootCause(err).Error()
			}
			results = append(results, dto.TestSingleResponse{KeyIdx: i, Status: "ERROR", IP: ip})
			continue
		}

		single.KeyIdx = i
		if single.Status == "OK" {
			okCount++
		}
		results = append(results, single)
	}

	return dto.TestSubscriptionResponse{Total: len(testKeys), OK: okCount, Results: results}, nil
}

// RegenerateAll перегенерирует все подписки с panel-KeySource.
func (s *SubscriptionService) RegenerateAll() (dto.RegenerateAllResponse, error) {
	subs, err := s.storage.ListSubscriptions()
	if err != nil {
		return dto.RegenerateAllResponse{}, errInternal(msgLoadSubsFailed)
	}

	panels, _ := s.storage.LoadPanels()
	panelMap := make(map[string]model.Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}

	allKS, _ := s.storage.LoadKeySources()
	ksByID := make(map[string]model.KeySource, len(allKS))
	for _, ks := range allKS {
		ksByID[ks.ID] = ks
	}

	// Фаза 1: собрать panel-KeySource'ы, сгруппировать по панели.
	sourcesByPanel := make(map[string]map[string]model.KeySource)
	for i := range subs {
		for _, k := range subs[i].Keys {
			if k.KeySourceID == nil {
				continue
			}
			ks, ok := ksByID[*k.KeySourceID]
			if !ok || ks.Type != "panel" {
				continue
			}
			if _, ok := panelMap[ks.PanelID]; !ok {
				continue
			}
			m := sourcesByPanel[ks.PanelID]
			if m == nil {
				m = make(map[string]model.KeySource)
				sourcesByPanel[ks.PanelID] = m
			}
			m[ks.ID] = ks
		}
	}

	// Фаза 2: резолв ключей (панели параллельно).
	resolved := s.resolvePanelKeys(panelMap, sourcesByPanel)

	// Фаза 3: пересобрать каждую подписку.
	results := []dto.RegenerateSubResult{}
	regenerated := 0
	skipped := 0

	for i := range subs {
		sub := &subs[i]

		hasPanel := false
		for _, k := range sub.Keys {
			if k.KeySourceID == nil {
				continue
			}
			if ks, ok := ksByID[*k.KeySourceID]; ok && ks.Type == "panel" {
				hasPanel = true
				break
			}
		}
		if !hasPanel {
			skipped++
			results = append(results, dto.RegenerateSubResult{Name: sub.Name, Regenerated: false, Reason: msgRptNoPanelKS})
			continue
		}

		keys := make([]model.SubKey, 0, len(sub.Keys))
		okCount := 0
		manualCount := 0
		skipCount := 0
		for _, k := range sub.Keys {
			if k.KeySourceID == nil {
				keys = append(keys, k)
				manualCount++
				continue
			}
			ks, exists := ksByID[*k.KeySourceID]
			if !exists {
				skipCount++
				continue
			}
			if ks.Type == "manual" {
				keys = append(keys, model.SubKey{ID: k.ID, Link: ks.VlessLink, KeySourceID: k.KeySourceID})
				manualCount++
				continue
			}
			if _, ok := panelMap[ks.PanelID]; !ok {
				skipCount++
				continue
			}
			res, ok := resolved[ks.ID]
			if !ok || res.err != nil {
				skipCount++
				continue
			}
			keys = append(keys, model.SubKey{ID: k.ID, Link: res.key.Link, KeySourceID: k.KeySourceID})
			okCount++
		}

		sub.Keys = keys
		sub.UpdatedAt = nowStr()
		if len(keys) == 0 {
			sub.Status = "draft"
			_ = s.storage.RemoveSubscriptionFile(sub.Name)
		} else {
			sub.Status = "active"
			if err := s.storage.WriteSubscriptionFile(sub.Name, keys); err != nil {
				skipped++
				log.Printf("regenerate-all: write file %s: %v", sub.Name, err)
				results = append(results, dto.RegenerateSubResult{Name: sub.Name, Regenerated: false, Reason: msgWriteSubFileFailed})
				continue
			}
		}
		s.sync.Mark()
		if err := s.storage.UpsertSubMeta(*sub); err != nil {
			skipped++
			log.Printf("regenerate-all: upsert meta %s: %v", sub.Name, err)
			results = append(results, dto.RegenerateSubResult{Name: sub.Name, Regenerated: false, Reason: msgInternal})
			continue
		}

		regenerated++
		results = append(results, dto.RegenerateSubResult{
			Name:        sub.Name,
			Regenerated: true,
			Included:    okCount + manualCount,
			SkippedKeys: skipCount,
		})
	}

	return dto.RegenerateAllResponse{Regenerated: regenerated, Skipped: skipped, Results: results}, nil
}

// keyResolveResult — результат резолва свежего ключа одного panel-KeySource.
type keyResolveResult struct {
	key model.VLESSKey
	err error
}

// perPanelConcurrency — сколько ссылок на клиентов панели запрашивать одновременно.
const perPanelConcurrency = 3

// resolvePanelKeys резолвит свежие ключи для panel-KeySource, сгруппировав по панелям.
func (s *SubscriptionService) resolvePanelKeys(panelMap map[string]model.Panel, sourcesByPanel map[string]map[string]model.KeySource) map[string]keyResolveResult {
	results := make(map[string]keyResolveResult)
	caches := make(map[string]model.CachedKey)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for pid, sources := range sourcesByPanel {
		panel := panelMap[pid]
		wg.Add(1)
		go func(panel model.Panel, sources map[string]model.KeySource) {
			defer wg.Done()

			emails := make([]string, 0, len(sources))
			seen := make(map[string]bool, len(sources))
			for _, ks := range sources {
				if !seen[ks.ClientEmail] {
					seen[ks.ClientEmail] = true
					emails = append(emails, ks.ClientEmail)
				}
			}

			keysByEmail, inbounds, err := s.panelAPI.GetClientKeysForEmails(panel, emails, perPanelConcurrency)

			portByInbound := make(map[int]int, len(inbounds))
			for _, ib := range inbounds {
				portByInbound[ib.ID] = ib.Port
			}

			for id, ks := range sources {
				res := keyResolveResult{}
				if err != nil {
					res.err = err
				} else {
					targetPort, ok := portByInbound[ks.InboundID]
					if !ok {
						res.err = fmt.Errorf("%w: inbound %d", ErrInboundNotFound, ks.InboundID)
					} else {
						found := false
						for _, k := range keysByEmail[ks.ClientEmail] {
							if k.Port == targetPort {
								res.key = k
								found = true
								break
							}
						}
						if !found {
							res.err = fmt.Errorf("%w: client %s on inbound %d", ErrClientNotFound, ks.ClientEmail, ks.InboundID)
						}
					}
				}

				mu.Lock()
				results[id] = res
				if res.err == nil {
					caches[id] = model.CachedKey{Link: res.key.Link, FetchedAt: nowStr()}
				}
				mu.Unlock()
			}
		}(panel, sources)
	}
	wg.Wait()

	if len(caches) > 0 {
		_ = s.storage.UpdateKeySourceCaches(caches)
	}
	return results
}

// resolveKeySources извлекает свежие ключи для списка KeySource ID.
func (s *SubscriptionService) resolveKeySources(ids []string, report *model.GenerationReport, includeManual bool) ([]model.SubKey, error) {
	panels, _ := s.storage.LoadPanels()
	panelMap := make(map[string]model.Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}

	keys := []model.SubKey{}
	for _, id := range ids {
		ks, err := s.storage.GetKeySource(id)
		if err != nil {
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "skip", Label: id, Why: msgRptKSNotFound})
			continue
		}

		label := ksLabelFor(ks, panelMap)

		if ks.Type == "manual" {
			if !includeManual {
				report.Items = append(report.Items, model.GenerationReportItem{Kind: "skip", Label: label, Why: msgRptManualSeparate})
				continue
			}
			keys = append(keys, model.SubKey{ID: "k-" + randID(), Link: ks.VlessLink, KeySourceID: strPtr(ks.ID)})
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "manual", Label: label})
			continue
		}

		panel, ok := panelMap[ks.PanelID]
		if !ok {
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "skip", Label: label, Why: msgRptPanelDeleted})
			continue
		}

		start := time.Now()
		key, err := s.panelAPI.GetClientKeyForInbound(panel, ks.ClientEmail, ks.InboundID)
		ms := int(time.Since(start).Milliseconds())

		if err != nil {
			why := err.Error()
			switch {
			case errors.Is(err, ErrClientNotFound):
				why = msgRptClientNotFound
			case errors.Is(err, ErrInboundNotFound):
				why = msgRptInboundNotFound
			case errors.Is(err, ErrPanelUnreachable):
				why = msgPanelUnreachable
			}
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "skip", Label: label, Why: why})
			continue
		}

		ks.CachedKey = &model.CachedKey{Link: key.Link, FetchedAt: nowStr()}
		ks.UpdatedAt = nowStr()
		_ = s.storage.UpdateKeySource(*ks)

		keys = append(keys, model.SubKey{ID: "k-" + randID(), Link: key.Link, KeySourceID: strPtr(ks.ID)})
		report.Items = append(report.Items, model.GenerationReportItem{Kind: "ok", Label: label, Ms: ms})
	}
	return keys, nil
}

// regenerateKeys обновляет panel-ключи подписки, сохраняя manual.
func (s *SubscriptionService) regenerateKeys(keys []model.SubKey, report *model.GenerationReport) ([]model.SubKey, error) {
	panels, _ := s.storage.LoadPanels()
	panelMap := make(map[string]model.Panel, len(panels))
	for _, p := range panels {
		panelMap[p.ID] = p
	}

	out := []model.SubKey{}
	for _, k := range keys {
		if k.KeySourceID == nil {
			out = append(out, k)
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "manual", Label: msgRptManualLegacy})
			continue
		}

		ks, err := s.storage.GetKeySource(*k.KeySourceID)
		if err != nil {
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "skip", Label: *k.KeySourceID, Why: msgRptKSNotFoundRemoved})
			continue
		}
		label := ksLabelFor(ks, panelMap)

		if ks.Type == "manual" {
			out = append(out, model.SubKey{ID: k.ID, Link: ks.VlessLink, KeySourceID: k.KeySourceID})
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "manual", Label: label})
			continue
		}

		panel, ok := panelMap[ks.PanelID]
		if !ok {
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "skip", Label: label, Why: msgRptPanelDeletedRemoved})
			continue
		}

		start := time.Now()
		key, err := s.panelAPI.GetClientKeyForInbound(panel, ks.ClientEmail, ks.InboundID)
		ms := int(time.Since(start).Milliseconds())
		if err != nil {
			why := err.Error()
			switch {
			case errors.Is(err, ErrClientNotFound):
				why = msgRptClientNotFoundNotIncluded
			case errors.Is(err, ErrInboundNotFound):
				why = msgRptInboundNotFoundNotIncluded
			case errors.Is(err, ErrPanelUnreachable):
				why = msgPanelUnreachable
			}
			report.Items = append(report.Items, model.GenerationReportItem{Kind: "skip", Label: label, Why: why})
			continue
		}

		ks.CachedKey = &model.CachedKey{Link: key.Link, FetchedAt: nowStr()}
		ks.UpdatedAt = nowStr()
		_ = s.storage.UpdateKeySource(*ks)

		out = append(out, model.SubKey{ID: k.ID, Link: key.Link, KeySourceID: k.KeySourceID})
		report.Items = append(report.Items, model.GenerationReportItem{Kind: "ok", Label: label, Ms: ms})
	}
	return out, nil
}
