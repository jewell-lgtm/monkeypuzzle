package main

import (
	"encoding/base64"
	"fmt"
	"os"
	"sort"
)

// Config is the mp-server runtime configuration, loaded from the environment.
type Config struct {
	Port               string
	PublicBaseURL      string // e.g. http://localhost:8080 (OAuth redirect + MCP resource)
	DatabaseURL        string
	TemporalHostPort   string
	WorkOSAPIKey       string
	WorkOSClientID     string
	AuthKitDomain      string // e.g. https://your-app.authkit.app (token issuer + AS metadata)
	WorkOSJWKSURL      string // where the MCP verifier fetches signing keys
	SessionSecret      []byte
	TokenEncryptionKey []byte // exactly 32 bytes (AES-256-GCM)
	SecureCookies      bool

	// GitLab is opt-in: the gitlab login leg is registered only when both
	// GitLabOAuthClientID and GitLabOAuthClientSecret are set, so existing
	// GitHub-only deploys are unaffected.
	GitLabBaseURL           string // e.g. https://gitlab.com or a self-managed instance
	GitLabOAuthClientID     string
	GitLabOAuthClientSecret string
}

// GitLabEnabled reports whether the direct-GitLab-OAuth login leg should be
// registered (both client id and secret present).
func (c Config) GitLabEnabled() bool {
	return c.GitLabOAuthClientID != "" && c.GitLabOAuthClientSecret != ""
}

// LoadConfig reads the environment and fails loudly on any missing/invalid value.
func LoadConfig() (Config, error) {
	cfg := Config{
		Port:             envOr("PORT", "8080"),
		PublicBaseURL:    os.Getenv("PUBLIC_BASE_URL"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		TemporalHostPort: envOr("TEMPORAL_HOSTPORT", "localhost:7233"),
		WorkOSAPIKey:     os.Getenv("WORKOS_API_KEY"),
		WorkOSClientID:   os.Getenv("WORKOS_CLIENT_ID"),
		AuthKitDomain:    os.Getenv("AUTHKIT_DOMAIN"),
		SecureCookies:    os.Getenv("SECURE_COOKIES") == "true",

		GitLabBaseURL:           envOr("GITLAB_BASE_URL", "https://gitlab.com"),
		GitLabOAuthClientID:     os.Getenv("GITLAB_OAUTH_CLIENT_ID"),
		GitLabOAuthClientSecret: os.Getenv("GITLAB_OAUTH_CLIENT_SECRET"),
	}

	required := map[string]string{
		"PUBLIC_BASE_URL":  cfg.PublicBaseURL,
		"DATABASE_URL":     cfg.DatabaseURL,
		"WORKOS_API_KEY":   cfg.WorkOSAPIKey,
		"WORKOS_CLIENT_ID": cfg.WorkOSClientID,
		"AUTHKIT_DOMAIN":   cfg.AuthKitDomain,
	}
	var missing []string
	for k, v := range required {
		if v == "" {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return Config{}, fmt.Errorf("missing required env: %v", missing)
	}

	sess, err := base64.StdEncoding.DecodeString(os.Getenv("SESSION_SECRET"))
	if err != nil || len(sess) < 16 {
		return Config{}, fmt.Errorf("SESSION_SECRET must be base64 of >= 16 random bytes")
	}
	cfg.SessionSecret = sess

	key, err := base64.StdEncoding.DecodeString(os.Getenv("TOKEN_ENCRYPTION_KEY"))
	if err != nil || len(key) != 32 {
		return Config{}, fmt.Errorf("TOKEN_ENCRYPTION_KEY must be base64 of exactly 32 bytes")
	}
	cfg.TokenEncryptionKey = key

	// Default the MCP verifier's JWKS to WorkOS's API endpoint, which is always
	// reachable (independent of whether a custom AuthKit domain is deployed).
	cfg.WorkOSJWKSURL = envOr("WORKOS_JWKS_URL", "https://api.workos.com/sso/jwks/"+cfg.WorkOSClientID)

	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
