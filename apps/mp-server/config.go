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
	AuthKitDomain      string // e.g. https://your-app.authkit.app (JWKS, issuer, AS metadata)
	SessionSecret      []byte
	TokenEncryptionKey []byte // exactly 32 bytes (AES-256-GCM)
	SecureCookies      bool
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

	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
