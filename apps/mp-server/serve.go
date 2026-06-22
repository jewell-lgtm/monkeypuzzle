package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"go.temporal.io/sdk/client"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/identity"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/workos"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	forgegithub "github.com/jewell-lgtm/monkeypuzzle/internal/server/forge/github"
	forgegitlab "github.com/jewell-lgtm/monkeypuzzle/internal/server/forge/gitlab"
	mcppkg "github.com/jewell-lgtm/monkeypuzzle/internal/server/mcp"
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

	svc := service.New(st, syncpkg.NewTemporalTrigger(tc, st))

	// The forge registry is provider-neutral and needs no OAuth secrets; both
	// factories are always available. The gitlab base URL becomes configurable
	// in B6; "" defaults to gitlab.com.
	forgeRegistry := forge.Registry{
		"github": forgegithub.NewFactory(),
		"gitlab": forgegitlab.NewFactory(""),
	}

	// Logins is keyed by provider. GitHub goes through WorkOS; GitLab (direct
	// OAuth) is registered in B6, gated on its client id/secret being set.
	logins := map[string]identity.Provider{
		"github": workos.NewAPIClient(cfg.WorkOSAPIKey, cfg.WorkOSClientID, cfg.PublicBaseURL+"/auth/callback"),
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
	webHandler.Routes(mux)
	mux.Handle("GET /.well-known/oauth-protected-resource", mcppkg.ProtectedResourceMetadata(resourceURL, cfg.AuthKitDomain))
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)

	addr := ":" + cfg.Port
	log.Printf("mp-server serving on %s (public %s)", addr, cfg.PublicBaseURL)
	return http.ListenAndServe(addr, mux)
}
