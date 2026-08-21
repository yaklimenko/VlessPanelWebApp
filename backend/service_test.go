package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"vlesspanel/dto"
	"vlesspanel/model"
)

// --- fakes ---

// fakeRepo — минимальный фейк Repository (embedded-interface trick: не
// переопределённые методы паникуют при вызове).
type fakeRepo struct {
	Repository
	tokens []model.APIToken
}

func (f *fakeRepo) AddToken(tok model.APIToken) error {
	f.tokens = append(f.tokens, tok)
	return nil
}

func (f *fakeRepo) LoadTokens() ([]model.APIToken, error) {
	return f.tokens, nil
}

// fakeSyncer — фейк AggregatorSyncer.
type fakeSyncer struct {
	out string
	err error
}

func (f *fakeSyncer) Sync(ctx context.Context) (string, error) { return f.out, f.err }

// --- tests ---

func TestTokenServiceCreate(t *testing.T) {
	repo := &fakeRepo{}
	auth := NewTokenAuth("admin")
	svc := NewTokenService(repo, auth)

	resp, err := svc.Create(dto.CreateTokenRequest{Label: "  bot  "})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Fatal("empty token")
	}
	if resp.APIToken.Label != "bot" {
		t.Fatalf("label not trimmed: %q", resp.APIToken.Label)
	}
	if resp.APIToken.TokenHash != "" {
		t.Fatal("hash leaked in response")
	}
	if len(repo.tokens) != 1 {
		t.Fatalf("expected 1 stored token, got %d", len(repo.tokens))
	}
	if repo.tokens[0].TokenHash == "" {
		t.Fatal("hash not persisted")
	}
	if role, ok := auth.Authenticate(resp.Token); !ok || role != "issued" {
		t.Fatal("issued token does not authenticate")
	}
}

func TestSyncServiceRun(t *testing.T) {
	syncState := NewSyncState()
	svc := NewSyncService(syncState, &fakeSyncer{out: "done"})

	resp, err := svc.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "synced" || resp.Output != "done" {
		t.Fatalf("resp = %+v", resp)
	}
	if syncState.Needed() {
		t.Fatal("sync flag not cleared after successful sync")
	}
}

func TestSyncServiceRunError(t *testing.T) {
	svc := NewSyncService(NewSyncState(), &fakeSyncer{out: "partial", err: errors.New("boom")})

	resp, err := svc.Run(context.Background())
	var ae *AppError
	if !errors.As(err, &ae) || ae.Status != 502 {
		t.Fatalf("expected 502 AppError, got %v", err)
	}
	if resp.Status != "error" || resp.Output != "partial" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestSyncServiceRunScriptNotFound(t *testing.T) {
	svc := NewSyncService(NewSyncState(), &fakeSyncer{err: fmt.Errorf("%w: /x", ErrSyncScriptNotFound)})

	_, err := svc.Run(context.Background())
	var ae *AppError
	if !errors.As(err, &ae) || ae.Status != 501 {
		t.Fatalf("expected 501 AppError, got %v", err)
	}
}
