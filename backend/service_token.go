package main

import "strings"

// TokenService — выпуск/список/отзыв API-токенов (для ботов/агентов).
type TokenService struct {
	storage *Storage
	auth    *TokenAuth
}

func NewTokenService(storage *Storage, auth *TokenAuth) *TokenService {
	return &TokenService{storage: storage, auth: auth}
}

// List возвращает выпущенные токены (без raw-токена и без хэша).
func (s *TokenService) List() ([]APIToken, error) {
	tokens, err := s.storage.LoadTokens()
	if err != nil {
		return nil, errInternal("Failed to load tokens")
	}
	for i := range tokens {
		tokens[i].TokenHash = ""
	}
	return tokens, nil
}

// Create выпускает новый токен. Raw-токен возвращается один раз.
func (s *TokenService) Create(req CreateTokenRequest) (CreateTokenResponse, error) {
	raw := newRawToken()
	tok := APIToken{
		ID:        "tok-" + randID(),
		Label:     strings.TrimSpace(req.Label),
		TokenHash: hashToken(raw),
		CreatedAt: nowStr(),
	}
	if err := s.storage.AddToken(tok); err != nil {
		return CreateTokenResponse{}, errInternal("Failed to create token")
	}
	s.auth.AddIssued(tok.TokenHash)

	tok.TokenHash = ""
	return CreateTokenResponse{Token: raw, APIToken: tok}, nil
}

// Delete отзывает токен по ID.
func (s *TokenService) Delete(id string) (StatusResponse, error) {
	tok, err := s.storage.DeleteToken(id)
	if err != nil {
		return StatusResponse{}, err
	}
	s.auth.RemoveIssued(tok.TokenHash)
	return StatusResponse{Status: "revoked", ID: tok.ID}, nil
}
