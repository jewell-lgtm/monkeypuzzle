// Package gitlab implements identity.Provider via direct GitLab OAuth2 (WorkOS
// has no built-in GitLab connector). Humans authorize against {base}/oauth and
// the server stores the resulting access token (encrypted) so the worker can
// call the GitLab API. GitHub continues to use the WorkOS leg unchanged.
package gitlab

import (
	"context"
	"fmt"
	"strconv"

	gitlab "gitlab.com/gitlab-org/api/client-go"
	"golang.org/x/oauth2"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/identity"
)

// defaultBaseURL is the SaaS GitLab instance.
const defaultBaseURL = "https://gitlab.com"

// OAuthClient is a direct-GitLab-OAuth2 identity.Provider.
type OAuthClient struct {
	baseURL string
	cfg     *oauth2.Config
}

// NewOAuthClient configures GitLab OAuth2. An empty baseURL defaults to
// https://gitlab.com; the authorize/token endpoints are derived from it. The
// read_api + read_user scopes let the server read the user's projects and MRs.
func NewOAuthClient(baseURL, clientID, clientSecret, redirectURI string) *OAuthClient {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &OAuthClient{
		baseURL: baseURL,
		cfg: &oauth2.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURI,
			Scopes:       []string{"read_api", "read_user"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  baseURL + "/oauth/authorize",
				TokenURL: baseURL + "/oauth/token",
			},
		},
	}
}

func (c *OAuthClient) AuthorizationURL(state string) string {
	return c.cfg.AuthCodeURL(state)
}

func (c *OAuthClient) Authenticate(ctx context.Context, code string) (identity.Identity, error) {
	tok, err := c.cfg.Exchange(ctx, code)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("gitlab oauth: exchange: %w", err)
	}
	// Resolve the stable subject from the token so the local external id is
	// unique and prefixed to avoid colliding with WorkOS subjects.
	gl, err := gitlab.NewClient(tok.AccessToken, gitlab.WithBaseURL(c.baseURL))
	if err != nil {
		return identity.Identity{}, fmt.Errorf("gitlab oauth: client: %w", err)
	}
	u, _, err := gl.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return identity.Identity{}, fmt.Errorf("gitlab oauth: current user: %w", err)
	}
	return identity.Identity{
		ProviderUserID: "gitlab:" + strconv.FormatInt(u.ID, 10),
		Provider:       "gitlab",
		Token:          tok.AccessToken,
	}, nil
}

var _ identity.Provider = (*OAuthClient)(nil)
