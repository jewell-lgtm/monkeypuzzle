package issue

import (
	"fmt"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// IssueFile represents created issue file info (kept for backwards compatibility)
type IssueFile struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Filename string `json:"filename"`
}

// IssueListItem represents an issue for listing/selection
type IssueListItem struct {
	Path   string `json:"path"`   // Relative path from repo root
	Title  string `json:"title"`  // Display title
	Status string `json:"status"` // Current status
}

// Handler executes issue commands
type Handler struct {
	deps    core.Deps
	workDir string
}

// NewHandler creates a new issue handler with dependencies
func NewHandler(deps core.Deps, workDir string) *Handler {
	return &Handler{deps: deps, workDir: workDir}
}

// Run creates an issue file with the given input.
// Expects input to be pre-validated via WithDefaults() and Validate().
func (h *Handler) Run(input Input) (IssueFile, error) {
	provider, issuesDir, err := h.getProvider()
	if err != nil {
		return IssueFile{}, err
	}

	issue, err := provider.Create(CreateInput(input))
	if err != nil {
		return IssueFile{}, err
	}

	// Convert to IssueFile for backwards compatibility
	filename := filepath.Base(issue.ID)
	relPath := filepath.Join(issuesDir, filename)

	result := IssueFile{
		Path:     relPath,
		Title:    issue.Title,
		Filename: filename,
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: "Created " + result.Path,
		Data:    result,
	})

	return result, nil
}

// ListIssues returns issues from the configured issues directory.
// If statusFilter is non-empty, only issues with matching status are returned.
// Issues are sorted alphabetically by title.
func (h *Handler) ListIssues(statusFilter []string) ([]IssueListItem, error) {
	provider, issuesDir, err := h.getProvider()
	if err != nil {
		return nil, err
	}

	issues, err := provider.List(statusFilter)
	if err != nil {
		return nil, err
	}

	// Convert to IssueListItem for backwards compatibility
	items := make([]IssueListItem, len(issues))
	for i, issue := range issues {
		filename := filepath.Base(issue.ID)
		items[i] = IssueListItem{
			Path:   filepath.Join(issuesDir, filename),
			Title:  issue.Title,
			Status: issue.Status,
		}
	}

	return items, nil
}

// getProvider returns the configured provider and issues directory
func (h *Handler) getProvider() (Provider, string, error) {
	cfg, err := piece.ReadConfig(h.workDir, h.deps.FS)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read config (run mp init first): %w", err)
	}

	if cfg.Issues.Provider != "markdown" {
		return nil, "", fmt.Errorf("unsupported issue provider: %s (only 'markdown' supported)", cfg.Issues.Provider)
	}

	issuesDir, ok := cfg.Issues.Config["directory"]
	if !ok || issuesDir == "" {
		issuesDir = "issues"
	}

	fullIssuesDir := filepath.Join(h.workDir, issuesDir)
	provider := NewMarkdownProvider(h.deps.FS, fullIssuesDir)

	return provider, issuesDir, nil
}
