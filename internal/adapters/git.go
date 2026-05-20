package adapters

import (
	"context"
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
func (g *Git) WorktreeAdd(ctx context.Context, repoRoot, worktreePath string) error {
	_, err := g.exec.RunWithDir(ctx, repoRoot, "git", "worktree", "add", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to create worktree at %s from repo %s: %w", worktreePath, repoRoot, err)
	}
	return nil
}

// WorktreeAddFrom creates a new git worktree at worktreePath checked out on a
// new branch named newBranch, starting at startPoint (a branch name or commit).
// Equivalent to `git worktree add -b <newBranch> <worktreePath> <startPoint>`.
func (g *Git) WorktreeAddFrom(ctx context.Context, repoRoot, worktreePath, newBranch, startPoint string) error {
	_, err := g.exec.RunWithDir(ctx, repoRoot, "git", "worktree", "add", "-b", newBranch, worktreePath, startPoint)
	if err != nil {
		return fmt.Errorf("failed to create worktree at %s on branch %s from %s: %w", worktreePath, newBranch, startPoint, err)
	}
	return nil
}

// WorktreeAddExisting creates a worktree for an existing branch.
// The branch must not be currently checked out anywhere.
func (g *Git) WorktreeAddExisting(ctx context.Context, repoRoot, worktreePath, branchName string) error {
	_, err := g.exec.RunWithDir(ctx, repoRoot, "git", "worktree", "add", worktreePath, branchName)
	if err != nil {
		return fmt.Errorf("failed to create worktree for branch %s at %s: %w", branchName, worktreePath, err)
	}
	return nil
}

// WorktreeAddTracking creates a worktree with a new local branch tracking a remote ref.
// Equivalent to: git worktree add -b <localBranch> --track <worktreePath> <remoteRef>.
func (g *Git) WorktreeAddTracking(ctx context.Context, repoRoot, worktreePath, localBranch, remoteRef string) error {
	_, err := g.exec.RunWithDir(ctx, repoRoot, "git", "worktree", "add", "-b", localBranch, "--track", worktreePath, remoteRef)
	if err != nil {
		return fmt.Errorf("failed to create tracking worktree for %s at %s: %w", remoteRef, worktreePath, err)
	}
	return nil
}

// Fetch runs `git fetch <remote> <ref>` (or `git fetch <remote>` when ref is empty).
func (g *Git) Fetch(ctx context.Context, workDir, remote, ref string) error {
	args := []string{"fetch", remote}
	if ref != "" {
		args = append(args, ref)
	}
	_, err := g.exec.RunWithDir(ctx, workDir, "git", args...)
	if err != nil {
		return fmt.Errorf("failed to fetch %s/%s: %w", remote, ref, err)
	}
	return nil
}

// Remotes lists configured git remotes for the repo.
func (g *Git) Remotes(ctx context.Context, workDir string) ([]string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "remote")
	if err != nil {
		return nil, fmt.Errorf("failed to list remotes: %w", err)
	}
	raw := strings.Split(strings.TrimSpace(string(output)), "\n")
	remotes := make([]string, 0, len(raw))
	for _, r := range raw {
		r = strings.TrimSpace(r)
		if r != "" {
			remotes = append(remotes, r)
		}
	}
	return remotes, nil
}

// LocalBranchExists checks if a local branch exists.
func (g *Git) LocalBranchExists(ctx context.Context, workDir, branchName string) bool {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	return err == nil
}

// ListLocalBranches returns the short names of every local branch in workDir.
// Empty slice (not nil) and no error on a brand-new repo with no branches yet.
func (g *Git) ListLocalBranches(ctx context.Context, workDir string) ([]string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, fmt.Errorf("failed to list local branches: %w", err)
	}
	raw := strings.Split(strings.TrimSpace(string(output)), "\n")
	branches := make([]string, 0, len(raw))
	for _, b := range raw {
		b = strings.TrimSpace(b)
		if b != "" {
			branches = append(branches, b)
		}
	}
	return branches, nil
}

// CheckedOutBranches returns the set of branch names currently checked out in
// any worktree of workDir's repository (main repo or any added worktree).
// Detached HEADs are skipped.
func (g *Git) CheckedOutBranches(ctx context.Context, workDir string) (map[string]bool, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, fmt.Errorf("failed to list worktrees: %w", err)
	}
	checkedOut := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "branch ") {
			continue
		}
		ref := strings.TrimSpace(strings.TrimPrefix(line, "branch "))
		// Porcelain prints the fully-qualified refname; strip refs/heads/ to
		// match what callers compare against.
		ref = strings.TrimPrefix(ref, "refs/heads/")
		if ref != "" {
			checkedOut[ref] = true
		}
	}
	return checkedOut, nil
}

// WorktreeRemove removes a git worktree
func (g *Git) WorktreeRemove(ctx context.Context, repoRoot, worktreePath string) error {
	_, err := g.exec.RunWithDir(ctx, repoRoot, "git", "worktree", "remove", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to remove worktree at %s from repo %s: %w", worktreePath, repoRoot, err)
	}
	return nil
}

// WorktreeRemoveForce removes a git worktree, discarding uncommitted changes
func (g *Git) WorktreeRemoveForce(ctx context.Context, repoRoot, worktreePath string) error {
	_, err := g.exec.RunWithDir(ctx, repoRoot, "git", "worktree", "remove", "--force", worktreePath)
	if err != nil {
		return fmt.Errorf("failed to force remove worktree at %s from repo %s: %w", worktreePath, repoRoot, err)
	}
	return nil
}

// BranchDelete deletes a local git branch
func (g *Git) BranchDelete(ctx context.Context, repoRoot, branchName string, force bool) error {
	flag := "-d"
	if force {
		flag = "-D"
	}
	_, err := g.exec.RunWithDir(ctx, repoRoot, "git", "branch", flag, branchName)
	if err != nil {
		return fmt.Errorf("failed to delete branch %s: %w", branchName, err)
	}
	return nil
}

// RevParseGitDir runs git rev-parse --git-dir to get the git directory.
// Returns the absolute path to the .git directory or worktree gitdir.
func (g *Git) RevParseGitDir(ctx context.Context, workDir string) (string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "rev-parse", "--git-dir")
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
func (g *Git) RepoRoot(ctx context.Context, workDir string) (string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("failed to get repo root: %w", err)
	}
	repoRoot := strings.TrimSpace(string(output))
	repoRoot, _ = filepath.Abs(repoRoot)
	return repoRoot, nil
}

// CurrentBranch gets the current branch name.
// Returns the short name of the current branch (e.g., "main", "piece-1").
func (g *Git) CurrentBranch(ctx context.Context, workDir string) (string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", fmt.Errorf("failed to get current branch: %w", err)
	}
	branch := strings.TrimSpace(string(output))
	return branch, nil
}

// Merge merges the specified branch into the current branch
func (g *Git) Merge(ctx context.Context, workDir, branch string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "merge", branch)
	if err != nil {
		return fmt.Errorf("failed to merge branch %s in %s: %w", branch, workDir, err)
	}
	return nil
}

// IsMainAhead checks if main branch has commits that are not in the piece branch
// Returns true if main is ahead (has commits not in piece), false otherwise
func (g *Git) IsMainAhead(ctx context.Context, workDir, mainBranch, pieceBranch string) (bool, error) {
	// Get the merge-base between main and piece branch
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "merge-base", mainBranch, pieceBranch)
	if err != nil {
		return false, fmt.Errorf("failed to find merge-base: %w", err)
	}
	mergeBase := strings.TrimSpace(string(output))

	// Check if main has commits ahead of the merge-base
	output, err = g.exec.RunWithDir(ctx, workDir, "git", "rev-list", "--count", mergeBase+".."+mainBranch)
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
func (g *Git) GetMainRepoRoot(ctx context.Context, workDir string) (string, error) {
	gitDir, err := g.RevParseGitDir(ctx, workDir)
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
	return g.RepoRoot(ctx, workDir)
}

// Checkout switches to the specified branch
func (g *Git) Checkout(ctx context.Context, workDir, branch string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "checkout", branch)
	if err != nil {
		return fmt.Errorf("failed to checkout branch %s in %s: %w", branch, workDir, err)
	}
	return nil
}

// MergeSquash performs a squash merge of the specified branch into the current branch.
// This stages all changes but does not commit - caller must commit with desired message.
func (g *Git) MergeSquash(ctx context.Context, workDir, branch string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "merge", "--squash", branch)
	if err != nil {
		return fmt.Errorf("failed to squash merge branch %s in %s: %w", branch, workDir, err)
	}
	return nil
}

// RebaseOnto replays the commits in upstream..branch onto newbase, in the given
// worktree (where branch must be checked out). Equivalent to
// `git rebase --onto <newbase> <upstream> <branch>`.
func (g *Git) RebaseOnto(ctx context.Context, workDir, newbase, upstream, branch string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "rebase", "--onto", newbase, upstream, branch)
	if err != nil {
		return fmt.Errorf("failed to rebase %s onto %s in %s: %w", branch, newbase, workDir, err)
	}
	return nil
}

// RebaseAbort aborts an in-progress rebase in the given worktree.
func (g *Git) RebaseAbort(ctx context.Context, workDir string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "rebase", "--abort")
	return err
}

// MergeAbort aborts an in-progress merge in the given worktree.
func (g *Git) MergeAbort(ctx context.Context, workDir string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "merge", "--abort")
	return err
}

// Commit creates a commit with the specified message
func (g *Git) Commit(ctx context.Context, workDir, message string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "commit", "-m", message)
	if err != nil {
		return fmt.Errorf("failed to commit in %s: %w", workDir, err)
	}
	return nil
}

// GetCommitMessages returns commit messages from branch that are not in base
func (g *Git) GetCommitMessages(ctx context.Context, workDir, base, branch string) ([]string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "log", "--format=%s", base+".."+branch)
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
func (g *Git) IsBranchMerged(ctx context.Context, workDir, mainBranch, branchName string) (bool, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "branch", "--merged", mainBranch)
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
func (g *Git) BranchExistsOnRemote(ctx context.Context, workDir, branchName string) (bool, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "ls-remote", "--heads", "origin", branchName)
	if err != nil {
		return false, fmt.Errorf("failed to check remote branches: %w", err)
	}
	return strings.TrimSpace(string(output)) != "", nil
}

// GetBranchCommit returns the commit hash of a branch.
func (g *Git) GetBranchCommit(ctx context.Context, workDir, branchName string) (string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "rev-parse", branchName)
	if err != nil {
		return "", fmt.Errorf("failed to get branch commit: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// IsCommitInBranch checks if a commit exists in a branch's history.
func (g *Git) IsCommitInBranch(ctx context.Context, workDir, commit, branch string) (bool, error) {
	// git merge-base --is-ancestor <commit> <branch> returns 0 if true
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "merge-base", "--is-ancestor", commit, branch)
	if err != nil {
		// Exit code 1 means not an ancestor, other errors are real errors
		if strings.Contains(err.Error(), "exit status 1") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check commit ancestry: %w", err)
	}
	return true, nil
}

// IsClean checks if the working directory has no uncommitted changes.
func (g *Git) IsClean(ctx context.Context, workDir string) (bool, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "git", "status", "--porcelain")
	if err != nil {
		return false, fmt.Errorf("failed to check git status: %w", err)
	}
	return strings.TrimSpace(string(output)) == "", nil
}
