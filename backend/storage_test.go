package main

import (
	"fmt"
	"sync"
	"testing"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	dir := t.TempDir()
	return NewStorage(dir+"/panels.json", dir+"/agg", dir+"/data")
}

// AddKeySource должен атомарно проверять дедупликацию: из N конкурентных
// запросов с одинаковой ссылкой ровно один создаёт запись, остальные
// получают существующую.
func TestAddKeySourceConcurrentDedup(t *testing.T) {
	s := newTestStorage(t)
	const n = 50
	ks := KeySource{
		ID:        "ks-test",
		Type:      "manual",
		VlessLink: "vless://test@example.com:443?security=reality",
		Label:     "test",
	}

	var wg sync.WaitGroup
	deduped := make([]bool, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, dedup, err := s.AddKeySource(ks)
			deduped[i] = dedup
			errs[i] = err
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
	}

	created := 0
	for _, d := range deduped {
		if !d {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("expected exactly 1 created, got %d", created)
	}

	sources, err := s.LoadKeySources()
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 key source persisted, got %d", len(sources))
	}
}

// AddPanel должен атомарно генерировать уникальные ID: N конкурентных
// добавлений с одинаковым именем дают N панелей с уникальными ID.
func TestAddPanelConcurrentUniqueID(t *testing.T) {
	s := newTestStorage(t)
	const n = 20

	var wg sync.WaitGroup
	panels := make([]Panel, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := s.AddPanel(CreatePanelRequest{Name: "Test Panel", URL: "https://x", Token: "t"})
			panels[i] = p
			errs[i] = err
		}(i)
	}
	wg.Wait()

	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
		if seen[panels[i].ID] {
			t.Fatalf("duplicate panel ID %q", panels[i].ID)
		}
		seen[panels[i].ID] = true
	}

	all, err := s.LoadPanels()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != n {
		t.Fatalf("expected %d panels persisted, got %d", n, len(all))
	}
}

// UpsertSubMeta разных подписок конкурентно не должен терять записи
// (классический read-modify-write lost update).
func TestUpsertSubMetaConcurrentNoLostUpdate(t *testing.T) {
	s := newTestStorage(t)
	const n = 20

	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = s.UpsertSubMeta(Subscription{
				Name:   fmt.Sprintf("sub-%d", i),
				Status: "active",
			})
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if errs[i] != nil {
			t.Fatalf("goroutine %d: %v", i, errs[i])
		}
	}

	subs, err := s.LoadSubscriptionsMeta()
	if err != nil {
		t.Fatal(err)
	}
	if len(subs) != n {
		t.Fatalf("expected %d subscriptions persisted, got %d (lost update)", n, len(subs))
	}
}
