package mcp

import (
	"encoding/json"
	"net/http"
)

// ProtectedResourceMetadata serves the RFC 9728 OAuth Protected Resource
// Metadata document at /.well-known/oauth-protected-resource. It tells MCP
// clients which Authorization Server (WorkOS AuthKit) issues tokens for this
// resource, so they can complete the OAuth 2.1 handshake. resourceURL is this
// server's public URL; authServerURL is the WorkOS/AuthKit domain.
func ProtectedResourceMetadata(resourceURL, authServerURL string) http.HandlerFunc {
	body, _ := json.Marshal(map[string]any{
		"resource":                 resourceURL,
		"authorization_servers":    []string{authServerURL},
		"bearer_methods_supported": []string{"header"},
	})
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}
