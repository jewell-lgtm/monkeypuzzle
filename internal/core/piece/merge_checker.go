package piece

import (
	"context"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// MergeChecker is the minimum surface piece needs from a PR provider to detect
// whether a branch has been merged via a PR/MR.
//
// Defined here (not in the pr package) so piece avoids importing pr, which
// would be circular. The pr.Provider interface happens to satisfy this.
type MergeChecker interface {
	FindMergedByBranch(ctx context.Context, workDir, branchName string) (bool, int, error)
	IsMerged(ctx context.Context, workDir string, number int) (bool, error)
	// FindOpenByBranch returns the open PR/MR for branchName, or a zero Number
	// when it has none. The forge merge strategy needs something to merge, and
	// needs it to target the branch it claims to be merging into.
	FindOpenByBranch(ctx context.Context, workDir, branchName string) (OpenPR, error)
	// Merge squash-merges the PR/MR on the forge.
	Merge(ctx context.Context, workDir string, number int) error
}

// OpenPR is the open PR/MR the forge merge strategy would land. Number is 0
// when the branch has none open. Base is the branch the forge would merge it
// into, which mp checks against the branch it was asked to merge into.
type OpenPR struct {
	Number  int
	Base    string
	IsDraft bool
}

// MergeCheckerFactory builds a MergeChecker for a given repo, reading the project
// config to decide which provider to instantiate. Returns nil if no PR provider
// is configured or instantiation fails.
type MergeCheckerFactory func(repoRoot string, deps core.Deps) MergeChecker

var mergeCheckerFactory MergeCheckerFactory

// SetMergeCheckerFactory registers a factory that produces MergeChecker instances.
// Called from the pr package's init() to wire the registry-driven provider in
// without a circular import. It returns the factory it replaced, so a caller
// that swaps in a stand-in can put the real one back rather than leaving the
// process with none.
func SetMergeCheckerFactory(f MergeCheckerFactory) MergeCheckerFactory {
	previous := mergeCheckerFactory
	mergeCheckerFactory = f
	return previous
}

// getMergeChecker returns a checker for the given repo, or nil if none is configured.
func (h *Handler) getMergeChecker(repoRoot string) MergeChecker {
	if mergeCheckerFactory == nil {
		return nil
	}
	return mergeCheckerFactory(repoRoot, h.deps)
}
