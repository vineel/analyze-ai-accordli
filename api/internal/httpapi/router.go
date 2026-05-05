// Package httpapi is the HTTP edge: router, middleware, handlers.
//
// Handlers depend on /core and /infra interfaces — never on concrete
// impls. The auth middleware reads from infra/auth.Provider.
package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"accordli.com/analyze-ai/api/internal/infra/auth"
	"accordli.com/analyze-ai/api/internal/infra/observability"
	"accordli.com/analyze-ai/api/internal/infra/repo"
)

type Deps struct {
	Auth    auth.Provider
	Log     observability.Logger
	Repos   *repo.Repos
	Matters *MattersDeps
	Version string // git SHA, surfaced by /health
}

type identityKey struct{}

// IdentityFrom returns the Identity attached by the auth middleware.
// Returns nil if the middleware is not in the chain (e.g., on /health).
func IdentityFrom(ctx context.Context) *auth.Identity {
	v, _ := ctx.Value(identityKey{}).(*auth.Identity)
	return v
}

func (d *Deps) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := d.Auth.Resolve(r.Context(), r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), identityKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func NewRouter(d *Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)

	// Public.
	r.Get("/health", d.healthHandler)
	r.Get("/api/health", d.healthHandler)

	// Authenticated routes.
	r.Group(func(r chi.Router) {
		r.Use(d.authMiddleware)
		if d.Matters != nil {
			d.mountMatters(r)
		}
	})

	return r
}

func (d *Deps) healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": d.Version,
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
