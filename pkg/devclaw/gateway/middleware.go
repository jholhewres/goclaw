// Package gateway provides an HTTP API gateway for DevClaw.
package gateway

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
)

// compareTokens performs timing-safe comparison by hashing both inputs with
// SHA-256 before calling ConstantTimeCompare to prevent length-based leakage.
func compareTokens(a, b string) bool {
	ha := sha256.Sum256([]byte(a))
	hb := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// authMiddleware requires Authorization: Bearer <token> when authToken is non-empty.
// Skips auth for /health. Applied to /api/* and /v1/* when token is set.
func (g *Gateway) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if g.config.AuthToken == "" {
			next.ServeHTTP(w, r)
			return
		}
		path := r.URL.Path
		if path == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		auth := r.Header.Get("Authorization")
		if auth == "" {
			g.writeError(w, "missing Authorization header", 401)
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			g.writeError(w, "invalid Authorization format", 401)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		if !compareTokens(token, g.config.AuthToken) {
			g.writeError(w, "invalid token", 401)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// corsMiddleware adds CORS headers when origins are configured.
func (g *Gateway) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(g.config.CORSOrigins) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		allowed := false
		for _, o := range g.config.CORSOrigins {
			if o == "*" || o == origin {
				allowed = true
				break
			}
		}
		if allowed {
			if origin == "" {
				origin = g.config.CORSOrigins[0]
			}
			if origin == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
