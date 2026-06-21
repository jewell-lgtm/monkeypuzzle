// Package githuboauth implements the GitHub OAuth App login flow for human web
// sessions: redirect to GitHub, exchange the code for a user access token, and
// fetch the user's profile. The token is then encrypted and stored; the worker
// uses it to call GitHub.
package githuboauth

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"
	oauth2github "golang.org/x/oauth2/github"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/githubapi"
)

// Profile is the authenticated GitHub account.
type Profile struct {
	GitHubUserID int64
	Login        string
	AvatarURL    string
}

// Client is the OAuth login surface; the real impl talks to GitHub, the stub is
// for tests.
type Client interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (token string, err error)
	FetchProfile(ctx context.Context, token string) (Profile, error)
}

// OAuth2Client is the real GitHub OAuth App client.
type OAuth2Client struct {
	cfg     *oauth2.Config
	factory githubapi.Factory
}

// NewOAuth2Client builds a client for the given OAuth App credentials. It reuses
// a githubapi.Factory to fetch the profile with the freshly minted token.
func NewOAuth2Client(clientID, clientSecret, redirectURL string, factory githubapi.Factory) *OAuth2Client {
	return &OAuth2Client{
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Scopes:       []string{"repo", "read:user"},
			Endpoint:     oauth2github.Endpoint,
		},
		factory: factory,
	}
}

func (c *OAuth2Client) AuthCodeURL(state string) string {
	return c.cfg.AuthCodeURL(state)
}

func (c *OAuth2Client) Exchange(ctx context.Context, code string) (string, error) {
	tok, err := c.cfg.Exchange(ctx, code)
	if err != nil {
		return "", fmt.Errorf("githuboauth: exchange: %w", err)
	}
	return tok.AccessToken, nil
}

func (c *OAuth2Client) FetchProfile(ctx context.Context, token string) (Profile, error) {
	u, err := c.factory.ForToken(token).GetAuthenticatedUser(ctx)
	if err != nil {
		return Profile{}, fmt.Errorf("githuboauth: fetch profile: %w", err)
	}
	return Profile{GitHubUserID: u.ID, Login: u.Login, AvatarURL: u.AvatarURL}, nil
}

var _ Client = (*OAuth2Client)(nil)
