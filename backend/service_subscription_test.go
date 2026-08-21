package main

import (
	"strings"
	"testing"

	"vlesspanel/dto"
	"vlesspanel/model"
	"vlesspanel/xui"
)

func TestSubscriptionServiceCreateValidation(t *testing.T) {
	s := newTestStorage(t)
	svc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	if _, err := svc.Create(dto.CreateSubscriptionRequest{Name: "  "}); appErrStatus(t, err) != 400 {
		t.Fatalf("empty name should be 400, got %v", err)
	}
	if _, err := svc.Create(dto.CreateSubscriptionRequest{Name: "bad/name"}); appErrStatus(t, err) != 400 {
		t.Fatalf("invalid name should be 400, got %v", err)
	}
}

func TestSubscriptionServiceCreateDuplicate(t *testing.T) {
	s := newTestStorage(t)
	svc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	req := dto.CreateSubscriptionRequest{Name: "sub1", Keys: []model.SubKey{{ID: "k1", Link: "vless://x"}}}
	if _, err := svc.Create(req); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(req); appErrStatus(t, err) != 409 {
		t.Fatalf("duplicate should be 409, got %v", err)
	}
}

func TestSubscriptionServiceCreateManual(t *testing.T) {
	s := newTestStorage(t)
	ks, _, err := s.AddKeySource(model.KeySource{ID: "ks-1", Type: "manual", VlessLink: "vless://u@example.com:443?security=reality#m", Label: "m"})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	resp, err := svc.Create(dto.CreateSubscriptionRequest{Name: "sub1", KeySourceIDs: []string{ks.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Subscription.Status != "active" || len(resp.Subscription.Keys) != 1 {
		t.Fatalf("sub = %+v", resp.Subscription)
	}
	if resp.Included != 1 || resp.Skipped != 0 {
		t.Fatalf("included=%d skipped=%d", resp.Included, resp.Skipped)
	}
	if !s.SubscriptionFileExists("sub1") {
		t.Fatal("file not written")
	}
	raw, _ := s.GetSubscriptionRaw("sub1")
	if !strings.Contains(raw, "vless://u@example.com:443") {
		t.Fatalf("raw = %q", raw)
	}
}

func TestSubscriptionServiceUpdateRename(t *testing.T) {
	s := newTestStorage(t)
	svc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})
	if _, err := svc.Create(dto.CreateSubscriptionRequest{Name: "sub1", Keys: []model.SubKey{{ID: "k1", Link: "vless://x"}}}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Update("sub1", dto.UpdateSubscriptionRequest{Name: "sub2"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Subscription.Name != "sub2" {
		t.Fatalf("name = %q", res.Subscription.Name)
	}
	if !s.SubscriptionFileExists("sub2") {
		t.Fatal("file sub2 not exists")
	}
	if s.SubscriptionFileExists("sub1") {
		t.Fatal("file sub1 still exists")
	}
}

func TestSubscriptionServiceUpdateRemoveKey(t *testing.T) {
	s := newTestStorage(t)
	svc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})
	_, err := svc.Create(dto.CreateSubscriptionRequest{Name: "sub1", Keys: []model.SubKey{{ID: "k1", Link: "vless://a"}, {ID: "k2", Link: "vless://b"}}})
	if err != nil {
		t.Fatal(err)
	}

	res, err := svc.Update("sub1", dto.UpdateSubscriptionRequest{RemoveKeyID: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Subscription.Keys) != 1 || res.Subscription.Keys[0].ID != "k2" {
		t.Fatalf("keys = %+v", res.Subscription.Keys)
	}
}

func TestSubscriptionServiceDelete(t *testing.T) {
	s := newTestStorage(t)
	svc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})
	if _, err := svc.Create(dto.CreateSubscriptionRequest{Name: "sub1", Keys: []model.SubKey{{ID: "k1", Link: "vless://x"}}}); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete("sub1"); err != nil {
		t.Fatal(err)
	}
	if s.SubscriptionFileExists("sub1") {
		t.Fatal("file still exists after delete")
	}
	if _, err := s.GetSubscription("sub1"); err == nil {
		t.Fatal("subscription still exists after delete")
	}
}

func TestSubscriptionServiceDeleteInvalidName(t *testing.T) {
	s := newTestStorage(t)
	svc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	if err := svc.Delete("bad/name"); appErrStatus(t, err) != 400 {
		t.Fatalf("invalid name should be 400, got %v", err)
	}
}

func TestSubscriptionServiceRegenerateAllPanel(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.AddPanel(dto.CreatePanelRequest{Name: "P", URL: "https://x:1", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	ks, _, err := s.AddKeySource(model.KeySource{ID: "ks-1", Type: "panel", PanelID: "p", ClientEmail: "e@x", InboundID: 5, Label: "ks1"})
	if err != nil {
		t.Fatal(err)
	}

	fake := &fakePanelClient{
		inbounds: []xui.XUIInbound{{ID: 5, Port: 11471, Remark: "PL1", Protocol: "vless"}},
		keysByEmail: map[string][]model.VLESSKey{
			"e@x": {{Label: "PL1-e@x", Port: 11471, Link: "vless://fresh-key", Security: "reality", Transport: "tcp"}},
		},
	}
	svc := NewSubscriptionService(s, fake, NewSyncState(), &fakeDaemon{})

	if _, err := svc.Create(dto.CreateSubscriptionRequest{Name: "sub1", KeySourceIDs: []string{ks.ID}}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.RegenerateAll()
	if err != nil {
		t.Fatal(err)
	}
	if res.Regenerated != 1 || res.Skipped != 0 {
		t.Fatalf("regenerated=%d skipped=%d", res.Regenerated, res.Skipped)
	}

	raw, _ := s.GetSubscriptionRaw("sub1")
	if !strings.Contains(raw, "vless://fresh-key") {
		t.Fatalf("raw = %q, want fresh key", raw)
	}
}
