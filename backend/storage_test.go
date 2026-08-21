package main

import (
	"fmt"
	"os"
	"sync"
	"testing"

	"vlesspanel/dto"
	"vlesspanel/model"
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
	ks := model.KeySource{
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
	panels := make([]model.Panel, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, err := s.AddPanel(dto.CreatePanelRequest{Name: "Test Panel", URL: "https://x", Token: "t"})
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
			errs[i] = s.UpsertSubMeta(model.Subscription{
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

// Имена, способные выйти за пределы aggregatorDir (path traversal), должны
// отвергаться на уровне storage (defense in depth), не трогая файлы вне каталога.
func TestSubscriptionFileRejectsUnsafeNames(t *testing.T) {
	dir := t.TempDir()
	aggDir := dir + "/agg"
	s := NewStorage(dir+"/panels.json", aggDir, dir+"/data")

	victim := dir + "/victim.txt"
	if err := os.WriteFile(victim, []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}

	unsafe := []string{
		"/../../victim",
		"../victim",
		"a/b",
		"..",
		".",
		"a..b",
		"",
		"x/",
	}
	for _, name := range unsafe {
		if _, err := s.GetSubscriptionRaw(name); err == nil {
			t.Errorf("GetSubscriptionRaw(%q) should be rejected", name)
		}
		if err := s.WriteSubscriptionFile(name, []model.SubKey{{Link: "vless://x"}}); err == nil {
			t.Errorf("WriteSubscriptionFile(%q) should be rejected", name)
		}
		if err := s.RemoveSubscriptionFile(name); err == nil {
			t.Errorf("RemoveSubscriptionFile(%q) should be rejected", name)
		}
		if s.SubscriptionFileExists(name) {
			t.Errorf("SubscriptionFileExists(%q) should be false", name)
		}
	}

	data, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "secret" {
		t.Fatalf("victim file was modified/deleted: %q", string(data))
	}

	// Валидное имя по-прежнему работает.
	if err := s.WriteSubscriptionFile("ok-name", []model.SubKey{{Link: "vless://x"}}); err != nil {
		t.Fatalf("valid name rejected: %v", err)
	}
	if raw, err := s.GetSubscriptionRaw("ok-name"); err != nil || raw != "vless://x\n" {
		t.Fatalf("valid read failed: %q, %v", raw, err)
	}
}

// UpdateKeySourceCaches должен обновить кэш нескольких model.KeySource одной атомарной
// записью и не трогать отсутствующие ID.
func TestUpdateKeySourceCaches(t *testing.T) {
	s := newTestStorage(t)

	mk := func(id string) {
		_, _, err := s.AddKeySource(model.KeySource{ID: id, Type: "manual", VlessLink: "vless://" + id + "@x", Label: id})
		if err != nil {
			t.Fatal(err)
		}
	}
	mk("a")
	mk("b")

	err := s.UpdateKeySourceCaches(map[string]model.CachedKey{
		"a":     {Link: "vless://a@x"},
		"b":     {Link: "vless://b@x"},
		"ghost": {Link: "vless://ghost@x"},
	})
	if err != nil {
		t.Fatal(err)
	}

	a, err := s.GetKeySource("a")
	if err != nil || a.CachedKey == nil || a.CachedKey.Link != "vless://a@x" {
		t.Fatalf("a cache not updated: %+v, %v", a.CachedKey, err)
	}
	b, err := s.GetKeySource("b")
	if err != nil || b.CachedKey == nil || b.CachedKey.Link != "vless://b@x" {
		t.Fatalf("b cache not updated: %+v, %v", b.CachedKey, err)
	}
	if _, err := s.GetKeySource("ghost"); err == nil {
		t.Fatalf("ghost should not have been created")
	}
}
