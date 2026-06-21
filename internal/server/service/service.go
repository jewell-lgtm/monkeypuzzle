// Package service is the shared, presentation-agnostic read layer for mp
// server. Both presentation adapters — the HTML/Alpine UI (humans) and the MCP
// server (agents) — call it, mirroring the CLI's "humans and agents share one
// API" design. It reads only from the store (the GitHub cache) and builds PR
// stacks via internal/stackgraph; it never calls GitHub directly.
package service

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/sync"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// Service is the read API plus sync triggering. All reads are scoped to a user.
type Service struct {
	store   store.Store
	trigger sync.SyncTrigger
}

// New builds a Service over the store and sync trigger.
func New(s store.Store, t sync.SyncTrigger) *Service {
	return &Service{store: s, trigger: t}
}

// RepoStacks is a repo together with its reconstructed PR stacks.
type RepoStacks struct {
	Repo   store.Repo
	Stacks []stackgraph.Stack
}

// ListRepos returns the repos visible to the user (owner/name ordered).
func (s *Service) ListRepos(ctx context.Context, userID int64) ([]store.Repo, error) {
	return s.store.ListReposForUser(ctx, userID)
}

// RepoStacks returns the PR stacks for one of the user's repos. It is scoped to
// the user's visible repos: a repo the user can't see yields store.ErrNotFound.
func (s *Service) RepoStacks(ctx context.Context, userID int64, owner, name string) (RepoStacks, error) {
	repos, err := s.store.ListReposForUser(ctx, userID)
	if err != nil {
		return RepoStacks{}, err
	}
	var repo store.Repo
	found := false
	for _, r := range repos {
		if r.Owner == owner && r.Name == name {
			repo, found = r, true
			break
		}
	}
	if !found {
		return RepoStacks{}, store.ErrNotFound
	}
	prs, err := s.store.ListPullRequestsForRepo(ctx, repo.ID)
	if err != nil {
		return RepoStacks{}, err
	}
	stacks := stackgraph.BuildStacks(storePRsToStackgraph(prs), repo.DefaultBranch)
	return RepoStacks{Repo: repo, Stacks: stacks}, nil
}

// StartSync triggers a refresh of the user's GitHub data.
func (s *Service) StartSync(ctx context.Context, userID int64) (string, error) {
	return s.trigger.StartSync(ctx, userID)
}

// SyncStatus reports the user's current sync status.
func (s *Service) SyncStatus(ctx context.Context, userID int64) (store.SyncStatus, error) {
	return s.trigger.SyncStatus(ctx, userID)
}

func storePRsToStackgraph(prs []store.PullRequest) []stackgraph.PRRef {
	out := make([]stackgraph.PRRef, len(prs))
	for i, p := range prs {
		out[i] = stackgraph.PRRef{
			Number:  p.Number,
			HeadRef: p.HeadRef,
			BaseRef: p.BaseRef,
			Title:   p.Title,
			State:   p.State,
			URL:     p.HTMLURL,
			Author:  p.Author,
			Draft:   p.IsDraft,
		}
	}
	return out
}
