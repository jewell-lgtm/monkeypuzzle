// Package service is the shared, presentation-agnostic read layer for mp
// server. Both presentation adapters — the HTML/Alpine UI (humans) and the MCP
// server (agents) — call it, mirroring the CLI's "humans and agents share one
// API" design. It reads only from the store (the GitHub cache) and builds PR
// stacks via internal/stackgraph; it never calls GitHub directly.
package service

import (
	"context"
	"log"

	"github.com/jewell-lgtm/monkeypuzzle/internal/server/auth/crypto"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/mprunner"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/store"
	"github.com/jewell-lgtm/monkeypuzzle/internal/server/sync"
	"github.com/jewell-lgtm/monkeypuzzle/internal/stackgraph"
)

// Service is the read API plus sync triggering. All reads are scoped to a user.
type Service struct {
	store   store.Store
	trigger sync.SyncTrigger
	// Optional `mp stack graph` path. When useMpCLI is set (and runner+cipher are
	// wired), RepoStacks reconstructs stacks by shelling out to the CLI instead of
	// rebuilding them in-process — both paths call the same stackgraph.BuildStacks,
	// so there is zero drift. Any failure falls back to the Go path.
	runner   mprunner.MpRunner
	cipher   crypto.TokenCipher
	useMpCLI bool
}

// New builds a Service over the store and sync trigger. runner/cipher/useMpCLI
// wire the optional `mp stack graph` path; pass (nil, nil, false) to keep the
// in-process Go path only.
func New(s store.Store, t sync.SyncTrigger, runner mprunner.MpRunner, cipher crypto.TokenCipher, useMpCLI bool) *Service {
	return &Service{store: s, trigger: t, runner: runner, cipher: cipher, useMpCLI: useMpCLI}
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

	// Optional: reconstruct the forest via `mp stack graph` (same BuildStacks,
	// zero drift). On ANY failure fall through to the in-process Go path below so
	// the page never breaks. Authz above already gated this to the user's repos.
	if s.useMpCLI && s.runner != nil && s.cipher != nil {
		if stacks, ok := s.stacksViaMp(ctx, userID, repo); ok {
			return RepoStacks{Repo: repo, Stacks: stacks}, nil
		}
	}

	prs, err := s.store.ListPullRequestsForRepo(ctx, repo.ID)
	if err != nil {
		return RepoStacks{}, err
	}
	stacks := stackgraph.BuildStacks(storePRsToStackgraph(prs), repo.DefaultBranch)
	return RepoStacks{Repo: repo, Stacks: stacks}, nil
}

// stacksViaMp reconstructs the forest by shelling out to `mp stack graph`,
// reusing the CLI's exact logic. It returns ok=false on ANY failure (load user,
// decrypt token, exec, parse) so the caller falls back to the Go path. The
// user's token is derived exactly as the sync activities do and is handed to mp
// only via the environment — it is never logged here or in the runner.
func (s *Service) stacksViaMp(ctx context.Context, userID int64, repo store.Repo) ([]stackgraph.Stack, bool) {
	user, err := s.store.GetUserByID(ctx, userID)
	if err != nil {
		log.Printf("service: mp stack graph %s/%s: load user: %v", repo.Owner, repo.Name, err)
		return nil, false
	}
	token, err := s.cipher.Decrypt(user.AccessTokenEnc)
	if err != nil {
		log.Printf("service: mp stack graph %s/%s: decrypt token: %v", repo.Owner, repo.Name, err)
		return nil, false
	}
	stacks, err := s.runner.StackGraph(ctx, mprunner.StackGraphInput{
		RepoSlug:      repo.Owner + "/" + repo.Name,
		DefaultBranch: repo.DefaultBranch,
		Provider:      "github",
		Token:         string(token),
	})
	if err != nil {
		log.Printf("service: mp stack graph %s/%s failed, falling back to Go path: %v", repo.Owner, repo.Name, err)
		return nil, false
	}
	return stacks, true
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
