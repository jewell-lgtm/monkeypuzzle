// Package gitlab is the go-gitlab-backed implementation of forge.Client: the
// read-only GitLab REST access layer for mp server. It maps go-gitlab types into
// the forge-neutral User/Repo and stackgraph.PRRef shapes. The factory binds a
// base URL so a self-managed GitLab instance can be targeted; the default is
// https://gitlab.com.
package gitlab

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/forge"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// defaultBaseURL is the SaaS GitLab API endpoint.
const defaultBaseURL = "https://gitlab.com"

const perPage = 100

// glFactory builds go-gitlab-backed clients bound to a base URL.
type glFactory struct {
	baseURL string
}

// NewFactory returns a forge.Factory backed by the real GitLab REST API
// (go-gitlab). An empty baseURL defaults to https://gitlab.com.
func NewFactory(baseURL string) forge.Factory {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return glFactory{baseURL: baseURL}
}

func (f glFactory) ForToken(token string) forge.Client {
	return &glClient{token: token, baseURL: f.baseURL}
}

// glClient lazily constructs the go-gitlab client; NewClient validates the base
// URL, so construction is deferred to the call sites that can surface the error.
type glClient struct {
	token   string
	baseURL string
}

func (c *glClient) api() (*gitlab.Client, error) {
	gl, err := gitlab.NewClient(c.token, gitlab.WithBaseURL(c.baseURL))
	if err != nil {
		return nil, fmt.Errorf("gitlab: client: %w", err)
	}
	return gl, nil
}

func (c *glClient) GetAuthenticatedUser(ctx context.Context) (forge.User, error) {
	gl, err := c.api()
	if err != nil {
		return forge.User{}, err
	}
	u, _, err := gl.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return forge.User{}, fmt.Errorf("gitlab: get user: %w", err)
	}
	return forge.User{ID: u.ID, Login: u.Username, AvatarURL: u.AvatarURL}, nil
}

func (c *glClient) ListAccessibleRepos(ctx context.Context) ([]forge.Repo, error) {
	gl, err := c.api()
	if err != nil {
		return nil, err
	}
	opt := &gitlab.ListProjectsOptions{
		Membership:  gitlab.Ptr(true),
		ListOptions: gitlab.ListOptions{PerPage: perPage},
	}
	var out []forge.Repo
	for {
		projects, resp, err := gl.Projects.ListProjects(opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list projects: %w", err)
		}
		for _, p := range projects {
			out = append(out, mapProject(p))
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}

func (c *glClient) ListPullRequests(ctx context.Context, owner, name string) ([]stackgraph.PRRef, error) {
	gl, err := c.api()
	if err != nil {
		return nil, err
	}
	pid := owner + "/" + name
	opt := &gitlab.ListProjectMergeRequestsOptions{
		State:       gitlab.Ptr("all"),
		ListOptions: gitlab.ListOptions{PerPage: perPage},
	}
	var out []stackgraph.PRRef
	for {
		mrs, resp, err := gl.MergeRequests.ListProjectMergeRequests(pid, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("gitlab: list MRs %s: %w", pid, err)
		}
		for _, mr := range mrs {
			out = append(out, mapMR(mr))
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}
	return out, nil
}

// mapProject converts a go-gitlab Project to forge.Repo. Owner is the immediate
// namespace path (a known limitation for nested group/subgroup/project paths).
func mapProject(p *gitlab.Project) forge.Repo {
	owner := ""
	if p.Namespace != nil {
		owner = p.Namespace.Path
	}
	return forge.Repo{
		ForgeRepoID:   p.ID,
		Owner:         owner,
		Name:          p.Path,
		DefaultBranch: p.DefaultBranch,
		Private:       p.Visibility != gitlab.PublicVisibility,
		HTMLURL:       p.WebURL,
	}
}

// mapMR converts a go-gitlab merge request to stackgraph.PRRef, normalizing the
// GitLab state vocabulary (opened|closed|merged|locked) to the OPEN|MERGED|
// CLOSED used everywhere else. The MR iid (project-scoped number) is used as the
// PR number.
func mapMR(mr *gitlab.BasicMergeRequest) stackgraph.PRRef {
	author := ""
	if mr.Author != nil {
		author = mr.Author.Username
	}
	return stackgraph.PRRef{
		Number:  int(mr.IID),
		HeadRef: mr.SourceBranch,
		BaseRef: mr.TargetBranch,
		Title:   mr.Title,
		State:   mapState(mr.State),
		URL:     mr.WebURL,
		Author:  author,
		Draft:   mr.Draft,
	}
}

// mapState normalizes a GitLab MR state to the OPEN|MERGED|CLOSED vocabulary.
func mapState(s string) string {
	switch s {
	case "merged":
		return "MERGED"
	case "closed", "locked":
		return "CLOSED"
	default: // "opened" and any unknown future state
		return stackgraph.StateOpen
	}
}

// compile-time assertions.
var (
	_ forge.Factory = glFactory{}
	_ forge.Client  = (*glClient)(nil)
)
