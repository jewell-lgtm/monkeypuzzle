package adapters

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// ErrGlabUnavailable indicates the glab CLI is missing or unauthenticated, so
// GitLab state can't be read. Callers should degrade to local-only behavior.
// Mirrors ErrGHUnavailable.
var ErrGlabUnavailable = errors.New("glab CLI unavailable or unauthenticated")

// GitLab provides GitLab MR operations via the glab CLI.
// Mirrors the GitHub adapter; results are normalised to the same shapes
// (MR is exposed as "PR" in the public types for parity).
type GitLab struct {
	exec core.Exec
}

// glabStateToCanonical maps GitLab MR states (opened/closed/merged/locked) onto
// the GitHub-style vocabulary (OPEN/MERGED/CLOSED) that the stack code and the
// rest of mp switch on. "locked" is treated as CLOSED (not open for drift).
func glabStateToCanonical(state string) string {
	switch strings.ToLower(state) {
	case "opened":
		return "OPEN"
	case "merged":
		return "MERGED"
	case "closed", "locked":
		return "CLOSED"
	default:
		return strings.ToUpper(state)
	}
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
		return fmt.Errorf("failed to mark MR ready: %w%s", err, cliHint("glab", glabInstallHint))
	}
	return nil
}

// GetPRStatus returns the MR state ("opened", "closed", "merged", "locked").
func (g *GitLab) GetPRStatus(ctx context.Context, workDir string, mrNumber int) (string, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "view", fmt.Sprintf("%d", mrNumber), "-F", "json")
	if err != nil {
		return "", fmt.Errorf("failed to get MR status: %w%s", err, cliHint("glab", glabInstallHint))
	}
	var result struct {
		State string `json:"state"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse MR view: %w", err)
	}
	return glabStateToCanonical(result.State), nil
}

// IsPRMerged reports whether the MR has been merged.
func (g *GitLab) IsPRMerged(ctx context.Context, workDir string, mrNumber int) (bool, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "view", fmt.Sprintf("%d", mrNumber), "-F", "json")
	if err != nil {
		return false, fmt.Errorf("failed to get MR view: %w%s", err, cliHint("glab", glabInstallHint))
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

// FindMergedPRByBranch checks whether the branch's CURRENT MR is merged.
// Returns (merged, mrNumber, error). If no MR exists, returns (false, 0, nil).
//
// Mirrors the GitHub adapter: a source branch can be reused after an old MR
// merged, so an open MR for the branch means "not merged" regardless of any
// older merged MR — otherwise cleanup would sweep an in-progress piece.
func (g *GitLab) FindMergedPRByBranch(ctx context.Context, workDir, branchName string) (bool, int, error) {
	opened, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "list",
		"--source-branch", branchName,
		"--state", "opened",
		"-F", "json",
	)
	if err != nil {
		return false, 0, fmt.Errorf("failed to list open MRs: %w", err)
	}
	var openMRs []struct {
		IID int `json:"iid"`
	}
	if err := json.Unmarshal(opened, &openMRs); err != nil {
		return false, 0, fmt.Errorf("failed to parse MR list: %w", err)
	}
	if len(openMRs) > 0 {
		return false, 0, nil
	}

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

// ListPRs returns all MRs (opened, merged, closed) for the repo, normalised to
// the same PRInfo shape as the GitHub adapter. This is the source of truth for
// stack lineage across machines. Returns ErrGlabUnavailable (with an empty
// slice) when glab is missing or unauthenticated so callers can degrade to
// local lineage rather than failing. --per-page 100 (GitLab's max) avoids
// silently truncating large stacks (default page size is 30).
func (g *GitLab) ListPRs(ctx context.Context, workDir string) ([]PRInfo, error) {
	output, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "list",
		"--all",
		"--per-page", "100",
		"-F", "json",
	)
	if err != nil {
		return []PRInfo{}, ErrGlabUnavailable
	}

	var rows []struct {
		IID          int    `json:"iid"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
		State        string `json:"state"`
		WebURL       string `json:"web_url"`
	}
	if err := json.Unmarshal(output, &rows); err != nil {
		return []PRInfo{}, fmt.Errorf("failed to parse MR list: %w", err)
	}

	prs := make([]PRInfo, 0, len(rows))
	for _, r := range rows {
		prs = append(prs, PRInfo{
			Number:      r.IID,
			HeadRefName: r.SourceBranch,
			BaseRefName: r.TargetBranch,
			State:       glabStateToCanonical(r.State),
			URL:         r.WebURL,
		})
	}
	return prs, nil
}

// SetPRBase changes the target branch of an open MR via `glab mr update`. Used
// only by the opt-in `mp stack status --apply-bases` path.
func (g *GitLab) SetPRBase(ctx context.Context, workDir string, mrNumber int, base string) error {
	_, err := g.exec.RunWithDir(ctx, workDir, "glab", "mr", "update", fmt.Sprintf("%d", mrNumber), "--target-branch", base)
	if err != nil {
		return fmt.Errorf("failed to set target branch of MR !%d to %s: %w%s", mrNumber, base, err, cliHint("glab", glabInstallHint))
	}
	return nil
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
