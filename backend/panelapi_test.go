package main

import (
	"encoding/json"
	"strings"
	"testing"

	"vlesspanel/model"
	"vlesspanel/xui"
)

// buildClientKeys (чистая функция) должен строить VLESS-ключ из инбаунда +
// клиента, с корректным host-извлечением и параметрами REALITY.
func TestBuildClientKeys(t *testing.T) {
	panel := model.Panel{ID: "p", URL: "https://203.0.113.3:5867"}
	inbound := xui.XUIInbound{
		ID:       5,
		Remark:   "PL1",
		Port:     11471,
		Protocol: "vless",
		Settings: json.RawMessage(`{"clients":[{"id":"client-uuid","email":"test@x","flow":"xtls-rprx-vision"}]}`),
	}

	keys := buildClientKeys(panel, []xui.XUIInbound{inbound}, nil, "test@x")

	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	k := keys[0]
	if k.Port != 11471 {
		t.Errorf("port = %d, want 11471", k.Port)
	}
	if k.Inbound != "PL1" {
		t.Errorf("inbound = %q, want PL1", k.Inbound)
	}
	if k.Protocol != "vless" {
		t.Errorf("protocol = %q, want vless", k.Protocol)
	}
	if k.Security != "reality" {
		t.Errorf("security = %q, want reality", k.Security)
	}
	if k.Transport != "tcp" {
		t.Errorf("transport = %q, want tcp", k.Transport)
	}
	if !strings.HasPrefix(k.Link, "vless://client-uuid@203.0.113.3:11471?") {
		t.Errorf("link = %q, want prefix vless://client-uuid@203.0.113.3:11471?", k.Link)
	}
	if !strings.Contains(k.Link, "flow=xtls-rprx-vision") {
		t.Errorf("link %q missing flow param", k.Link)
	}
	if !strings.Contains(k.Link, "security=reality") {
		t.Errorf("link %q missing security=reality", k.Link)
	}
}
