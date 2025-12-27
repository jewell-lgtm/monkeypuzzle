package adapters

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

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
}

// CreatePR creates a GitHub PR using gh CLI and returns the PR number and URL.
// Must be run from within a git repository.
func (g *GitHub) CreatePR(workDir string, input PRCreateInput) (*PRCreateResult, error) {
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

	output, err := g.exec.RunWithDir(workDir, "gh", args...)
	if err != nil {
		// Extract meaningful error message from gh output
		errMsg := string(output)
		if errMsg != "" {
			return nil, fmt.Errorf("failed to create PR: %s", strings.TrimSpace(errMsg))
		}
		return nil, fmt.Errorf("failed to create PR: %w", err)
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
func (g *GitHub) Push(workDir string) error {
	_, err := g.exec.RunWithDir(workDir, "git", "push", "-u", "origin", "HEAD")
	if err != nil {
		return fmt.Errorf("failed to push to remote: %w", err)
	}
	return nil
}

// GetPRStatus gets the status of a PR by number
func (g *GitHub) GetPRStatus(workDir string, prNumber int) (string, error) {
	output, err := g.exec.RunWithDir(workDir, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "state", "--jq", ".state")
	if err != nil {
		return "", fmt.Errorf("failed to get PR status: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

// IsPRMerged checks if a PR has been merged
func (g *GitHub) IsPRMerged(workDir string, prNumber int) (bool, error) {
	output, err := g.exec.RunWithDir(workDir, "gh", "pr", "view", fmt.Sprintf("%d", prNumber), "--json", "mergedAt")
	if err != nil {
		return false, fmt.Errorf("failed to get PR merge status: %w", err)
	}

	var result struct {
		MergedAt *string `json:"mergedAt"`
	}
	if err := json.Unmarshal(output, &result); err != nil {
		return false, fmt.Errorf("failed to parse PR merge status: %w", err)
	}

	return result.MergedAt != nil && *result.MergedAt != "", nil
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
