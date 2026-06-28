// Package forge is the forge-neutral, read-only API access layer for mp server.
// Unlike the CLI (which shells out to `gh`/`glab`), the server talks to each
// forge's REST API over HTTP using each user's OAuth token. Following the repo's
// ports/adapters convention, Client is an interface with per-forge
// implementations (forge/github, forge/gitlab) and a provider-agnostic stub for
// hermetic tests.
package forge

import (
	"context"
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// User is the authenticated account behind a token.
type User struct {
	ID        int64
	Login     string
	AvatarURL string
}

// Repo is a forge repository, shaped for store.Repo. ForgeRepoID is the
// forge-native numeric id (GitHub repo id / GitLab project id).
type Repo struct {
	ForgeRepoID   int64
	Owner         string
	Name          string
	DefaultBranch string
	Private       bool
	HTMLURL       string
}

// Client is the read surface the worker depends on. It is token-scoped: a client
// is built per user token (via Factory), so methods carry no token.
type Client interface {
	// GetAuthenticatedUser returns the account the token belongs to.
	GetAuthenticatedUser(ctx context.Context) (User, error)
	// ListAccessibleRepos returns every repo the token can see (owner +
	// collaborator + organization/group member), following pagination.
	ListAccessibleRepos(ctx context.Context) ([]Repo, error)
	// ListPullRequests returns all PRs/MRs (state=all) of a repo as
	// stackgraph.PRRef.
	ListPullRequests(ctx context.Context, owner, name string) ([]stackgraph.PRRef, error)
}

// Factory builds a token-scoped Client.
type Factory interface {
	ForToken(token string) Client
}

// Registry maps a provider name (e.g. "github", "gitlab") to its Factory. It is
// the forge-neutral entry point: callers resolve a token-scoped client by
// provider, so the store/sync/web layers stay provider-agnostic.
type Registry map[string]Factory

// ForToken returns a token-scoped Client for provider, or an error if no factory
// is registered for it.
func (r Registry) ForToken(provider, token string) (Client, error) {
	f, ok := r[provider]
	if !ok {
		return nil, fmt.Errorf("forge: no factory registered for provider %q", provider)
	}
	return f.ForToken(token), nil
}
