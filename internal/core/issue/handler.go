package issue

import (
	"fmt"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/workflow"
)

// IssueFile represents created issue file info (kept for backwards compatibility)
type IssueFile struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Filename string `json:"filename"`
}

// IssueListItem represents an issue for listing/selection
type IssueListItem struct {
	ID       string `json:"id"`       // Provider-specific ID (path for markdown)
	Path     string `json:"path"`     // Relative path from repo root (for display)
	Title    string `json:"title"`    // Display title
	Status   string `json:"status"`   // Current status
	Number   string `json:"number"`   // Human-readable issue number (e.g., "ABC-123")
	Provider string `json:"provider"` // Provider type (markdown, linear)
}

// ToIssueRef converts an IssueListItem to a provider-agnostic IssueRef
func (i IssueListItem) ToIssueRef() piece.IssueRef {
	return piece.IssueRef{
		Provider: i.Provider,
		ID:       i.ID,
		Number:   i.Number,
		Title:    i.Title,
	}
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
	pinfo, err := h.getProvider()
	if err != nil {
		return IssueFile{}, err
	}

	issue, err := pinfo.provider.Create(CreateInput(input))
	if err != nil {
		return IssueFile{}, err
	}

	// Convert to IssueFile for backwards compatibility
	filename := filepath.Base(issue.ID)
	relPath := filepath.Join(pinfo.issuesDir, filename)

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
	pinfo, err := h.getProvider()
	if err != nil {
		return nil, err
	}

	h.pubLoading(true, "Fetching issues...")
	issues, err := pinfo.provider.List(statusFilter)
	h.pubLoading(false, "")
	if err != nil {
		return nil, err
	}

	// Convert to IssueListItem
	items := make([]IssueListItem, len(issues))
	for i, issue := range issues {
		filename := filepath.Base(issue.ID)
		items[i] = IssueListItem{
			ID:       issue.ID,
			Path:     filepath.Join(pinfo.issuesDir, filename),
			Title:    issue.Title,
			Status:   issue.Status,
			Number:   issue.Number,
			Provider: pinfo.providerType,
		}
	}

	return items, nil
}

// SearchIssues returns issues matching search criteria, sorted by recency.
func (h *Handler) SearchIssues(input SearchInput) ([]IssueListItem, error) {
	pinfo, err := h.getProvider()
	if err != nil {
		return nil, err
	}

	h.pubLoading(true, "Searching issues...")
	issues, err := pinfo.provider.SearchIssues(input)
	h.pubLoading(false, "")
	if err != nil {
		return nil, err
	}

	// Convert to IssueListItem
	items := make([]IssueListItem, len(issues))
	for i, issue := range issues {
		filename := filepath.Base(issue.ID)
		items[i] = IssueListItem{
			ID:       issue.ID,
			Path:     filepath.Join(pinfo.issuesDir, filename),
			Title:    issue.Title,
			Status:   issue.Status,
			Number:   issue.Number,
			Provider: pinfo.providerType,
		}
	}

	return items, nil
}

// providerInfo contains provider metadata
type providerInfo struct {
	provider     Provider
	providerType string
	issuesDir    string
}

// getProvider returns the configured provider and issues directory
func (h *Handler) getProvider() (providerInfo, error) {
	cfg, err := piece.ReadConfig(h.workDir, h.deps.FS)
	if err != nil {
		return providerInfo{}, fmt.Errorf("failed to read config (run mp init first): %w", err)
	}

	issuesDir, ok := cfg.Issues.Config["directory"]
	if !ok || issuesDir == "" {
		issuesDir = "issues"
	}

	// Build config with absolute paths for markdown provider
	providerCfg := cfg.Issues.Config
	if cfg.Issues.Provider == "markdown" {
		providerCfg = make(map[string]string)
		for k, v := range cfg.Issues.Config {
			providerCfg[k] = v
		}
		providerCfg["directory"] = filepath.Join(h.workDir, issuesDir)
	}

	wf, err := workflow.LoadForRepo(h.workDir, h.deps.FS)
	if err != nil {
		return providerInfo{}, err
	}

	provider, err := NewProvider(ProviderConfig{
		ProviderType: cfg.Issues.Provider,
		Config:       providerCfg,
		Deps: ProviderDeps{
			FS:   h.deps.FS,
			HTTP: h.deps.HTTP,
		},
		Workflow: wf,
	})
	if err != nil {
		return providerInfo{}, err
	}

	return providerInfo{
		provider:     provider,
		providerType: cfg.Issues.Provider,
		issuesDir:    issuesDir,
	}, nil
}

// pubLoading publishes a loading state change if Loading is configured.
func (h *Handler) pubLoading(active bool, label string) {
	if h.deps.Loading != nil {
		h.deps.Loading.Pub(active, label)
	}
}

// SyncStatus updates the status of an issue via the configured provider.
// This is called by the issue sync subscriber to persist status changes.
func (h *Handler) SyncStatus(issueID string, status string) error {
	pinfo, err := h.getProvider()
	if err != nil {
		return err
	}

	return pinfo.provider.UpdateStatus(issueID, status)
}
