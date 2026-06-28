// Package github is the go-github-backed implementation of forge.Client: the
// read-only GitHub REST access layer for mp server. It maps go-github types into
// the forge-neutral User/Repo and stackgraph.PRRef shapes.
package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v66/github"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

const perPage = 100

// goFactory builds go-github-backed clients.
type goFactory struct{}

// NewFactory returns a forge.Factory backed by the real GitHub REST API (go-github).
func NewFactory() forge.Factory { return goFactory{} }

func (goFactory) ForToken(token string) forge.Client {
	return &goClient{gh: github.NewClient(nil).WithAuthToken(token)}
}

type goClient struct {
	gh *github.Client
}

func (c *goClient) GetAuthenticatedUser(ctx context.Context) (forge.User, error) {
	u, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return forge.User{}, fmt.Errorf("github: get user: %w", err)
	}
	return forge.User{ID: u.GetID(), Login: u.GetLogin(), AvatarURL: u.GetAvatarURL()}, nil
}

func (c *goClient) ListAccessibleRepos(ctx context.Context) ([]forge.Repo, error) {
	// Default affiliation is owner,collaborator,organization_member — everything
	// the user can access.
	opt := &github.RepositoryListByAuthenticatedUserOptions{
		ListOptions: github.ListOptions{PerPage: perPage},
	}
	var out []forge.Repo
	for {
		repos, resp, err := c.gh.Repositories.ListByAuthenticatedUser(ctx, opt)
		if err != nil {
			return nil, fmt.Errorf("github: list repos: %w", err)
		}
		for _, r := range repos {
			out = append(out, mapRepo(r))
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}

func (c *goClient) ListPullRequests(ctx context.Context, owner, name string) ([]stackgraph.PRRef, error) {
	opt := &github.PullRequestListOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: perPage},
	}
	var out []stackgraph.PRRef
	for {
		prs, resp, err := c.gh.PullRequests.List(ctx, owner, name, opt)
		if err != nil {
			return nil, fmt.Errorf("github: list PRs %s/%s: %w", owner, name, err)
		}
		for _, pr := range prs {
			out = append(out, mapPR(pr))
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}

func mapRepo(r *github.Repository) forge.Repo {
	return forge.Repo{
		ForgeRepoID:   r.GetID(),
		Owner:         r.GetOwner().GetLogin(),
		Name:          r.GetName(),
		DefaultBranch: r.GetDefaultBranch(),
		Private:       r.GetPrivate(),
		HTMLURL:       r.GetHTMLURL(),
	}
}

// mapPR converts a go-github PR to stackgraph.PRRef, deriving the
// OPEN/MERGED/CLOSED state to match adapters.PRInfo semantics: a PR with a
// merged-at timestamp is MERGED, an otherwise-closed PR is CLOSED, else OPEN.
func mapPR(pr *github.PullRequest) stackgraph.PRRef {
	state := stackgraph.StateOpen
	switch {
	case !pr.GetMergedAt().IsZero():
		state = "MERGED"
	case strings.EqualFold(pr.GetState(), "closed"):
		state = "CLOSED"
	}
	return stackgraph.PRRef{
		Number:  pr.GetNumber(),
		HeadRef: pr.GetHead().GetRef(),
		BaseRef: pr.GetBase().GetRef(),
		Title:   pr.GetTitle(),
		State:   state,
		URL:     pr.GetHTMLURL(),
		Author:  pr.GetUser().GetLogin(),
		Draft:   pr.GetDraft(),
	}
}

// compile-time assertions.
var (
	_ forge.Factory = goFactory{}
	_ forge.Client  = (*goClient)(nil)
)
