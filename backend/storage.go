package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"vlesspanel/dto"
	"vlesspanel/model"
)

// Storage handles file-based data persistence.
// Layout:
//   - panels.json        — list of 3X-UI panels
//   - key-sources.json   — list of KeySource records
//   - subscriptions.json — subscription metadata (status, keySourceId refs, updatedAt)
//   - tokens.json        — issued API tokens (sha256 hashes only)
//   - aggregatorDir      — generated subscription files configs-{name}.txt
//
// Legacy subscriptions (files without metadata) are still readable: their keys
// are reported as manual SubKeys with keySourceId=null.
type Storage struct {
	mu             sync.RWMutex
	panelsPath     string
	aggregatorDir  string
	keySourcesPath string
	subsMetaPath   string
	tokensPath     string
}

// NewStorage creates a new Storage instance.
func NewStorage(panelsPath, aggregatorDir, dataDir string) *Storage {
	// Ensure panels.json exists
	if _, err := os.Stat(panelsPath); os.IsNotExist(err) {
		initial := []model.Panel{}
		data, _ := json.MarshalIndent(initial, "", "  ")
		os.WriteFile(panelsPath, data, 0644)
	}
	// Ensure dirs exist
	os.MkdirAll(aggregatorDir, 0755)
	os.MkdirAll(dataDir, 0755)

	keySourcesPath := filepath.Join(dataDir, "key-sources.json")
	subsMetaPath := filepath.Join(dataDir, "subscriptions.json")
	tokensPath := filepath.Join(dataDir, "tokens.json")

	// Ensure key-sources.json exists
	if _, err := os.Stat(keySourcesPath); os.IsNotExist(err) {
		initial := []model.KeySource{}
		data, _ := json.MarshalIndent(initial, "", "  ")
		os.WriteFile(keySourcesPath, data, 0644)
	}
	// Ensure subscriptions.json exists
	if _, err := os.Stat(subsMetaPath); os.IsNotExist(err) {
		initial := []model.Subscription{}
		data, _ := json.MarshalIndent(initial, "", "  ")
		os.WriteFile(subsMetaPath, data, 0644)
	}
	// Ensure tokens.json exists
	if _, err := os.Stat(tokensPath); os.IsNotExist(err) {
		initial := []model.APIToken{}
		data, _ := json.MarshalIndent(initial, "", "  ")
		os.WriteFile(tokensPath, data, 0644)
	}

	return &Storage{
		panelsPath:     panelsPath,
		aggregatorDir:  aggregatorDir,
		keySourcesPath: keySourcesPath,
		subsMetaPath:   subsMetaPath,
		tokensPath:     tokensPath,
	}
}

// --- Panels ---

// GetPanel returns a single panel by ID. Returns ErrPanelNotFound when absent.
// Loads the whole file under RLock (panels list is small and unindexed).
func (s *Storage) GetPanel(id string) (model.Panel, error) {
	panels, err := s.LoadPanels()
	if err != nil {
		return model.Panel{}, err
	}
	for i := range panels {
		if panels[i].ID == id {
			return panels[i], nil
		}
	}
	return model.Panel{}, fmt.Errorf("%w: %s", ErrPanelNotFound, id)
}

// loadPanelsLocked reads panels.json. Caller must hold at least RLock.
func (s *Storage) loadPanelsLocked() ([]model.Panel, error) {
	data, err := os.ReadFile(s.panelsPath)
	if err != nil {
		return nil, fmt.Errorf("reading panels file: %w", err)
	}

	var panels []model.Panel
	if err := json.Unmarshal(data, &panels); err != nil {
		return nil, fmt.Errorf("parsing panels file: %w", err)
	}

	return panels, nil
}

// savePanelsLocked writes panels.json. Caller must hold Lock.
func (s *Storage) savePanelsLocked(panels []model.Panel) error {
	data, err := json.MarshalIndent(panels, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling panels: %w", err)
	}

	if err := os.WriteFile(s.panelsPath, data, 0644); err != nil {
		return fmt.Errorf("writing panels file: %w", err)
	}

	return nil
}

// LoadPanels reads all panels from panels.json
func (s *Storage) LoadPanels() ([]model.Panel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadPanelsLocked()
}

// AddPanel adds a new panel and returns it with an assigned ID. The whole
// read-modify-write (load → unique-id → append → save) is atomic under one lock.
func (s *Storage) AddPanel(req dto.CreatePanelRequest) (model.Panel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	panels, err := s.loadPanelsLocked()
	if err != nil {
		return model.Panel{}, err
	}

	// Generate ID from name, make unique.
	id := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	baseID := id
	for i := 1; ; i++ {
		exists := false
		for _, p := range panels {
			if p.ID == id {
				exists = true
				break
			}
		}
		if !exists {
			break
		}
		id = fmt.Sprintf("%s-%d", baseID, i)
	}

	panel := model.Panel{
		ID:                 id,
		Name:               req.Name,
		URL:                req.URL,
		Token:              req.Token,
		WebBasePath:        req.WebBasePath,
		InsecureSkipVerify: req.InsecureSkipVerify,
	}

	panels = append(panels, panel)
	if err := s.savePanelsLocked(panels); err != nil {
		return model.Panel{}, err
	}

	return panel, nil
}

// UpdatePanelName переименовывает панель по ID (атомарный read-modify-write).
// Возвращает обновлённую панель. Панель не найдена → ErrPanelNotFound.
func (s *Storage) UpdatePanelName(id, name string) (model.Panel, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	panels, err := s.loadPanelsLocked()
	if err != nil {
		return model.Panel{}, err
	}
	for i := range panels {
		if panels[i].ID == id {
			panels[i].Name = name
			if err := s.savePanelsLocked(panels); err != nil {
				return model.Panel{}, err
			}
			return panels[i], nil
		}
	}
	return model.Panel{}, fmt.Errorf("%w: %s", ErrPanelNotFound, id)
}

// DeletePanel removes a panel by ID (atomic read-modify-write).
func (s *Storage) DeletePanel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	panels, err := s.loadPanelsLocked()
	if err != nil {
		return err
	}

	found := false
	for i, p := range panels {
		if p.ID == id {
			panels = append(panels[:i], panels[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("%w: %s", ErrPanelNotFound, id)
	}

	return s.savePanelsLocked(panels)
}

// --- Key Sources ---

// loadKeySourcesLocked reads key-sources.json. Caller must hold at least RLock.
func (s *Storage) loadKeySourcesLocked() ([]model.KeySource, error) {
	data, err := os.ReadFile(s.keySourcesPath)
	if err != nil {
		return nil, fmt.Errorf("reading key sources file: %w", err)
	}

	var sources []model.KeySource
	if err := json.Unmarshal(data, &sources); err != nil {
		return nil, fmt.Errorf("parsing key sources file: %w", err)
	}

	return sources, nil
}

// saveKeySourcesLocked writes key-sources.json. Caller must hold Lock.
func (s *Storage) saveKeySourcesLocked(sources []model.KeySource) error {
	data, err := json.MarshalIndent(sources, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling key sources: %w", err)
	}

	if err := os.WriteFile(s.keySourcesPath, data, 0644); err != nil {
		return fmt.Errorf("writing key sources file: %w", err)
	}

	return nil
}

// LoadKeySources reads all KeySources from key-sources.json
func (s *Storage) LoadKeySources() ([]model.KeySource, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadKeySourcesLocked()
}

// GetKeySource returns a KeySource by ID.
func (s *Storage) GetKeySource(id string) (*model.KeySource, error) {
	sources, err := s.LoadKeySources()
	if err != nil {
		return nil, err
	}
	for i := range sources {
		if sources[i].ID == id {
			ks := sources[i]
			return &ks, nil
		}
	}
	return nil, fmt.Errorf("%w: %s", ErrKeySourceNotFound, id)
}

// AddKeySource appends a new KeySource unless a duplicate of the same type
// already exists (panel: same panel/email/inbound triplet; manual: same link).
// The dedup check and append are atomic. Returns (existing, true) when a
// duplicate is found, otherwise (new, false).
func (s *Storage) AddKeySource(ks model.KeySource) (*model.KeySource, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sources, err := s.loadKeySourcesLocked()
	if err != nil {
		return nil, false, err
	}

	for i := range sources {
		existing := &sources[i]
		if ks.Type == "panel" {
			if existing.Type == "panel" && existing.PanelID == ks.PanelID &&
				existing.ClientEmail == ks.ClientEmail && existing.InboundID == ks.InboundID {
				dup := sources[i]
				return &dup, true, nil
			}
		} else {
			if existing.Type == "manual" && strings.TrimSpace(existing.VlessLink) == strings.TrimSpace(ks.VlessLink) {
				dup := sources[i]
				return &dup, true, nil
			}
		}
	}

	sources = append(sources, ks)
	if err := s.saveKeySourcesLocked(sources); err != nil {
		return nil, false, err
	}
	return &ks, false, nil
}

// UpdateKeySource replaces a KeySource by ID (atomic read-modify-write).
func (s *Storage) UpdateKeySource(updated model.KeySource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sources, err := s.loadKeySourcesLocked()
	if err != nil {
		return err
	}
	found := false
	for i := range sources {
		if sources[i].ID == updated.ID {
			sources[i] = updated
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrKeySourceNotFound, updated.ID)
	}
	return s.saveKeySourcesLocked(sources)
}

// UpdateKeySourceCaches обновляет CachedKey для нескольких KeySource в одном
// атомарном read-modify-write (одна запись файла). Несуществующие ID
// игнорируются. Используется при массовой регенерации, чтобы не перезаписывать
// key-sources.json по одному на каждый ключ.
func (s *Storage) UpdateKeySourceCaches(caches map[string]model.CachedKey) error {
	if len(caches) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	sources, err := s.loadKeySourcesLocked()
	if err != nil {
		return err
	}
	for i := range sources {
		if c, ok := caches[sources[i].ID]; ok {
			sources[i].CachedKey = &c
			sources[i].UpdatedAt = nowStr()
		}
	}
	return s.saveKeySourcesLocked(sources)
}

// DeleteKeySource removes a KeySource by ID (atomic read-modify-write).
func (s *Storage) DeleteKeySource(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	sources, err := s.loadKeySourcesLocked()
	if err != nil {
		return err
	}
	found := false
	for i := range sources {
		if sources[i].ID == id {
			sources = append(sources[:i], sources[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("%w: %s", ErrKeySourceNotFound, id)
	}
	return s.saveKeySourcesLocked(sources)
}

// --- Subscription metadata ---

// LoadSubscriptionsMeta reads subscription metadata (id/name/status/keys/updatedAt).
func (s *Storage) LoadSubscriptionsMeta() ([]model.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.subsMetaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Subscription{}, nil
		}
		return nil, fmt.Errorf("reading subscriptions meta: %w", err)
	}

	var subs []model.Subscription
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, fmt.Errorf("parsing subscriptions meta: %w", err)
	}
	normalizeSubKeys(subs)
	return subs, nil
}

// saveSubsMetaLocked writes subscriptions.json. Caller must hold Lock.
func (s *Storage) saveSubsMetaLocked(subs []model.Subscription) error {
	data, err := json.MarshalIndent(subs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling subscriptions meta: %w", err)
	}
	if err := os.WriteFile(s.subsMetaPath, data, 0644); err != nil {
		return fmt.Errorf("writing subscriptions meta: %w", err)
	}
	return nil
}

// GetSubMeta returns subscription metadata by name.
func (s *Storage) GetSubMeta(name string) (*model.Subscription, bool) {
	subs, err := s.LoadSubscriptionsMeta()
	if err != nil {
		return nil, false
	}
	for i := range subs {
		if subs[i].Name == name {
			sub := subs[i]
			return &sub, true
		}
	}
	return nil, false
}

// UpsertSubMeta inserts or replaces subscription metadata by name (atomic).
func (s *Storage) UpsertSubMeta(sub model.Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	subs, err := s.loadSubsMetaLocked()
	if err != nil {
		return err
	}
	found := false
	for i := range subs {
		if subs[i].Name == sub.Name {
			subs[i] = sub
			found = true
			break
		}
	}
	if !found {
		subs = append(subs, sub)
	}
	return s.saveSubsMetaLocked(subs)
}

// DeleteSubMeta removes subscription metadata by name (atomic).
func (s *Storage) DeleteSubMeta(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	subs, err := s.loadSubsMetaLocked()
	if err != nil {
		return err
	}
	found := false
	for i := range subs {
		if subs[i].Name == name {
			subs = append(subs[:i], subs[i+1:]...)
			found = true
			break
		}
	}
	if !found {
		return nil
	}
	return s.saveSubsMetaLocked(subs)
}

// --- API tokens ---

// loadTokensLocked reads tokens.json. Caller must hold at least RLock.
func (s *Storage) loadTokensLocked() ([]model.APIToken, error) {
	data, err := os.ReadFile(s.tokensPath)
	if err != nil {
		return nil, fmt.Errorf("reading tokens file: %w", err)
	}
	var tokens []model.APIToken
	if err := json.Unmarshal(data, &tokens); err != nil {
		return nil, fmt.Errorf("parsing tokens file: %w", err)
	}
	return tokens, nil
}

// saveTokensLocked writes tokens.json. Caller must hold Lock.
func (s *Storage) saveTokensLocked(tokens []model.APIToken) error {
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling tokens: %w", err)
	}
	if err := os.WriteFile(s.tokensPath, data, 0644); err != nil {
		return fmt.Errorf("writing tokens file: %w", err)
	}
	return nil
}

// LoadTokens reads all issued API tokens.
func (s *Storage) LoadTokens() ([]model.APIToken, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.loadTokensLocked()
}

// AddToken appends a new API token (atomic).
func (s *Storage) AddToken(tok model.APIToken) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tokens, err := s.loadTokensLocked()
	if err != nil {
		return err
	}
	tokens = append(tokens, tok)
	return s.saveTokensLocked(tokens)
}

// DeleteToken removes an API token by ID and returns it (atomic). The caller
// uses the returned TokenHash to drop it from the in-memory auth cache.
func (s *Storage) DeleteToken(id string) (model.APIToken, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tokens, err := s.loadTokensLocked()
	if err != nil {
		return model.APIToken{}, err
	}
	for i := range tokens {
		if tokens[i].ID == id {
			tok := tokens[i]
			tokens = append(tokens[:i], tokens[i+1:]...)
			if err := s.saveTokensLocked(tokens); err != nil {
				return model.APIToken{}, err
			}
			return tok, nil
		}
	}
	return model.APIToken{}, fmt.Errorf("%w: %s", ErrTokenNotFound, id)
}

// --- Subscription files ---

// subscriptionFile returns the path for a subscription file. The name is
// validated so it cannot escape aggregatorDir (defense in depth: read/delete
// handlers already validate, but no future caller may silently bypass it).
func (s *Storage) subscriptionFile(name string) (string, error) {
	if !validSubscriptionName(name) {
		return "", fmt.Errorf("%w: %s", ErrInvalidSubscriptionName, name)
	}
	return filepath.Join(s.aggregatorDir, "configs-"+name+".txt"), nil
}

// SubscriptionFileExists reports whether the file for name exists.
// Invalid names report false.
func (s *Storage) SubscriptionFileExists(name string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, err := s.subscriptionFile(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// WriteSubscriptionFile writes the subscription file from key links.
func (s *Storage) WriteSubscriptionFile(name string, keys []model.SubKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.subscriptionFile(name)
	if err != nil {
		return err
	}

	var lines []string
	for _, k := range keys {
		lines = append(lines, k.Link)
	}
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing subscription file: %w", err)
	}
	return nil
}

// RenameSubscriptionFile renames configs-{old}.txt → configs-{new}.txt.
func (s *Storage) RenameSubscriptionFile(oldName, newName string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	oldPath, err := s.subscriptionFile(oldName)
	if err != nil {
		return err
	}
	newPath, err := s.subscriptionFile(newName)
	if err != nil {
		return err
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("renaming subscription file: %w", err)
	}
	return nil
}

// RemoveSubscriptionFile deletes the file for name (missing file is not an error).
func (s *Storage) RemoveSubscriptionFile(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.subscriptionFile(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing subscription file: %w", err)
	}
	return nil
}

// SubscriptionFileMtime returns the file mtime as RFC3339, or "" if missing
// or the name is invalid.
func (s *Storage) SubscriptionFileMtime(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, err := s.subscriptionFile(name)
	if err != nil {
		return ""
	}
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")
}

// ListSubscriptions returns all subscriptions, merging metadata (KeySource refs,
// status) with the actual files. Legacy files without metadata are reported with
// manual keys (keySourceId=null) and status active.
func (s *Storage) ListSubscriptions() ([]model.Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metas := map[string]model.Subscription{}
	metaList, err := s.loadSubsMetaLocked()
	if err != nil {
		return nil, err
	}
	for _, m := range metaList {
		metas[m.Name] = m
	}

	entries, err := os.ReadDir(s.aggregatorDir)
	if err != nil {
		return nil, fmt.Errorf("reading aggregator dir: %w", err)
	}

	var subs []model.Subscription
	seen := map[string]bool{}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "configs-") || !strings.HasSuffix(name, ".txt") {
			continue
		}
		clientName := strings.TrimPrefix(name, "configs-")
		clientName = strings.TrimSuffix(clientName, ".txt")
		if clientName == "" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.aggregatorDir, name))
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

		if meta, ok := metas[clientName]; ok {
			// Pair meta keys with file lines by index (file is truth for links).
			keys := make([]model.SubKey, 0, len(lines)+2)
			for i, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if i < len(meta.Keys) {
					k := meta.Keys[i]
					k.Link = line
					keys = append(keys, k)
				} else {
					keys = append(keys, model.SubKey{ID: fmt.Sprintf("k-%d", i+1), Link: line})
				}
			}
			// Meta keys beyond the file lines (e.g. KeySource added but not yet
			// generated) are kept as-is with their stored (possibly empty) link.
			if len(meta.Keys) > len(lines) {
				for i := len(lines); i < len(meta.Keys); i++ {
					keys = append(keys, meta.Keys[i])
				}
			}
			sub := meta
			if sub.ID == "" {
				sub.ID = clientName
			}
			sub.Name = clientName
			sub.Keys = keys
			if sub.Status == "" {
				sub.Status = "active"
			}
			info, _ := entry.Info()
			if info != nil {
				sub.FileMtime = info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			subs = append(subs, sub)
		} else {
			// Legacy subscription: no metadata, all keys manual.
			keys := make([]model.SubKey, 0, len(lines))
			for i, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				keys = append(keys, model.SubKey{ID: fmt.Sprintf("k-%d", i+1), Link: line})
			}
			info, _ := entry.Info()
			mtime := ""
			if info != nil {
				mtime = info.ModTime().UTC().Format("2006-01-02T15:04:05Z07:00")
			}
			subs = append(subs, model.Subscription{
				ID:        clientName,
				Name:      clientName,
				Status:    "active",
				Keys:      keys,
				UpdatedAt: mtime,
				FileMtime: mtime,
			})
		}
		seen[clientName] = true
	}

	// Metadata without files → drafts (or active with missing file).
	for name, meta := range metas {
		if seen[name] {
			continue
		}
		sub := meta
		if sub.ID == "" {
			sub.ID = name
		}
		sub.Name = name
		if sub.Status == "" {
			sub.Status = "draft"
		}
		sub.FileMtime = ""
		subs = append(subs, sub)
	}

	sort.Slice(subs, func(i, j int) bool {
		return strings.ToLower(subs[i].Name) < strings.ToLower(subs[j].Name)
	})

	return subs, nil
}

// normalizeSubKeys ensures Keys is never nil so JSON serializes as [] not null.
func normalizeSubKeys(subs []model.Subscription) {
	for i := range subs {
		if subs[i].Keys == nil {
			subs[i].Keys = []model.SubKey{}
		}
	}
}

// loadSubsMetaLocked reads subscriptions.json (caller must hold at least RLock).
func (s *Storage) loadSubsMetaLocked() ([]model.Subscription, error) {
	data, err := os.ReadFile(s.subsMetaPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []model.Subscription{}, nil
		}
		return nil, fmt.Errorf("reading subscriptions meta: %w", err)
	}
	var subs []model.Subscription
	if err := json.Unmarshal(data, &subs); err != nil {
		return nil, fmt.Errorf("parsing subscriptions meta: %w", err)
	}
	normalizeSubKeys(subs)
	return subs, nil
}

// GetSubscription returns a single subscription by name
func (s *Storage) GetSubscription(name string) (*model.Subscription, error) {
	subs, err := s.ListSubscriptions()
	if err != nil {
		return nil, err
	}

	for _, sub := range subs {
		if sub.ID == name || sub.Name == name {
			return &sub, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", ErrSubscriptionNotFound, name)
}

// GetSubscriptionRaw returns the raw content of a subscription file
func (s *Storage) GetSubscriptionRaw(name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath, err := s.subscriptionFile(name)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrSubscriptionNotFound, name)
		}
		return "", fmt.Errorf("reading subscription file: %w", err)
	}

	return string(data), nil
}
