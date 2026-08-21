package main

import (
	"errors"
	"testing"

	"vlesspanel/dto"
	"vlesspanel/xui"
)

func TestPanelServiceCreateValidation(t *testing.T) {
	s := newTestStorage(t)
	svc := NewPanelService(s, &fakePanelClient{})

	if _, err := svc.Create(dto.CreatePanelRequest{Name: "x"}); appErrStatus(t, err) != 400 {
		t.Fatalf("missing url/token should be 400, got %v", err)
	}
}

func TestPanelServiceCreate(t *testing.T) {
	s := newTestStorage(t)
	svc := NewPanelService(s, &fakePanelClient{})

	panel, err := svc.Create(dto.CreatePanelRequest{Name: "Test Panel", URL: "https://x:1", Token: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if panel.ID != "test-panel" {
		t.Fatalf("id = %q, want test-panel", panel.ID)
	}
	if _, err := s.GetPanel(panel.ID); err != nil {
		t.Fatalf("panel not persisted: %v", err)
	}
}

func TestPanelServiceListClientsNotFound(t *testing.T) {
	s := newTestStorage(t)
	svc := NewPanelService(s, &fakePanelClient{})

	if _, err := svc.ListClients("nope"); !errors.Is(err, ErrPanelNotFound) {
		t.Fatalf("expected ErrPanelNotFound, got %v", err)
	}
}

func TestPanelServiceListInbounds(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.AddPanel(dto.CreatePanelRequest{Name: "P", URL: "https://x:1", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	fake := &fakePanelClient{
		inbounds: []xui.XUIInbound{{ID: 5, Port: 11471, Remark: "PL1", Protocol: "vless", Enable: true}},
	}
	svc := NewPanelService(s, fake)

	ibs, err := svc.ListInbounds("p")
	if err != nil {
		t.Fatal(err)
	}
	if len(ibs) != 1 || ibs[0].Remark != "PL1" || ibs[0].Port != 11471 {
		t.Fatalf("ibs = %+v", ibs)
	}
}

func TestPanelServiceCreateClientValidation(t *testing.T) {
	s := newTestStorage(t)
	svc := NewPanelService(s, &fakePanelClient{})

	if _, err := svc.CreateClient("p", dto.CreateClientRequest{}); appErrStatus(t, err) != 400 {
		t.Fatalf("missing email/inboundId should be 400, got %v", err)
	}
}

func TestPanelServiceUpdateClientBadDate(t *testing.T) {
	s := newTestStorage(t)
	if _, err := s.AddPanel(dto.CreatePanelRequest{Name: "P", URL: "https://x:1", Token: "t"}); err != nil {
		t.Fatal(err)
	}
	svc := NewPanelService(s, &fakePanelClient{})

	_, err := svc.UpdateClient("p", "e@x", dto.UpdateClientRequest{ExpiryDate: "not-a-date"})
	if appErrStatus(t, err) != 400 {
		t.Fatalf("bad expiryDate should be 400, got %v", err)
	}
}
