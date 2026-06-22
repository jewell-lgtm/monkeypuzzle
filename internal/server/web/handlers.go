package web

import (
	"encoding/json"
	"errors"
	"net/http"

	g "maragu.dev/gomponents"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
)

const (
	oauthStateCookie    = "mp_oauth_state"
	oauthProviderCookie = "mp_oauth_provider"
	defaultProvider     = "github"
)

// login renders the provider chooser when called without ?provider=, otherwise
// it starts that provider's OAuth flow: set CSRF state + provider cookies,
// redirect to the provider's authorization URL. An unknown provider is a 400.
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		h.render(w, loginPage())
		return
	}
	login, ok := h.deps.Logins[provider]
	if !ok {
		http.Error(w, "unknown login provider", http.StatusBadRequest)
		return
	}
	state := randomState()
	h.setCookie(w, oauthStateCookie, state)
	h.setCookie(w, oauthProviderCookie, provider)
	http.Redirect(w, r, login.AuthorizationURL(state), http.StatusFound)
}

// setCookie writes a short-lived, HttpOnly OAuth cookie.
func (h *Handler) setCookie(w http.ResponseWriter, name, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.deps.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
}

// callback completes login: verify state, recover the provider from its cookie,
// exchange the code (WorkOS for GitHub, direct OAuth for GitLab) for an identity
// + forge token, derive the forge profile, store the user (encrypted token), set
// the session, and kick off a sync.
func (h *Handler) callback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	stateCookie, err := r.Cookie(oauthStateCookie)
	if err != nil || r.URL.Query().Get("state") == "" || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	provider := defaultProvider
	if pc, err := r.Cookie(oauthProviderCookie); err == nil && pc.Value != "" {
		provider = pc.Value
	}
	login, ok := h.deps.Logins[provider]
	if !ok {
		http.Error(w, "unknown login provider", http.StatusBadRequest)
		return
	}
	res, err := login.Authenticate(ctx, r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "authentication failed", http.StatusBadGateway)
		return
	}
	client, err := h.deps.Forge.ForToken(res.Provider, res.Token)
	if err != nil {
		http.Error(w, "unknown forge provider", http.StatusInternalServerError)
		return
	}
	profile, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		http.Error(w, "failed to fetch forge profile", http.StatusBadGateway)
		return
	}
	enc, err := h.deps.Cipher.Encrypt([]byte(res.Token))
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	uid, err := h.deps.Store.UpsertUser(ctx, store.User{
		ExternalUserID: res.ProviderUserID,
		Provider:       res.Provider,
		ForgeUserID:    profile.ID,
		ForgeLogin:     profile.Login,
		AvatarURL:      profile.AvatarURL,
		AccessTokenEnc: enc,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	value, err := h.deps.Session.Encode(uid)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	session.Set(w, value, h.deps.SecureCookies)
	// First-login sync so data populates while the user lands on the dashboard.
	_, _ = h.deps.Service.StartSync(ctx, uid)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// logout clears the session.
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	session.Clear(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// dashboard renders the shell immediately (skeleton); Alpine hydrates the repo
// list from /partials/repos for fast TTFB.
func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	h.render(w, dashboardPage())
}

// partialRepos returns just the repo-list HTML fragment from the cache.
func (h *Handler) partialRepos(w http.ResponseWriter, r *http.Request) {
	uid := userID(r.Context())
	repos, err := h.deps.Service.ListRepos(r.Context(), uid)
	if err != nil {
		http.Error(w, "failed to load repos", http.StatusInternalServerError)
		return
	}
	syncing := false
	if st, err := h.deps.Service.SyncStatus(r.Context(), uid); err == nil {
		syncing = st.Status == store.SyncRunning
	}
	h.render(w, reposFragment(repos, syncing))
}

// repoPage renders a repo's PR stacks as nested trees.
func (h *Handler) repoPage(w http.ResponseWriter, r *http.Request) {
	owner, name := r.PathValue("owner"), r.PathValue("name")
	rs, err := h.deps.Service.RepoStacks(r.Context(), userID(r.Context()), owner, name)
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "failed to load stacks", http.StatusInternalServerError)
		return
	}
	h.render(w, repoPageView(rs.Repo, rs.Stacks))
}

// startSync triggers a refresh; the UI polls /sync/status.
func (h *Handler) startSync(w http.ResponseWriter, r *http.Request) {
	if _, err := h.deps.Service.StartSync(r.Context(), userID(r.Context())); err != nil {
		http.Error(w, "failed to start sync", http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// syncStatus reports the current sync status as JSON for the Alpine poll.
func (h *Handler) syncStatus(w http.ResponseWriter, r *http.Request) {
	st, err := h.deps.Service.SyncStatus(r.Context(), userID(r.Context()))
	if err != nil {
		http.Error(w, "failed to read sync status", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": st.Status})
}

// render writes a gomponents node as the HTML response.
func (h *Handler) render(w http.ResponseWriter, node g.Node) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := node.Render(w); err != nil {
		http.Error(w, "render error", http.StatusInternalServerError)
	}
}
