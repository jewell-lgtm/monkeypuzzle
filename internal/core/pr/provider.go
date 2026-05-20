package pr

import "context"

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
}
