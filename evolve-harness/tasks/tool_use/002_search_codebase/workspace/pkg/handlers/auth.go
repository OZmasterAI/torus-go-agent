package handlers

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

const bearerPrefix = "Bearer "

// AuthMiddleware validates the Authorization header against a static token.
// TODO: Replace static token validation with JWT verification using RS256.
func AuthMiddleware(validToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, bearerPrefix) {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(header, bearerPrefix)
		if subtle.ConstantTimeCompare([]byte(token), []byte(validToken)) != 1 {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
