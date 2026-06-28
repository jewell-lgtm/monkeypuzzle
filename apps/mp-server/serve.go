package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	gitlabauth "github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/gitlab"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/identity"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/workos"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	forgegithub "github.com/jewell-lgtm/monkeypuzzle/internal/server/forge/github"
	forgegitlab "github.com/jewell-lgtm/monkeypuzzle/internal/server/forge/gitlab"
	mcppkg "github.com/jewell-lgtm/monkeypuzzle/internal/server/mcp"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/mprunner"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/service"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	syncpkg "github.com/jewell-lgtm/monkeypuzzle/internal/server/sync"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/web"
)

// runServe wires the read service, the HTML UI (humans), and the MCP endpoint
// (agents) — both over the same service — and serves HTTP.
func runServe() error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	ctx := context.Background()

	st, err := store.NewPgxStore(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	cipher, err := crypto.NewAESGCMCipher(cfg.TokenEncryptionKey)
	if err != nil {
		return err
	}

	tc, err := client.Dial(client.Options{HostPort: cfg.TemporalHostPort})
	if err != nil {
		return fmt.Errorf("temporal dial: %w", err)
	}
	defer tc.Close()

	// Optional `mp stack graph` path (flag-gated by USE_MP_CLI); falls back to the
	// in-process Go path on any failure. Reuses the same token cipher as sync.
	runner := mprunner.NewExecRunner(cfg.MpBin)
	svc := service.New(st, syncpkg.NewTemporalTrigger(tc, st), runner, cipher, cfg.UseMpCLI)

	// The forge registry is provider-neutral and needs no OAuth secrets; both
	// factories are always available so a synced user of either forge can be read.
	forgeRegistry := forge.Registry{
		"github": forgegithub.NewFactory(),
		"gitlab": forgegitlab.NewFactory(cfg.GitLabBaseURL),
	}

	// Logins is keyed by provider. GitHub goes through WorkOS; GitLab (direct
	// OAuth) is opt-in — registered only when its client id/secret are set.
	logins := map[string]identity.Provider{
		"github": workos.NewAPIClient(cfg.WorkOSAPIKey, cfg.WorkOSClientID, cfg.PublicBaseURL+"/auth/callback"),
	}
	if cfg.GitLabEnabled() {
		logins["gitlab"] = gitlabauth.NewOAuthClient(
			cfg.GitLabBaseURL, cfg.GitLabOAuthClientID, cfg.GitLabOAuthClientSecret, cfg.PublicBaseURL+"/auth/callback")
	}

	webHandler := web.NewHandler(web.Deps{
		Service:       svc,
		Store:         st,
		Logins:        logins,
		Forge:         forgeRegistry,
		Session:       session.NewSecureCookieCodec(cfg.SessionSecret),
		Cipher:        cipher,
		SecureCookies: cfg.SecureCookies,
	})

	// MCP resource server: validate WorkOS tokens against AuthKit's JWKS, bind
	// to the resource (this server's public URL).
	resourceURL := cfg.PublicBaseURL
	verifier, err := workos.NewTokenVerifier(cfg.WorkOSJWKSURL, cfg.AuthKitDomain, resourceURL, st)
	if err != nil {
		return fmt.Errorf("mcp token verifier: %w", err)
	}
	mcpHandler := mcppkg.NewHTTPHandler(mcppkg.NewServer(svc), verifier, resourceURL+"/.well-known/oauth-protected-resource")

	mux := http.NewServeMux()
	// Health endpoints are registered first and are NOT behind auth: probes hit
	// them unauthenticated. Liveness never touches dependencies; readiness gates
	// on the DB and best-effort checks Temporal.
	mux.HandleFunc("GET /healthz", healthzHandler)
	mux.HandleFunc("GET /readyz", newReadyzHandler(st, func(ctx context.Context) error {
		_, err := tc.CheckHealth(ctx, &client.CheckHealthRequest{})
		return err
	}))
	webHandler.Routes(mux)
	mux.Handle("GET /.well-known/oauth-protected-resource", mcppkg.ProtectedResourceMetadata(resourceURL, cfg.AuthKitDomain))
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)

	addr := ":" + cfg.Port
	srv := &http.Server{Addr: addr, Handler: mux}

	// Serve in the background so the main goroutine can wait for a termination
	// signal and drive a graceful shutdown.
	errCh := make(chan error, 1)
	go func() {
		log.Printf("mp-server serving on %s (public %s)", addr, cfg.PublicBaseURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		log.Printf("mp-server received %s, draining connections", sig)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Print("mp-server stopped")
	return nil
}

// healthzHandler is the liveness probe: it returns 200 unconditionally whenever
// the process is serving. It performs NO dependency checks on purpose — a DB or
// Temporal blip must not get the pod killed and restarted.
func healthzHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, "ok")
}

// dbPinger is the readiness check's hard dependency: a reachable store.
type dbPinger interface {
	Ping(ctx context.Context) error
}

// newReadyzHandler builds the readiness probe. The DB ping is the hard gate
// (503 with a generic body on failure; the raw error is logged, not returned).
// temporalCheck, when non-nil, is a
// best-effort probe whose failure is logged but does NOT fail readiness — a
// Temporal outage should degrade sync, not take the whole server out of rotation.
func newReadyzHandler(db dbPinger, temporalCheck func(context.Context) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if err := db.Ping(ctx); err != nil {
			// Log the raw driver error server-side (it can carry DB
			// user/database/host:port), but return a generic body so we don't
			// disclose it to unauthenticated probe callers.
			log.Printf("readyz: db ping failed: %v", err)
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "database unreachable")
			return
		}
		if temporalCheck != nil {
			if err := temporalCheck(ctx); err != nil {
				log.Printf("readyz: temporal health check failed (best-effort): %v", err)
			}
		}
		_, _ = io.WriteString(w, "ok")
	}
}
