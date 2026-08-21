package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sync"
)

// TokenAuth проверяет bearer-токены: master-токен (из env VLESSPANEL_ADMIN_TOKEN)
// и выпущенные токены (persistятся в tokens.json, кэшируются в памяти).
//
// Роли:
//   - "admin" — master-токен, полный доступ включая управление токенами.
//   - "issued" — выпущенный токен (для ботов/агентов), доступ к panel/API без
//     управления токенами.
//
// Если admin-токен не задан (пустой env), auth отключён — разрешено всё.
type TokenAuth struct {
	mu      sync.RWMutex
	enabled bool
	admin   string              // sha256 hex master-токена
	issued  map[string]struct{} // sha256 hex выпущенных токенов
}

// NewTokenAuth создаёт TokenAuth. Пустой adminToken отключает проверку.
func NewTokenAuth(adminToken string) *TokenAuth {
	a := &TokenAuth{issued: make(map[string]struct{})}
	if adminToken != "" {
		a.enabled = true
		a.admin = hashToken(adminToken)
	}
	return a
}

// Enabled сообщает, включена ли аутентификация.
func (a *TokenAuth) Enabled() bool { return a.enabled }

// Authenticate возвращает роль и ok для валидного токена. При отключённой
// проверке всё разрешено (роль "admin").
func (a *TokenAuth) Authenticate(token string) (role string, ok bool) {
	if !a.enabled {
		return "admin", true
	}
	h := hashToken(token)
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.admin != "" && subtle.ConstantTimeCompare([]byte(h), []byte(a.admin)) == 1 {
		return "admin", true
	}
	if _, ok := a.issued[h]; ok {
		return "issued", true
	}
	return "", false
}

// SetIssued заменяет in-memory набор выпущенных токенов (при старте).
func (a *TokenAuth) SetIssued(hashes map[string]struct{}) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.issued = hashes
}

// AddIssued регистрирует только что выпущенный токен.
func (a *TokenAuth) AddIssued(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.issued[hash] = struct{}{}
}

// RemoveIssued забывает отозванный токен.
func (a *TokenAuth) RemoveIssued(hash string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.issued, hash)
}

// hashToken возвращает sha256 hex-хэш токена.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
