package main

import (
	"testing"

	"vlesspanel/dto"
	"vlesspanel/model"
)

func TestKeySourceServiceCreateValidation(t *testing.T) {
	s := newTestStorage(t)
	svc := NewKeySourceService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	if _, err := svc.Create(dto.CreateKeySourceRequest{Type: "bogus"}); appErrStatus(t, err) != 400 {
		t.Fatalf("bad type should be 400, got %v", err)
	}
	if _, err := svc.Create(dto.CreateKeySourceRequest{Type: "manual", VlessLink: "not-vless"}); appErrStatus(t, err) != 400 {
		t.Fatalf("manual without vless:// should be 400, got %v", err)
	}
	if _, err := svc.Create(dto.CreateKeySourceRequest{Type: "panel"}); appErrStatus(t, err) != 400 {
		t.Fatalf("panel missing fields should be 400, got %v", err)
	}
}

func TestKeySourceServiceCreateManualLabel(t *testing.T) {
	s := newTestStorage(t)
	svc := NewKeySourceService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	resp, err := svc.Create(dto.CreateKeySourceRequest{Type: "manual", VlessLink: "vless://u@x:443?security=reality#my-label"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Deduped {
		t.Fatal("should not dedup")
	}
	if resp.KeySource.Label != "my-label" {
		t.Fatalf("label = %q, want my-label", resp.KeySource.Label)
	}
	if _, err := s.GetKeySource(resp.KeySource.ID); err != nil {
		t.Fatalf("key source not persisted: %v", err)
	}
}

func TestKeySourceServiceCreateDedup(t *testing.T) {
	s := newTestStorage(t)
	svc := NewKeySourceService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	link := "vless://u@x:443?security=reality#dup"
	if _, err := svc.Create(dto.CreateKeySourceRequest{Type: "manual", VlessLink: link}); err != nil {
		t.Fatal(err)
	}
	resp, err := svc.Create(dto.CreateKeySourceRequest{Type: "manual", VlessLink: link})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Deduped {
		t.Fatal("second create should dedup")
	}
}

func TestKeySourceServiceGetKeyManual(t *testing.T) {
	s := newTestStorage(t)
	ks, _, err := s.AddKeySource(model.KeySource{ID: "ks-1", Type: "manual", VlessLink: "vless://m@x:443", Label: "m"})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewKeySourceService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})

	resp, err := svc.GetKey(ks.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Key.Link != "vless://m@x:443" {
		t.Fatalf("link = %q", resp.Key.Link)
	}
}

func TestKeySourceServiceDeleteCascade(t *testing.T) {
	s := newTestStorage(t)
	ks, _, err := s.AddKeySource(model.KeySource{ID: "ks-1", Type: "manual", VlessLink: "vless://m@x:443", Label: "m"})
	if err != nil {
		t.Fatal(err)
	}

	subSvc := NewSubscriptionService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})
	if _, err := subSvc.Create(dto.CreateSubscriptionRequest{Name: "sub1", KeySourceIDs: []string{ks.ID}}); err != nil {
		t.Fatal(err)
	}

	ksSvc := NewKeySourceService(s, &fakePanelClient{}, NewSyncState(), &fakeDaemon{})
	resp, err := ksSvc.Delete(ks.ID)
	if err != nil {
		t.Fatal(err)
	}
	if resp.UsedInSubscriptions != 1 || len(resp.Subscriptions) != 1 {
		t.Fatalf("resp = %+v", resp)
	}

	sub, err := s.GetSubscription("sub1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub.Keys) != 0 || sub.Status != "draft" {
		t.Fatalf("sub should be draft/empty after cascade, got %+v", sub)
	}
}
