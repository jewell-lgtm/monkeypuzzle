package githubapi

import (
	"strings"

	"github.com/google/go-github/v66/github"

	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

func mapRepo(r *github.Repository) Repo {
	return Repo{
		GitHubRepoID:  r.GetID(),
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
