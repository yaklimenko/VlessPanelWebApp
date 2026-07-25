package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Storage handles file-based data persistence
type Storage struct {
	mu           sync.RWMutex
	panelsPath   string
	aggregatorDir string
}

// NewStorage creates a new Storage instance
func NewStorage(panelsPath, aggregatorDir string) *Storage {
	// Ensure panels.json exists
	if _, err := os.Stat(panelsPath); os.IsNotExist(err) {
		initial := []Panel{}
		data, _ := json.MarshalIndent(initial, "", "  ")
		os.WriteFile(panelsPath, data, 0644)
	}
	// Ensure aggregator dir exists
	os.MkdirAll(aggregatorDir, 0755)

	return &Storage{
		panelsPath:    panelsPath,
		aggregatorDir: aggregatorDir,
	}
}

// --- Panels ---

// LoadPanels reads all panels from panels.json
func (s *Storage) LoadPanels() ([]Panel, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data, err := os.ReadFile(s.panelsPath)
	if err != nil {
		return nil, fmt.Errorf("reading panels file: %w", err)
	}

	var panels []Panel
	if err := json.Unmarshal(data, &panels); err != nil {
		return nil, fmt.Errorf("parsing panels file: %w", err)
	}

	return panels, nil
}

// SavePanels writes panels to panels.json
func (s *Storage) SavePanels(panels []Panel) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(panels, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling panels: %w", err)
	}

	if err := os.WriteFile(s.panelsPath, data, 0644); err != nil {
		return fmt.Errorf("writing panels file: %w", err)
	}

	return nil
}

// AddPanel adds a new panel and returns it with an assigned ID
func (s *Storage) AddPanel(req CreatePanelRequest) (Panel, error) {
	panels, err := s.LoadPanels()
	if err != nil {
		return Panel{}, err
	}

	// Generate ID from name
	id := strings.ToLower(strings.ReplaceAll(req.Name, " ", "-"))
	// Make unique
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

	panel := Panel{
		ID:          id,
		Name:        req.Name,
		URL:         req.URL,
		Token:       req.Token,
		WebBasePath: req.WebBasePath,
	}

	panels = append(panels, panel)
	if err := s.SavePanels(panels); err != nil {
		return Panel{}, err
	}

	return panel, nil
}

// DeletePanel removes a panel by ID
func (s *Storage) DeletePanel(id string) error {
	panels, err := s.LoadPanels()
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
		return fmt.Errorf("panel %s not found", id)
	}

	return s.SavePanels(panels)
}

// --- Subscriptions (files in aggregator directory) ---

// ListSubscriptions returns all subscription files
func (s *Storage) ListSubscriptions() ([]Subscription, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.aggregatorDir)
	if err != nil {
		return nil, fmt.Errorf("reading aggregator dir: %w", err)
	}

	var subs []Subscription
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "config-") || !strings.HasSuffix(name, ".txt") {
			continue
		}

		// Extract client name: config-{ClientName}.txt
		clientName := strings.TrimPrefix(name, "config-")
		clientName = strings.TrimSuffix(clientName, ".txt")

		sub := Subscription{
			ID:   clientName,
			Name: clientName,
			Link: "", // Link isn't stored in the file
		}

		// Read keys from file
		data, err := os.ReadFile(filepath.Join(s.aggregatorDir, name))
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		keys := make([]SubKey, 0)
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			keys = append(keys, SubKey{
				ID:   fmt.Sprintf("k-%d", len(keys)+1),
				Link: line,
			})
		}
		sub.Keys = keys
		subs = append(subs, sub)
	}

	return subs, nil
}

// GetSubscription returns a single subscription by name
func (s *Storage) GetSubscription(name string) (*Subscription, error) {
	subs, err := s.ListSubscriptions()
	if err != nil {
		return nil, err
	}

	for _, sub := range subs {
		if sub.ID == name || sub.Name == name {
			return &sub, nil
		}
	}

	return nil, fmt.Errorf("subscription %s not found", name)
}

// CreateSubscription creates a new subscription file
func (s *Storage) CreateSubscription(req CreateSubscriptionRequest) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.aggregatorDir, "config-"+req.Name+".txt")

	var lines []string
	for _, k := range req.Keys {
		lines = append(lines, k.Link)
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return Subscription{}, fmt.Errorf("writing subscription file: %w", err)
	}

	return Subscription{
		ID:   req.Name,
		Name: req.Name,
		Keys: req.Keys,
	}, nil
}

// UpdateSubscription updates a subscription file
func (s *Storage) UpdateSubscription(name string, req UpdateSubscriptionRequest) (Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.aggregatorDir, "config-"+name+".txt")

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return Subscription{}, fmt.Errorf("subscription %s not found", name)
	}

	var lines []string
	for _, k := range req.Keys {
		lines = append(lines, k.Link)
	}

	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return Subscription{}, fmt.Errorf("writing subscription file: %w", err)
	}

	newName := name
	if req.Name != "" {
		newName = req.Name
		// Rename file
		newPath := filepath.Join(s.aggregatorDir, "config-"+req.Name+".txt")
		if err := os.Rename(filePath, newPath); err != nil {
			return Subscription{}, fmt.Errorf("renaming subscription file: %w", err)
		}
	}

	return Subscription{
		ID:   newName,
		Name: newName,
		Keys: req.Keys,
	}, nil
}

// DeleteSubscription removes a subscription file
func (s *Storage) DeleteSubscription(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.aggregatorDir, "config-"+name+".txt")
	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("subscription %s not found", name)
		}
		return fmt.Errorf("removing subscription file: %w", err)
	}

	return nil
}

// GetSubscriptionRaw returns the raw content of a subscription file
func (s *Storage) GetSubscriptionRaw(name string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := filepath.Join(s.aggregatorDir, "config-"+name+".txt")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("subscription %s not found", name)
		}
		return "", fmt.Errorf("reading subscription file: %w", err)
	}

	return string(data), nil
}
