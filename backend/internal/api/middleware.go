package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/ranjitharani/webhook-delivery-platform/internal/models"
	"github.com/ranjitharani/webhook-delivery-platform/internal/store"
)

type ctxKey string

const tenantKey ctxKey = "tenant"

func TenantFromContext(ctx context.Context) *models.Tenant {
	t, _ := ctx.Value(tenantKey).(*models.Tenant)
	return t
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
				apiKey = strings.TrimSpace(auth[7:])
			}
		}
		if apiKey == "" {
			writeError(w, http.StatusUnauthorized, "missing api key")
			return
		}
		tenant, err := s.store.GetTenantByAPIKey(r.Context(), apiKey)
		if err != nil {
			if err == store.ErrNotFound {
				writeError(w, http.StatusUnauthorized, "invalid api key")
				return
			}
			writeError(w, http.StatusInternalServerError, "auth lookup failed")
			return
		}
		ctx := context.WithValue(r.Context(), tenantKey, tenant)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, o := range s.corsOrigins {
		allowed[o] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && corsAllowed(origin, allowed) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func corsAllowed(origin string, allowed map[string]struct{}) bool {
	if len(allowed) == 0 {
		return true
	}
	if _, ok := allowed[origin]; ok {
		return true
	}
	// Allow local Vite and this project's GitHub Pages host.
	if strings.HasPrefix(origin, "http://localhost:") || strings.HasPrefix(origin, "http://127.0.0.1:") {
		return true
	}
	if strings.HasSuffix(origin, ".github.io") {
		return true
	}
	return false
}
