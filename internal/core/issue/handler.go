package issue

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

const (
	defaultFilePerm = 0644
)

// IssueFile represents created issue file info
type IssueFile struct {
	Path     string `json:"path"`
	Title    string `json:"title"`
	Filename string `json:"filename"`
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
	// Get issues directory from config
	issuesDir, err := h.getIssuesDirectory()
	if err != nil {
		return IssueFile{}, err
	}

	// Ensure issues directory exists
	fullIssuesDir := filepath.Join(h.workDir, issuesDir)
	if err := h.deps.FS.MkdirAll(fullIssuesDir, initcmd.DefaultDirPerm); err != nil {
		return IssueFile{}, fmt.Errorf("failed to create issues directory: %w", err)
	}

	// Generate unique filename
	baseName := piece.SanitizePieceName(input.Title)
	filename, err := h.resolveUniqueFilename(fullIssuesDir, baseName)
	if err != nil {
		return IssueFile{}, err
	}

	// Build markdown content
	content := h.buildMarkdownContent(input)

	// Write file
	filePath := filepath.Join(fullIssuesDir, filename)
	if err := h.deps.FS.WriteFile(filePath, content, defaultFilePerm); err != nil {
		return IssueFile{}, fmt.Errorf("failed to write issue file: %w", err)
	}

	result := IssueFile{
		Path:     filepath.Join(issuesDir, filename),
		Title:    input.Title,
		Filename: filename,
	}

	h.deps.Output.Write(core.Message{
		Type:    core.MsgSuccess,
		Content: "Created " + result.Path,
		Data:    result,
	})

	return result, nil
}

// getIssuesDirectory reads the issues directory from config
func (h *Handler) getIssuesDirectory() (string, error) {
	cfg, err := piece.ReadConfig(h.workDir, h.deps.FS)
	if err != nil {
		return "", fmt.Errorf("failed to read config (run mp init first): %w", err)
	}

	if cfg.Issues.Provider != "markdown" {
		return "", fmt.Errorf("issue provider must be 'markdown', got: %s", cfg.Issues.Provider)
	}

	issuesDir, ok := cfg.Issues.Config["directory"]
	if !ok || issuesDir == "" {
		// Fallback to default
		return "issues", nil
	}

	return issuesDir, nil
}

// resolveUniqueFilename generates a unique filename, adding numeric suffix if needed
func (h *Handler) resolveUniqueFilename(dir, baseName string) (string, error) {
	filename := baseName + ".md"
	path := filepath.Join(dir, filename)

	// Check if file exists
	if _, err := h.deps.FS.Stat(path); err != nil {
		// File doesn't exist, use this name
		return filename, nil
	}

	// Add numeric suffix
	for i := 1; i <= 1000; i++ {
		filename = fmt.Sprintf("%s-%d.md", baseName, i)
		path = filepath.Join(dir, filename)
		if _, err := h.deps.FS.Stat(path); err != nil {
			return filename, nil
		}
	}

	return "", fmt.Errorf("too many issues with similar names")
}

// buildMarkdownContent creates the markdown file content with YAML frontmatter
func (h *Handler) buildMarkdownContent(input Input) []byte {
	var b strings.Builder

	// YAML frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: %s\n", escapeYAMLString(input.Title)))
	b.WriteString(fmt.Sprintf("status: %s\n", piece.StatusTodo))
	if input.Description != "" {
		b.WriteString(fmt.Sprintf("description: %s\n", escapeYAMLString(input.Description)))
	}
	b.WriteString("---\n\n")

	// Markdown body
	b.WriteString(fmt.Sprintf("# %s\n", input.Title))
	if input.Description != "" {
		b.WriteString("\n")
		b.WriteString(input.Description)
		b.WriteString("\n")
	}

	return []byte(b.String())
}

// escapeYAMLString escapes a string for safe YAML output
func escapeYAMLString(s string) string {
	// If string contains special characters, wrap in quotes
	needsQuotes := strings.ContainsAny(s, ":#{}[]!|>\"'`@&*?\\")
	if needsQuotes {
		// Escape internal quotes and wrap
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

// IssueListItem represents an issue for listing/selection
type IssueListItem struct {
	Path   string `json:"path"`   // Relative path from repo root
	Title  string `json:"title"`  // Display title
	Status string `json:"status"` // Current status
}

// ListIssues returns issues from the configured issues directory.
// If statusFilter is non-empty, only issues with matching status are returned.
// Issues are sorted alphabetically by title.
func (h *Handler) ListIssues(statusFilter []string) ([]IssueListItem, error) {
	issuesDir, err := h.getIssuesDirectory()
	if err != nil {
		return nil, err
	}

	fullIssuesDir := filepath.Join(h.workDir, issuesDir)

	entries, err := h.deps.FS.ReadDir(fullIssuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []IssueListItem{}, nil
		}
		return nil, fmt.Errorf("failed to read issues directory: %w", err)
	}

	var issues []IssueListItem
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(fullIssuesDir, entry.Name())
		relPath := filepath.Join(issuesDir, entry.Name())

		// Parse status (defaults to "todo" if not set)
		status, err := piece.ParseStatus(filePath, h.deps.FS)
		if err != nil {
			continue // Skip files with parse errors
		}

		// Apply status filter
		if len(statusFilter) > 0 && !containsStatus(statusFilter, status) {
			continue
		}

		// Extract title
		title, err := piece.ExtractIssueName(filePath, h.deps.FS)
		if err != nil {
			continue
		}

		issues = append(issues, IssueListItem{
			Path:   relPath,
			Title:  title,
			Status: status,
		})
	}

	// Sort by title
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Title < issues[j].Title
	})

	return issues, nil
}

func containsStatus(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
