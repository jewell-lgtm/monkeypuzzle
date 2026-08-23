package pr

import (
	"context"
	"errors"
)

// ErrProviderUnavailable indicates the underlying forge CLI is missing or
// unauthenticated, so PR/MR state can't be read. Callers (e.g. mp stack) should
// degrade to local-only behaviour. Each provider translates its adapter's
// forge-specific sentinel (ErrGHUnavailable / ErrGlabUnavailable) into this.
var ErrProviderUnavailable = errors.New("pr provider unavailable or unauthenticated")

// CreateInput holds input for creating a PR/MR via a provider.
type CreateInput struct {
	Title string
	Body  string
	Base  string
	Draft bool
}

// CreateResult is what a provider returns from Create.
type CreateResult struct {
	Number int
	URL    string
}

// PRInfo is a provider-neutral PR/MR record. It encodes a stack edge:
// HeadRefName is the piece branch, BaseRefName its parent. State is normalised
// to OPEN/MERGED/CLOSED across forges.
type PRInfo struct {
	Number      int
	HeadRefName string
	BaseRefName string
	State       string // OPEN, MERGED, CLOSED
	URL         string
	Draft       bool
}

// Provider abstracts PR/MR backends (GitHub, GitLab, etc.).
// All methods take a workDir so providers can shell out to provider CLIs (gh, glab) inside a worktree.
type Provider interface {
	// Push pushes the current branch to the remote with upstream tracking.
	Push(ctx context.Context, workDir string) error

	// Create opens a PR/MR. If input.Draft is true, the provider must create it as a draft.
	Create(ctx context.Context, workDir string, input CreateInput) (*CreateResult, error)

	// MarkReady flips a draft PR/MR to ready-for-review. Idempotent if already ready.
	MarkReady(ctx context.Context, workDir string, number int) error

	// GetStatus returns a provider-defined status string (e.g. "OPEN", "MERGED", "CLOSED").
	GetStatus(ctx context.Context, workDir string, number int) (string, error)

	// IsMerged returns true if the PR/MR with this number has been merged.
	IsMerged(ctx context.Context, workDir string, number int) (bool, error)

	// FindMergedByBranch checks whether a merged PR/MR exists for the given source branch.
	FindMergedByBranch(ctx context.Context, workDir, branchName string) (bool, int, error)

	// ListPRs returns all PRs/MRs (open, merged, closed) for the repo, the source
	// of truth for stack lineage across machines. Returns ErrProviderUnavailable
	// (with an empty slice) when the forge CLI is missing/unauthenticated so
	// callers can degrade to local lineage.
	ListPRs(ctx context.Context, workDir string) ([]PRInfo, error)

	// SetPRBase re-points the base/target branch of an open PR/MR.
	SetPRBase(ctx context.Context, workDir string, number int, base string) error
}
