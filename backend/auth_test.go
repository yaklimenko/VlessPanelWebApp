package main

import (
	"errors"
	"testing"
)

func TestTokenAuth(t *testing.T) {
	// Отключено: пустой admin-токен разрешает всё с ролью admin.
	a := NewTokenAuth("")
	if a.Enabled() {
		t.Fatal("empty admin token should disable auth")
	}
	if role, ok := a.Authenticate("whatever"); !ok || role != "admin" {
		t.Fatal("disabled auth should allow all as admin")
	}

	// Включено.
	a = NewTokenAuth("secret-admin")
	if !a.Enabled() {
		t.Fatal("admin token should enable auth")
	}
	if role, ok := a.Authenticate("secret-admin"); !ok || role != "admin" {
		t.Fatal("admin token rejected")
	}
	if _, ok := a.Authenticate("wrong"); ok {
		t.Fatal("wrong token accepted")
	}

	// Выпущенный токен.
	raw := "vlt_test123"
	a.AddIssued(hashToken(raw))
	if role, ok := a.Authenticate(raw); !ok || role != "issued" {
		t.Fatal("issued token rejected")
	}

	// Отзыв.
	a.RemoveIssued(hashToken(raw))
	if _, ok := a.Authenticate(raw); ok {
		t.Fatal("revoked token still accepted")
	}
}

func TestStorageTokens(t *testing.T) {
	s := newTestStorage(t)

	if err := s.AddToken(APIToken{ID: "tok-1", Label: "bot", TokenHash: "hash1", CreatedAt: "now"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddToken(APIToken{ID: "tok-2", Label: "agent", TokenHash: "hash2", CreatedAt: "now"}); err != nil {
		t.Fatal(err)
	}

	tokens, err := s.LoadTokens()
	if err != nil || len(tokens) != 2 {
		t.Fatalf("expected 2 tokens, got %d (err %v)", len(tokens), err)
	}

	tok, err := s.DeleteToken("tok-1")
	if err != nil || tok.ID != "tok-1" || tok.TokenHash != "hash1" {
		t.Fatalf("DeleteToken returned %+v, %v", tok, err)
	}

	tokens, _ = s.LoadTokens()
	if len(tokens) != 1 || tokens[0].ID != "tok-2" {
		t.Fatalf("expected 1 remaining token, got %+v", tokens)
	}

	if _, err := s.DeleteToken("tok-1"); err == nil || !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound, got %v", err)
	}
}
