package main

import (
	"strings"

	"vlesspanel/dto"
	"vlesspanel/model"
)

// TokenService — выпуск/список/отзыв API-токенов (для ботов/агентов).
type TokenService struct {
	storage Repository
	auth    *TokenAuth
}

func NewTokenService(storage Repository, auth *TokenAuth) *TokenService {
	return &TokenService{storage: storage, auth: auth}
}

// List возвращает выпущенные токены (без raw-токена и без хэша).
func (s *TokenService) List() ([]model.APIToken, error) {
	tokens, err := s.storage.LoadTokens()
	if err != nil {
		return nil, errInternal(msgLoadTokensFailed)
	}
	for i := range tokens {
		tokens[i].TokenHash = ""
	}
	return tokens, nil
}

// Create выпускает новый токен. Raw-токен возвращается один раз.
func (s *TokenService) Create(req dto.CreateTokenRequest) (dto.CreateTokenResponse, error) {
	raw := newRawToken()
	tok := model.APIToken{
		ID:        "tok-" + randID(),
		Label:     strings.TrimSpace(req.Label),
		TokenHash: hashToken(raw),
		CreatedAt: nowStr(),
	}
	if err := s.storage.AddToken(tok); err != nil {
		return dto.CreateTokenResponse{}, errInternal(msgCreateTokenFailed)
	}
	s.auth.AddIssued(tok.TokenHash)

	tok.TokenHash = ""
	return dto.CreateTokenResponse{Token: raw, APIToken: tok}, nil
}

// Delete отзывает токен по ID.
func (s *TokenService) Delete(id string) (dto.StatusResponse, error) {
	tok, err := s.storage.DeleteToken(id)
	if err != nil {
		return dto.StatusResponse{}, err
	}
	s.auth.RemoveIssued(tok.TokenHash)
	return dto.StatusResponse{Status: "revoked", ID: tok.ID}, nil
}
