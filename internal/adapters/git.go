package adapters

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Git provides git operations using an Exec interface
type Git struct {
	exec core.Exec
}

// NewGit creates a Git adapter with the provided Exec interface
func NewGit(exec core.Exec) *Git {
	return &Git{exec: exec}
}

// WorktreeAdd creates a new git worktree at the specified path
func (g *Git) WorktreeAdd(repoRoot, worktreePath string) error {
	_, err := g.exec.RunWithDir(repoRoot, "git", "worktree", "add", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to create worktree: %w", err)
	}
	return nil
}

// RevParseGitDir runs git rev-parse --git-dir to get the git directory
func (g *Git) RevParseGitDir(workDir string) (string, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("failed to get git dir: %w", err)
	}
	gitDir := strings.TrimSpace(string(output))
	// Convert to absolute path
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(workDir, gitDir)
	}
	gitDir, _ = filepath.Abs(gitDir)
	return gitDir, nil
}

// IsWorktree checks if the git directory indicates a worktree
// Worktrees have .git directories that are either:
// - Files containing "gitdir: /path/to/main/.git/worktrees/name"
// - Directories under .git/worktrees/
func (g *Git) IsWorktree(gitDir string) bool {
	absGitDir, _ := filepath.Abs(gitDir)
	return strings.Contains(absGitDir, "worktrees") || filepath.Base(filepath.Dir(absGitDir)) == "worktrees"
}

// RepoRoot runs git rev-parse --show-toplevel to get the repository root
func (g *Git) RepoRoot(workDir string) (string, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("failed to get repo root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))
	repoRoot, _ = filepath.Abs(repoRoot)
	return repoRoot, nil
}

// CurrentBranch gets the current branch name
func (g *Git) CurrentBranch(workDir string) (string, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))
	return branch, nil
}

// Merge merges the specified branch into the current branch
func (g *Git) Merge(workDir, branch string) error {
	_, err := g.exec.RunWithDir(workDir, "git", "merge", branch)
	if err != nil {
		return fmt.Errorf("failed to merge branch %s: %w", branch, err)
	}
	return nil
}
