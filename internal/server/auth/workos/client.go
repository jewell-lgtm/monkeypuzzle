// Package workos integrates WorkOS as the identity provider for mp server.
// WorkOS is the OAuth 2.1 Authorization Server for both consumer classes: human
// web login (AuthKit, "Sign in with GitHub") and AI agents on the MCP surface.
// A verified WorkOS identity is sufficient for registry login. An optional
// GitHub access token supports PR monitoring when token passthrough is enabled.
package workos

import (
	"context"
	"fmt"
	"strings"

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
	return registryIdentity(resp), nil
}

func registryIdentity(resp usermanagement.AuthenticateResponse) identity.Identity {
	result := identity.Identity{
		ProviderUserID: resp.User.ID,
		Provider:       "github",
		DisplayName:    strings.TrimSpace(resp.User.FirstName + " " + resp.User.LastName),
		AvatarURL:      resp.User.ProfilePictureURL,
	}
	if result.DisplayName == "" {
		result.DisplayName = resp.User.ID
	}
	if resp.OAuthTokens != nil {
		result.Token = resp.OAuthTokens.AccessToken
	}
	return result
}

var _ identity.Provider = (*APIClient)(nil)
