// Package githubapi is the read-only GitHub access layer for mp server. Unlike
// the CLI (which shells out to `gh`), the server talks to the GitHub REST API
// over HTTP using each user's OAuth token. Following the repo's ports/adapters
// convention, GitHubClient is an interface with a go-github implementation and
// a stub for hermetic tests.
package githubapi

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// GitHubUser is the authenticated account behind a token.
type GitHubUser struct {
	ID        int64
	Login     string
	AvatarURL string
}

// Repo is a GitHub repository, shaped for store.Repo.
type Repo struct {
	GitHubRepoID  int64
	Owner         string
	Name          string
	DefaultBranch string
	Private       bool
	HTMLURL       string
}

// GitHubClient is the read surface the worker depends on. It is token-scoped: a
// client is built per user token (via Factory), so methods carry no token.
type GitHubClient interface {
	// GetAuthenticatedUser returns the account the token belongs to.
	GetAuthenticatedUser(ctx context.Context) (GitHubUser, error)
	// ListAccessibleRepos returns every repo the token can see (owner +
	// collaborator + organization member), following pagination.
	ListAccessibleRepos(ctx context.Context) ([]Repo, error)
	// ListPullRequests returns all PRs (state=all) of a repo as stackgraph.PRRef.
	ListPullRequests(ctx context.Context, owner, name string) ([]stackgraph.PRRef, error)
}

// Factory builds a token-scoped GitHubClient.
type Factory interface {
	ForToken(token string) GitHubClient
}
