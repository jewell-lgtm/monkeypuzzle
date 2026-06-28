// Package workos integrates WorkOS as the identity provider for mp server.
// WorkOS is the OAuth 2.1 Authorization Server for both consumer classes: human
// web login (AuthKit, "Sign in with GitHub") and AI agents on the MCP surface.
// For humans it also passes through the user's GitHub access token (enable
// "Return GitHub OAuth tokens" in the WorkOS dashboard), which the server stores
// encrypted so the worker can call the GitHub API.
package workos

import (
	"context"
	"errors"
	"fmt"

	"github.com/workos/workos-go/v4/pkg/usermanagement"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/identity"
)

// providerGitHub routes WorkOS authorization through GitHub OAuth.
const providerGitHub = "GitHubOAuth"

// APIClient is the real WorkOS-backed identity.Provider.
type APIClient struct {
	clientID    string
	redirectURI string
}

// NewAPIClient configures the WorkOS SDK and returns a provider.
func NewAPIClient(apiKey, clientID, redirectURI string) *APIClient {
	usermanagement.SetAPIKey(apiKey)
	return &APIClient{clientID: clientID, redirectURI: redirectURI}
}

func (c *APIClient) AuthorizationURL(state string) string {
	u, err := usermanagement.GetAuthorizationURL(usermanagement.GetAuthorizationURLOpts{
		ClientID:    c.clientID,
		RedirectURI: c.redirectURI,
		Provider:    providerGitHub,
		State:       state,
	})
	if err != nil {
		return ""
	}
	return u.String()
}

func (c *APIClient) Authenticate(ctx context.Context, code string) (identity.Identity, error) {
	resp, err := usermanagement.AuthenticateWithCode(ctx, usermanagement.AuthenticateWithCodeOpts{
		ClientID: c.clientID,
		Code:     code,
	})
	if err != nil {
		return identity.Identity{}, fmt.Errorf("workos: authenticate: %w", err)
	}
	if resp.OAuthTokens == nil || resp.OAuthTokens.AccessToken == "" {
		return identity.Identity{}, errors.New("workos: no GitHub OAuth token in response; enable 'Return GitHub OAuth tokens' in the WorkOS dashboard")
	}
	return identity.Identity{
		ProviderUserID: resp.User.ID,
		Provider:       "github",
		Token:          resp.OAuthTokens.AccessToken,
	}, nil
}

var _ identity.Provider = (*APIClient)(nil)
