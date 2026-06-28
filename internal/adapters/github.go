package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// cliHint annotates forge-CLI errors with an install pointer when the binary
// is missing from PATH — "exit status 1" alone is a dead end for new users.
func cliHint(bin, installURL string) string {
	if _, err := exec.LookPath(bin); err != nil {
		return fmt.Sprintf(" (%s CLI not found in PATH; install from %s)", bin, installURL)
	}
	return ""
}

const ghInstallHint = "https://cli.github.com"
const glabInstallHint = "https://gitlab.com/gitlab-org/cli"

// ErrGHUnavailable indicates the gh CLI is missing or unauthenticated, so GitHub
// state can't be read. Callers should degrade to local-only behavior.
var ErrGHUnavailable = errors.New("gh CLI unavailable or unauthenticated")

// GitHub provides GitHub operations via gh CLI
type GitHub struct {
	exec core.Exec
}

// NewGitHub creates a GitHub adapter with the provided Exec interface
func NewGitHub(exec core.Exec) *GitHub {
	return &GitHub{exec: exec}
}

// PRCreateResult contains the result of creating a PR
type PRCreateResult struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// PRCreateInput contains input for creating a PR
type PRCreateInput struct {
	Title string
	Body  string
	Base  string // Base branch (e.g., "main")
	Draft bool   // If true, create as draft PR
}

// CreatePR creates a GitHub PR using gh CLI and returns the PR number and URL.
// Must be run from within a git repository.
func (g *GitHub) CreatePR(ctx context.Context, workDir string, input PRCreateInput) (*PRCreateResult, error) {
	// Build gh pr create command
	args := []string{"pr", "create", "--title", input.Title}

	if input.Body != "" {
		args = append(args, "--body", input.Body)
	} else {
		args = append(args, "--body", "")
	}

	if input.Base != "" {
		args = append(args, "--base", input.Base)
	}

	if input.Draft {
		args = append(args, "--draft")
	}

	output, err := g.exec.RunWithDir(ctx, workDir, "gh", args...)
	if err != nil {
		// Extract meaningful error message from gh output
		errMsg := string(output)
		if errMsg != "" {
			return nil, fmt.Errorf("failed to create PR: %s%s", strings.TrimSpace(errMsg), cliHint("gh", ghInstallHint))
		}
		return nil, fmt.Errorf("failed to create PR: %w%s", err, cliHint("gh", ghInstallHint))
	}

	// gh pr create outputs the PR URL
	prURL := strings.TrimSpace(string(output))
	if prURL == "" {
		return nil, fmt.Errorf("gh pr create returned empty output")
	}

	// Extract PR number from URL
	// URL format: https://github.com/owner/repo/pull/123
	prNumber, err := extractPRNumberFromURL(prURL)
	if err != nil {
		return nil, err
	}

	return &PRCreateResult{
		Number: prNumber,
		URL:    prURL,
	}, nil
}

// Push pushes the current branch to remote with upstream tracking
func (g *GitHub) Push(ctx context.Context, workDir string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "push", "-u", "origin", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to push to remote: %w", err)
	}
	return nil
}

// MarkPRReady flips a draft PR to ready-for-review.
func (g *GitHub) MarkPRReady(ctx context.Context, workDir string, prNumber int) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "gh", "pr", "ready", fmt.Sprintf("%d", prNumber))
	if err != nil {
		return fmt.Errorf("failed to mark PR ready: %w%s", err, cliHint("gh", ghInstallHint))
	}
	return nil
}

// GetPRStatus gets the status of a PR by number
func (g *GitHub) GetPRStatus(ctx context.Context, workDir string, prNumber int) (string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "state", "--jq", ".state")
	if err != nil {
		return "", fmt.Errorf("failed to get PR status: %w%s", err, cliHint("gh", ghInstallHint))
	}
	return strings.TrimSpace(string(output)), nil
}

// IsPRMerged checks if a PR has been merged
func (g *GitHub) IsPRMerged(ctx context.Context, workDir string, prNumber int) (bool, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "mergedAt")
	if err != nil {
		return false, fmt.Errorf("failed to get PR merge status: %w%s", err, cliHint("gh", ghInstallHint))
	}

	var result struct {
		MergedAt *string `json:"mergedAt"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, fmt.Errorf("failed to parse PR merge status: %w", err)
	}

	return result.MergedAt != nil && *result.MergedAt != "", nil
}

// FindMergedPRByBranch checks if there's a merged PR for the given branch name.
// Returns (merged, prNumber, error). If no merged PR exists, returns (false, 0, nil).
func (g *GitHub) FindMergedPRByBranch(ctx context.Context, workDir, branchName string) (bool, int, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "gh", "pr", "list",
		"--head", branchName,
		"--state", "merged",
		"--json", "number",
		"--limit", "1",
	)
	if err != nil {
		return false, 0, fmt.Errorf("failed to list merged PRs: %w", err)
	}

	var results []struct {
		Number int `json:"number"`
	}
	if err := json.Unmarshal(output, &results); err != nil {
		return false, 0, fmt.Errorf("failed to parse PR list: %w", err)
	}

	if len(results) == 0 {
		return false, 0, nil
	}

	return true, results[0].Number, nil
}

// PRInfo is one row of `gh pr list --json …`. It encodes a stack edge:
// HeadRefName is the piece branch, BaseRefName its parent. Title/Author/IsDraft
// are populated by the richer ListPRsForRepo fetch; ListPRs leaves them zero.
type PRInfo struct {
	Number      int    `json:"number"`
	HeadRefName string `json:"headRefName"`
	BaseRefName string `json:"baseRefName"`
	State       string `json:"state"` // OPEN, MERGED, CLOSED
	URL         string `json:"url"`
	Title       string `json:"title"`
	Author      struct {
		Login string `json:"login"`
	} `json:"author"`
	IsDraft bool `json:"isDraft"`
}

// ListPRs returns all PRs (open, merged, closed) for the repo. This is the source
// of truth for stack lineage across machines. Returns ErrGHUnavailable (with an
// empty slice) when gh is missing or unauthenticated so callers can degrade to
// local lineage rather than failing.
func (g *GitHub) ListPRs(ctx context.Context, workDir string) ([]PRInfo, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "gh", "pr", "list",
		"--state", "all",
		"--json", "number,headRefName,baseRefName,state,url",
		"--limit", "200",
	)
	if err != nil {
		return []PRInfo{}, ErrGHUnavailable
	}

	var prs []PRInfo
	if err := json.Unmarshal(output, &prs); err != nil {
		return []PRInfo{}, fmt.Errorf("failed to parse PR list: %w", err)
	}
	return prs, nil
}

// ListPRsForRepo lists PRs for an arbitrary repo (owner/name) WITHOUT a local
// clone, via `gh pr list --repo`. Auth comes from the ambient GH_TOKEN/
// GITHUB_TOKEN environment, so a server can shell out as a specific user. Returns
// ErrGHUnavailable when gh is missing or unauthenticated. limit<=0 defaults to 200.
func (g *GitHub) ListPRsForRepo(ctx context.Context, repoSlug string, limit int) ([]PRInfo, error) {
	if limit <= 0 {
		limit = 200
	}
	output, err := g.exec.Run(ctx, "gh", "pr", "list",
		"--repo", repoSlug,
		"--state", "all",
		"--json", "number,headRefName,baseRefName,state,url,title,author,isDraft",
		"--limit", fmt.Sprintf("%d", limit),
	)
	if err != nil {
		return []PRInfo{}, ErrGHUnavailable
	}

	var prs []PRInfo
	if err := json.Unmarshal(output, &prs); err != nil {
		return []PRInfo{}, fmt.Errorf("failed to parse PR list: %w", err)
	}
	return prs, nil
}

// RepoDefaultBranch returns the default branch of repoSlug (owner/name) via
// `gh repo view`, no local clone required. Auth via the ambient GH_TOKEN.
func (g *GitHub) RepoDefaultBranch(ctx context.Context, repoSlug string) (string, error) {
	output, err := g.exec.Run(ctx, "gh", "repo", "view", repoSlug, "--json", "defaultBranchRef")
	if err != nil {
		return "", ErrGHUnavailable
	}

	var v struct {
		DefaultBranchRef struct {
			Name string `json:"name"`
		} `json:"defaultBranchRef"`
	}
	if err := json.Unmarshal(output, &v); err != nil {
		return "", fmt.Errorf("failed to parse repo view: %w", err)
	}
	return v.DefaultBranchRef.Name, nil
}

// SetPRBase changes the base branch of an open PR via `gh pr edit`. Used only by
// the opt-in `mp stack status --apply-bases` path.
func (g *GitHub) SetPRBase(ctx context.Context, workDir string, prNumber int, base string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "gh", "pr", "edit", fmt.Sprintf("%d", prNumber), "--base", base)
	if err != nil {
		return fmt.Errorf("failed to set base of PR #%d to %s: %w", prNumber, base, err)
	}
	return nil
}

// PushForceWithLease force-pushes the current branch (HEAD) with --force-with-lease,
// which refuses to clobber remote commits the local repo hasn't seen. Used after a
// rebase rewrites an already-pushed branch.
func (g *GitHub) PushForceWithLease(ctx context.Context, workDir string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "push", "--force-with-lease", "origin", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to force-push to remote: %w", err)
	}
	return nil
}

// extractPRNumberFromURL extracts the PR number from a GitHub PR URL
func extractPRNumberFromURL(url string) (int, error) {
	// URL format: https://github.com/owner/repo/pull/123
	parts := strings.Split(url, "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid PR URL format: %s", url)
	}

	// The PR number is the last part
	prNumStr := parts[len(parts)-1]
	var prNumber int
	_, err := fmt.Sscanf(prNumStr, "%d", &prNumber)
	if err != nil {
		return 0, fmt.Errorf("failed to parse PR number from URL %s: %w", url, err)
	}

	return prNumber, nil
}
