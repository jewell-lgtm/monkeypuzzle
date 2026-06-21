// Package identity is the provider-neutral seam for mp server's authentication.
// WorkOS is the current implementation, but nothing outside the provider package
// depends on it: the web login flow depends on Provider, and the MCP resource
// server depends on the SDK's generic auth.TokenVerifier. To swap providers (or
// offer several), implement Provider and supply a matching token verifier — the
// store, service, sync, and MCP layers are unchanged.
package identity

import "context"

// Identity is the result of a successful login: the identity provider's stable
// subject (used as the local user's external id, shared with the agent's MCP
// token) plus the passed-through GitHub access token used to call the API.
type Identity struct {
	ProviderUserID string
	GitHubToken    string
}

// Provider is the human login surface: produce an authorization URL, then
// exchange the returned code for an Identity.
type Provider interface {
	AuthorizationURL(state string) string
	Authenticate(ctx context.Context, code string) (Identity, error)
}
