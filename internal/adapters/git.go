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
		return fmt.Errorf("failed to create worktree at %s from repo %s: %w", worktreePath, repoRoot, err)
	}
	return nil
}

// WorktreeRemove removes a git worktree
func (g *Git) WorktreeRemove(repoRoot, worktreePath string) error {
	_, err := g.exec.RunWithDir(repoRoot, "git", "worktree", "remove", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to remove worktree at %s from repo %s: %w", worktreePath, repoRoot, err)
	}
	return nil
}

// RevParseGitDir runs git rev-parse --git-dir to get the git directory.
// Returns the absolute path to the .git directory or worktree gitdir.
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

// RepoRoot runs git rev-parse --show-toplevel to get the repository root.
// Returns the absolute path to the top-level directory of the git repository.
func (g *Git) RepoRoot(workDir string) (string, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("failed to get repo root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))
	repoRoot, _ = filepath.Abs(repoRoot)
	return repoRoot, nil
}

// CurrentBranch gets the current branch name.
// Returns the short name of the current branch (e.g., "main", "piece-1").
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
		return fmt.Errorf("failed to merge branch %s in %s: %w", branch, workDir, err)
	}
	return nil
}

// IsMainAhead checks if main branch has commits that are not in the piece branch
// Returns true if main is ahead (has commits not in piece), false otherwise
func (g *Git) IsMainAhead(workDir, mainBranch, pieceBranch string) (bool, error) {
	// Get the merge-base between main and piece branch
	output, err := g.exec.RunWithDir(workDir, "git", "merge-base", mainBranch, pieceBranch)
	if err != nil {
		return false, fmt.Errorf("failed to find merge-base: %w", err)
	}
	mergeBase := strings.TrimSpace(string(output))

	// Check if main has commits ahead of the merge-base
	output, err = g.exec.RunWithDir(workDir, "git", "rev-list", "--count", mergeBase+".."+mainBranch)
	if err != nil {
		return false, fmt.Errorf("failed to count commits: %w", err)
	}

	count := strings.TrimSpace(string(output))
	// If count > 0, main is ahead
	return count != "0", nil
}

// GetMainRepoRoot gets the main repository root from a worktree.
// For worktrees, this finds the main repo by examining the gitdir structure.
// For regular repositories, it returns the same as RepoRoot.
func (g *Git) GetMainRepoRoot(workDir string) (string, error) {
	gitDir, err := g.RevParseGitDir(workDir)
	if err != nil {
		return "", err
	}

	// If it's a worktree, the gitdir will be in .git/worktrees/<name>
	// The main repo root is the parent of .git
	if g.IsWorktree(gitDir) {
		// For worktrees, gitDir is something like /repo/.git/worktrees/piece-1
		// We need to go up to /repo/.git, then to /repo
		mainGitDir := filepath.Dir(filepath.Dir(gitDir))
		mainRepoRoot := filepath.Dir(mainGitDir)
		mainRepoRoot, _ = filepath.Abs(mainRepoRoot)
		return mainRepoRoot, nil
	}

	// Not a worktree, just return the repo root
	return g.RepoRoot(workDir)
}

// Checkout switches to the specified branch
func (g *Git) Checkout(workDir, branch string) error {
	_, err := g.exec.RunWithDir(workDir, "git", "checkout", branch)
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s in %s: %w", branch, workDir, err)
	}
	return nil
}

// MergeSquash performs a squash merge of the specified branch into the current branch.
// This stages all changes but does not commit - caller must commit with desired message.
func (g *Git) MergeSquash(workDir, branch string) error {
	_, err := g.exec.RunWithDir(workDir, "git", "merge", "--squash", branch)
	if err != nil {
		return fmt.Errorf("failed to squash merge branch %s in %s: %w", branch, workDir, err)
	}
	return nil
}

// Commit creates a commit with the specified message
func (g *Git) Commit(workDir, message string) error {
	_, err := g.exec.RunWithDir(workDir, "git", "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("failed to commit in %s: %w", workDir, err)
	}
	return nil
}

// GetCommitMessages returns commit messages from branch that are not in base
func (g *Git) GetCommitMessages(workDir, base, branch string) ([]string, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "log", "--format=%s", base+".."+branch)
	if err != nil {
		return nil, fmt.Errorf("failed to get commit messages: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var messages []string
	for _, line := range lines {
		if line != "" {
			messages = append(messages, line)
		}
	}
	return messages, nil
}

// IsBranchMerged checks if branchName is merged into mainBranch.
// Uses git branch --merged to detect merged branches.
func (g *Git) IsBranchMerged(workDir, mainBranch, branchName string) (bool, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "branch", "--merged", mainBranch)
	if err != nil {
		return false, fmt.Errorf("failed to list merged branches: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		// branch output has format: "  branch-name" or "* current-branch"
		name := strings.TrimSpace(strings.TrimPrefix(line, "*"))
		if name == branchName {
			return true, nil
		}
	}
	return false, nil
}

// BranchExistsOnRemote checks if a branch exists on the remote.
func (g *Git) BranchExistsOnRemote(workDir, branchName string) (bool, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "ls-remote", "--heads", "origin", branchName)
	if err != nil {
		return false, fmt.Errorf("failed to check remote branches: %w", err)
	}
	return strings.TrimSpace(string(output)) != "", nil
}

// GetBranchCommit returns the commit hash of a branch.
func (g *Git) GetBranchCommit(workDir, branchName string) (string, error) {
	output, err := g.exec.RunWithDir(workDir, "git", "rev-parse", branchName)
	if err != nil {
		return "", fmt.Errorf("failed to get branch commit: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// IsCommitInBranch checks if a commit exists in a branch's history.
func (g *Git) IsCommitInBranch(workDir, commit, branch string) (bool, error) {
	// git merge-base --is-ancestor <commit> <branch> returns 0 if true
	_, err := g.exec.RunWithDir(workDir, "git", "merge-base", "--is-ancestor", commit, branch)
	if err != nil {
		// Exit code 1 means not an ancestor, other errors are real errors
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check commit ancestry: %w", err)
	}
	return true, nil
}
