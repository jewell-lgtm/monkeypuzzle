package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// GitLab provides GitLab MR operations via the glab CLI.
// Mirrors the GitHub adapter; results are normalised to the same shapes
// (MR is exposed as "PR" in the public types for parity).
type GitLab struct {
	exec core.Exec
}

// NewGitLab creates a GitLab adapter with the provided Exec interface.
func NewGitLab(exec core.Exec) *GitLab {
	return &GitLab{exec: exec}
}

// CreatePR creates a GitLab MR via glab and returns the MR number and URL.
// Must be run from within a git repository.
func (g *GitLab) CreatePR(ctx context.Context, workDir string, input PRCreateInput) (*PRCreateResult, error) {
	args := []string{"mr", "create", "--yes", "--title", input.Title}

	if input.Body != "" {
		args = append(args, "--description", input.Body)
	} else {
		args = append(args, "--description", "")
	}

	if input.Base != "" {
		args = append(args, "--target-branch", input.Base)
	}

	if input.Draft {
		args = append(args, "--draft")
	}

	output, err := g.exec.RunWithDir(ctx, workDir, "glab", args...)
	if err != nil {
		errMsg := string(output)
		if errMsg != "" {
			return nil, fmt.Errorf("failed to create MR: %s%s", strings.TrimSpace(errMsg), cliHint("glab", glabInstallHint))
		}
		return nil, fmt.Errorf("failed to create MR: %w%s", err, cliHint("glab", glabInstallHint))
	}

	mrURL := extractGitLabMRURL(string(output))
	if mrURL == "" {
		return nil, fmt.Errorf("glab mr create returned no URL; output: %q", strings.TrimSpace(string(output)))
	}

	mrNumber, err := extractMRNumberFromURL(mrURL)
	if err != nil {
		return nil, err
	}

	return &PRCreateResult{
		Number: mrNumber,
		URL:    mrURL,
	}, nil
}

// Push pushes the current branch to remote with upstream tracking.
func (g *GitLab) Push(ctx context.Context, workDir string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "git", "push", "-u", "origin", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to push to remote: %w", err)
	}
	return nil
}

// MarkPRReady flips a draft MR to ready (i.e., removes the WIP/Draft prefix).
func (g *GitLab) MarkPRReady(ctx context.Context, workDir string, mrNumber int) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "update", fmt.Sprintf("%d", mrNumber), "--ready")
	if err != nil {
		return fmt.Errorf("failed to mark MR ready: %w", err)
	}
	return nil
}

// GetPRStatus returns the MR state ("opened", "closed", "merged", "locked").
func (g *GitLab) GetPRStatus(ctx context.Context, workDir string, mrNumber int) (string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "view", fmt.Sprintf("%d", mrNumber), "-F", "json")
	if err != nil {
		return "", fmt.Errorf("failed to get MR status: %w", err)
	}
	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse MR view: %w", err)
	}
	return strings.ToUpper(result.State), nil
}

// IsPRMerged reports whether the MR has been merged.
func (g *GitLab) IsPRMerged(ctx context.Context, workDir string, mrNumber int) (bool, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "view", fmt.Sprintf("%d", mrNumber), "-F", "json")
	if err != nil {
		return false, fmt.Errorf("failed to get MR view: %w", err)
	}
	var result struct {
		State    string  `json:"state"`
		MergedAt *string `json:"merged_at"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, fmt.Errorf("failed to parse MR view: %w", err)
	}
	if result.State == "merged" {
		return true, nil
	}
	return result.MergedAt != nil && *result.MergedAt != "", nil
}

// FindMergedPRByBranch looks for a merged MR with the given source branch.
// Returns (merged, mrNumber, error). If no merged MR exists, returns (false, 0, nil).
func (g *GitLab) FindMergedPRByBranch(ctx context.Context, workDir, branchName string) (bool, int, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "list",
		"--source-branch", branchName,
		"--state", "merged",
		"-F", "json",
	)
	if err != nil {
		return false, 0, fmt.Errorf("failed to list merged MRs: %w", err)
	}

	var results []struct {
		IID int `json:"iid"`
	}
	if err := json.Unmarshal(output, &results); err != nil {
		return false, 0, fmt.Errorf("failed to parse MR list: %w", err)
	}
	if len(results) == 0 {
		return false, 0, nil
	}
	return true, results[0].IID, nil
}

// extractGitLabMRURL scans glab output for the first https URL.
// glab prints various banners before the URL; the URL itself is the stable bit.
func extractGitLabMRURL(out string) string {
	for _, field := range strings.Fields(out) {
		if strings.HasPrefix(field, "https://") && strings.Contains(field, "/merge_requests/") {
			return field
		}
	}
	return ""
}

// extractMRNumberFromURL extracts the MR iid from a GitLab MR URL.
// URL format: https://gitlab.example.com/group/project/-/merge_requests/123
func extractMRNumberFromURL(url string) (int, error) {
	parts := strings.Split(strings.TrimRight(url, "/"), "/")
	if len(parts) < 2 {
		return 0, fmt.Errorf("invalid MR URL format: %s", url)
	}
	last := parts[len(parts)-1]
	var n int
	if _, err := fmt.Sscanf(last, "%d", &n); err != nil {
		return 0, fmt.Errorf("failed to parse MR number from URL %s: %w", url, err)
	}
	return n, nil
}
