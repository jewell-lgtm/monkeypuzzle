package githubapi

import (
	"context"
	"fmt"

	"github.com/google/go-github/v66/github"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

const perPage = 100

// goFactory builds go-github-backed clients.
type goFactory struct{}

// NewFactory returns a Factory backed by the real GitHub REST API (go-github).
func NewFactory() Factory { return goFactory{} }

func (goFactory) ForToken(token string) GitHubClient {
	return &goClient{gh: github.NewClient(nil).WithAuthToken(token)}
}

type goClient struct {
	gh *github.Client
}

func (c *goClient) GetAuthenticatedUser(ctx context.Context) (GitHubUser, error) {
	u, _, err := c.gh.Users.Get(ctx, "")
	if err != nil {
		return GitHubUser{}, fmt.Errorf("githubapi: get user: %w", err)
	}
	return GitHubUser{ID: u.GetID(), Login: u.GetLogin(), AvatarURL: u.GetAvatarURL()}, nil
}

func (c *goClient) ListAccessibleRepos(ctx context.Context) ([]Repo, error) {
	// Default affiliation is owner,collaborator,organization_member — everything
	// the user can access.
	opt := &github.RepositoryListByAuthenticatedUserOptions{
		ListOptions: github.ListOptions{PerPage: perPage},
	}
	var out []Repo
	for {
		repos, resp, err := c.gh.Repositories.ListByAuthenticatedUser(ctx, opt)
		if err != nil {
			return nil, fmt.Errorf("githubapi: list repos: %w", err)
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
			return nil, fmt.Errorf("githubapi: list PRs %s/%s: %w", owner, name, err)
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
