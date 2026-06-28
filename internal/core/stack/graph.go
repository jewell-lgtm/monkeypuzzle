package stack

import (
	"context"
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// GraphResult is the stdout JSON of `mp stack graph`: a repo's forest of stacked
// PRs, reconstructed purely from forge PR base->head edges with no local clone.
// Stacks is exactly the []stackgraph.Stack the server's read path produces, so
// the hosted dashboard and the CLI share one forest builder and cannot drift.
type GraphResult struct {
	Repo          string             `json:"repo"`
	DefaultBranch string             `json:"default_branch"`
	Provider      string             `json:"provider"`
	Stacks        []stackgraph.Stack `json:"stacks"`
}

// Graph reconstructs the stacked-PR forest for in.Repo straight from the forge,
// with no local git repository. Auth flows through the ambient GH_TOKEN/
// GITLAB_TOKEN environment to gh/glab, so a caller (e.g. mp-server) can run it as
// a specific user. Provider defaults to github; gitlab is not wired yet.
//
// This is the CLI side of "the PaaS uses the CLI internally": the server shells
// out to `mp stack graph` rather than reimplementing stack reconstruction, and
// both ultimately call stackgraph.BuildStacks — one source of truth.
func (h *Handler) Graph(ctx context.Context, in GraphInput) (GraphResult, error) {
	in = WithGraphDefaults(in)
	if in.Repo == "" {
		return GraphResult{}, fmt.Errorf("repo is required (owner/name)")
	}
	if in.Provider != ProviderGitHub {
		return GraphResult{}, fmt.Errorf("provider %q is not supported yet (only %q)", in.Provider, ProviderGitHub)
	}

	prs, err := h.github.ListPRsForRepo(ctx, in.Repo, in.Limit)
	if err != nil {
		return GraphResult{}, fmt.Errorf("cannot read stacks for %s: %w", in.Repo, err)
	}

	defaultBranch := in.DefaultBranch
	if defaultBranch == "" {
		defaultBranch, err = h.github.RepoDefaultBranch(ctx, in.Repo)
		if err != nil {
			return GraphResult{}, fmt.Errorf("failed to resolve default branch for %s: %w", in.Repo, err)
		}
	}

	refs := make([]stackgraph.PRRef, 0, len(prs))
	for _, p := range prs {
		refs = append(refs, stackgraph.PRRef{
			Number:  p.Number,
			HeadRef: p.HeadRefName,
			BaseRef: p.BaseRefName,
			Title:   p.Title,
			State:   p.State,
			URL:     p.URL,
			Author:  p.Author.Login,
			Draft:   p.IsDraft,
		})
	}

	return GraphResult{
		Repo:          in.Repo,
		DefaultBranch: defaultBranch,
		Provider:      in.Provider,
		Stacks:        stackgraph.BuildStacks(refs, defaultBranch),
	}, nil
}
