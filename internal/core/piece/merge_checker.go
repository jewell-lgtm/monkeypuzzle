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
}

// MergeCheckerFactory builds a MergeChecker for a given repo, reading the project
// config to decide which provider to instantiate. Returns nil if no PR provider
// is configured or instantiation fails.
type MergeCheckerFactory func(repoRoot string, deps core.Deps) MergeChecker

var mergeCheckerFactory MergeCheckerFactory

// SetMergeCheckerFactory registers a factory that produces MergeChecker instances.
// Called from the pr package's init() to wire the registry-driven provider in
// without a circular import.
func SetMergeCheckerFactory(f MergeCheckerFactory) {
	mergeCheckerFactory = f
}

// getMergeChecker returns a checker for the given repo, or nil if none is configured.
func (h *Handler) getMergeChecker(repoRoot string) MergeChecker {
	if mergeCheckerFactory == nil {
		return nil
	}
	return mergeCheckerFactory(repoRoot, h.deps)
}
