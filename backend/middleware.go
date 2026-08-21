package main

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"
)

// loggingMiddleware logs each HTTP request
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

// ctxKey — ключ контекста для роли аутентифицированного запроса.
type ctxKey int

const authRoleKey ctxKey = 0

// authRole возвращает роль запроса ("admin"/"issued"), сохранённую middleware.
func authRole(r *http.Request) string {
	v, _ := r.Context().Value(authRoleKey).(string)
	return v
}

// bearerToken извлекает токен из заголовка Authorization: Bearer <token>.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// authMiddleware требует валидный bearer-токен (master или issued) и сохраняет
// роль в контекст. При выключенной аутентификации (нет admin-токена) пропускает
// всё с ролью "admin".
func authMiddleware(auth *TokenAuth) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role, ok := auth.Authenticate(bearerToken(r))
			if !ok {
				respondError(w, http.StatusUnauthorized, msgUnauthorized)
				return
			}
			r = r.WithContext(context.WithValue(r.Context(), authRoleKey, role))
			next.ServeHTTP(w, r)
		})
	}
}

// requireAdmin пропускает только запросы с ролью "admin".
func requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authRole(r) != "admin" {
			respondError(w, http.StatusForbidden, msgAdminRequired)
			return
		}
		next.ServeHTTP(w, r)
	})
}
