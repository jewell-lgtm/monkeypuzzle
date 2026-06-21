// Package web is the human-facing HTML/AlpineJS presentation adapter over
// internal/server/service. It renders server-side templates and hydrates the
// repo list from a fragment endpoint (skeleton-first for fast TTFB), and owns
// the WorkOS login flow + session middleware.
package web

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"net/http"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/identity"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/githubapi"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

//go:embed static/*
var staticFS embed.FS

// Deps bundles the web handler's dependencies.
type Deps struct {
	Service       *service.Service
	Store         store.Store
	Login         identity.Provider // pluggable login: authorization URL + code exchange
	GitHub        githubapi.Factory // derive the GitHub profile from the passed-through token
	Session       session.Codec
	Cipher        crypto.TokenCipher
	SecureCookies bool // set the Secure flag on cookies (true behind HTTPS)
}

// Handler serves the HTML UI (views are gomponents, in views.go).
type Handler struct {
	deps Deps
}

// NewHandler returns the web handler.
func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// Routes registers the web routes on mux.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("GET /login", h.login)
	mux.HandleFunc("GET /auth/callback", h.callback)
	mux.HandleFunc("POST /logout", h.logout)
	mux.HandleFunc("GET /{$}", h.requireAuth(h.dashboard))
	mux.HandleFunc("GET /partials/repos", h.requireAuth(h.partialRepos))
	mux.HandleFunc("GET /repos/{owner}/{name}", h.requireAuth(h.repoPage))
	mux.HandleFunc("POST /sync", h.requireAuth(h.startSync))
	mux.HandleFunc("GET /sync/status", h.requireAuth(h.syncStatus))
}

type ctxKey int

const userIDKey ctxKey = iota

// requireAuth validates the session cookie and injects the user id into ctx,
// redirecting to /login otherwise. The cookie is integrity-protected, so a
// valid signature is trusted without a store round-trip.
func (h *Handler) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		val, err := session.Read(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		uid, err := h.deps.Session.Decode(val)
		if err != nil {
			session.Clear(w)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userIDKey, uid)
		next(w, r.WithContext(ctx))
	}
}

func userID(ctx context.Context) int64 {
	uid, _ := ctx.Value(userIDKey).(int64)
	return uid
}

// randomState returns a URL-safe random string for OAuth CSRF state.
func randomState() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
