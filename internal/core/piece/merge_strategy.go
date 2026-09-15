package piece

import (
	"errors"
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// MergeStrategy is how `mp merge` lands a piece.
type MergeStrategy string

const (
	// MergeLocal squash-merges the piece branch into its target in the main
	// checkout. This is mp's original behaviour and the default.
	MergeLocal MergeStrategy = "local"
	// MergeForge merges the piece's open PR/MR on the forge, then fast-forwards
	// the local target branch onto the result.
	MergeForge MergeStrategy = "forge"
)

// ErrNoOpenPR means the forge strategy has nothing to merge: the piece branch
// has no open PR/MR. Merging on the forge is a claim that the work went through
// a review surface, so mp refuses rather than quietly merging it locally.
var ErrNoOpenPR = errors.New("no open PR for this branch")

// ErrNoForgeProvider means the forge strategy was asked for but no PR provider
// could be resolved for the project.
var ErrNoForgeProvider = errors.New("no PR provider configured for the forge merge strategy")

// ErrPRBaseMismatch means the branch's open PR/MR targets a different base than
// the branch mp was asked to merge into. The forge would land the work against
// the PR's own base, so mp refuses rather than report a merge into the target.
var ErrPRBaseMismatch = errors.New("open PR targets a different base branch")

// ErrPRIsDraft means the branch's open PR/MR is still a draft. Flipping it to
// ready is a deliberate step (`mp pr ready`), never something merge does.
var ErrPRIsDraft = errors.New("open PR is still a draft")

// ParseMergeStrategy validates a configured or flag-supplied strategy name.
// An empty string means "unset" and resolves to the next layer down.
func ParseMergeStrategy(v string) (MergeStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case string(MergeLocal):
		return MergeLocal, nil
	case string(MergeForge):
		return MergeForge, nil
	}
	return "", fmt.Errorf("unknown merge strategy %q: want %q or %q", v, MergeLocal, MergeForge)
}

// ResolveMergeStrategy picks the strategy for a merge, most specific first:
// the per-call override, then the project's `merge.strategy`, then the
// user-level fallback, then "local". repoRoot may be a piece worktree; the
// project config lives in the main checkout's state dir, so it is resolved
// the way NewMergeChecker does.
func ResolveMergeStrategy(repoRoot string, fs core.FS, override, userDefault string) (MergeStrategy, error) {
	if s, err := ParseMergeStrategy(override); err != nil || s != "" {
		return s, err
	}

	configRoot := repoRoot
	if mainRoot, err := projectdir.MainRepoRoot(repoRoot); err == nil {
		configRoot = mainRoot
	}
	if cfg, err := ReadConfig(configRoot, fs); err == nil && cfg.Merge != nil {
		s, err := ParseMergeStrategy(cfg.Merge.Strategy)
		if err != nil {
			return "", fmt.Errorf("project config: %w", err)
		}
		if s != "" {
			return s, nil
		}
	}

	if s, err := ParseMergeStrategy(userDefault); err != nil {
		return "", fmt.Errorf("user config: %w", err)
	} else if s != "" {
		return s, nil
	}

	return MergeLocal, nil
}
