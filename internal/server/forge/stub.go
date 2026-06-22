package forge

import (
	"context"
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// StubFactory is an in-memory Factory for hermetic tests: it serves canned
// users, repos, and PRs keyed by token, the analog of adapters.MockExec. It is
// safe for the single-goroutine setup used in tests (configure, then run).
type StubFactory struct {
	users map[string]User
	repos map[string][]Repo
	prs   map[string]map[string][]stackgraph.PRRef // token -> "owner/name" -> PRs
}

// NewStubFactory returns an empty stub.
func NewStubFactory() *StubFactory {
	return &StubFactory{
		users: map[string]User{},
		repos: map[string][]Repo{},
		prs:   map[string]map[string][]stackgraph.PRRef{},
	}
}

// SetUser registers the account a token authenticates as.
func (f *StubFactory) SetUser(token string, u User) { f.users[token] = u }

// SetRepos registers the repos a token can access.
func (f *StubFactory) SetRepos(token string, repos ...Repo) { f.repos[token] = repos }

// SetPullRequests registers the PRs a token sees for owner/name.
func (f *StubFactory) SetPullRequests(token, owner, name string, prs ...stackgraph.PRRef) {
	if f.prs[token] == nil {
		f.prs[token] = map[string][]stackgraph.PRRef{}
	}
	f.prs[token][owner+"/"+name] = prs
}

// ForToken returns a client scoped to token.
func (f *StubFactory) ForToken(token string) Client {
	return &stubClient{f: f, token: token}
}

type stubClient struct {
	f     *StubFactory
	token string
}

func (c *stubClient) GetAuthenticatedUser(context.Context) (User, error) {
	u, ok := c.f.users[c.token]
	if !ok {
		return User{}, fmt.Errorf("forge stub: unknown token %q", c.token)
	}
	return u, nil
}

func (c *stubClient) ListAccessibleRepos(context.Context) ([]Repo, error) {
	return c.f.repos[c.token], nil
}

func (c *stubClient) ListPullRequests(_ context.Context, owner, name string) ([]stackgraph.PRRef, error) {
	return c.f.prs[c.token][owner+"/"+name], nil
}

// compile-time assertions.
var (
	_ Factory = (*StubFactory)(nil)
	_ Client  = (*stubClient)(nil)
)
