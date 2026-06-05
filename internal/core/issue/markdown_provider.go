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
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/workflow"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/fuzzy"
)

const defaultFilePerm = 0644

// MarkdownProvider implements Provider using markdown files
type MarkdownProvider struct {
	fs        core.FS
	issuesDir string // Absolute path to issues directory
	wf        workflow.Workflow
}

// NewMarkdownProvider creates a provider for markdown-based issues
func NewMarkdownProvider(fs core.FS, issuesDir string, wf workflow.Workflow) *MarkdownProvider {
	return &MarkdownProvider{
		fs:        fs,
		issuesDir: issuesDir,
		wf:        wf,
	}
}

// frontmatterForState returns the literal status string the markdown provider
// should write to frontmatter for the given workflow state. The default
// workflow's provider_map maps each state to its own name, so the function is
// effectively identity for default-flow projects; custom workflows can
// override via provider_map.markdown.<state>.frontmatter.
func (p *MarkdownProvider) frontmatterForState(state string) string {
	if entry, ok := p.wf.ProviderEntry("markdown", state); ok && entry.Frontmatter != "" {
		return entry.Frontmatter
	}
	return state
}

// stateForFrontmatter is the inverse of frontmatterForState.
func (p *MarkdownProvider) stateForFrontmatter(frontmatter string) string {
	for _, s := range p.wf.AllStates() {
		if entry, ok := p.wf.ProviderEntry("markdown", s); ok && entry.Frontmatter == frontmatter {
			return s
		}
	}
	// Round-trip identity: if no explicit mapping, the frontmatter value IS
	// the workflow state name. This is the default-workflow case.
	return frontmatter
}

// Create creates a new issue markdown file
func (p *MarkdownProvider) Create(input CreateInput) (Issue, error) {
	// Ensure issues directory exists
	if err := p.fs.MkdirAll(p.issuesDir, initcmd.DefaultDirPerm); err != nil {
		return Issue{}, fmt.Errorf("failed to create issues directory: %w", err)
	}

	// Generate unique filename
	baseName := piece.SanitizePieceName(input.Title)
	filename, err := p.resolveUniqueFilename(baseName)
	if err != nil {
		return Issue{}, err
	}

	// Build markdown content using the workflow's initial state.
	initialFrontmatter := p.frontmatterForState(p.wf.Initial)
	content := buildMarkdownContent(input, initialFrontmatter)

	// Write file
	filePath := filepath.Join(p.issuesDir, filename)
	if err := p.fs.WriteFile(filePath, content, defaultFilePerm); err != nil {
		return Issue{}, fmt.Errorf("failed to write issue file: %w", err)
	}

	return Issue{
		ID:          filePath,
		Title:       input.Title,
		Status:      p.wf.Initial,
		Description: input.Description,
	}, nil
}

// List returns issues from the issues directory
func (p *MarkdownProvider) List(statusFilter []string) ([]Issue, error) {
	entries, err := p.fs.ReadDir(p.issuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Issue{}, nil
		}
		return nil, fmt.Errorf("failed to read issues directory: %w", err)
	}

	var issues []Issue
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(p.issuesDir, entry.Name())

		issue, err := p.Get(filePath)
		if err != nil {
			// Log warning for unexpected errors (file read or parse issues)
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", entry.Name(), err)
			continue
		}

		// Apply status filter
		if len(statusFilter) > 0 && !containsStatus(statusFilter, issue.Status) {
			continue
		}

		issues = append(issues, issue)
	}

	// Sort by title
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Title < issues[j].Title
	})

	return issues, nil
}

// SearchIssues returns issues matching search criteria, sorted by modification time
func (p *MarkdownProvider) SearchIssues(input SearchInput) ([]Issue, error) {
	entries, err := p.fs.ReadDir(p.issuesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Issue{}, nil
		}
		return nil, fmt.Errorf("failed to read issues directory: %w", err)
	}

	// Collect issues with mod times for sorting
	type issueWithTime struct {
		issue   Issue
		modTime int64
	}
	var issues []issueWithTime

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		filePath := filepath.Join(p.issuesDir, entry.Name())

		issue, err := p.Get(filePath)
		if err != nil {
			continue
		}

		// Apply status filter
		if len(input.Status) > 0 && !containsStatus(input.Status, issue.Status) {
			continue
		}

		// Apply query filter (fuzzy match on title)
		if input.Query != "" && !fuzzy.Match(input.Query, issue.Title) {
			continue
		}

		// Get modification time
		info, err := entry.Info()
		modTime := int64(0)
		if err == nil {
			modTime = info.ModTime().UnixNano()
		}

		issues = append(issues, issueWithTime{issue: issue, modTime: modTime})
	}

	// Sort by modification time (most recent first)
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].modTime > issues[j].modTime
	})

	// Apply limit
	limit := input.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if len(issues) > limit {
		issues = issues[:limit]
	}

	// Extract just the issues
	result := make([]Issue, len(issues))
	for i, it := range issues {
		result[i] = it.issue
	}

	return result, nil
}

// Get returns an issue by its file path
func (p *MarkdownProvider) Get(id string) (Issue, error) {
	raw, err := piece.ParseStatus(id, p.fs)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to parse status: %w", err)
	}

	title, err := piece.ExtractIssueName(id, p.fs)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to extract title: %w", err)
	}

	return Issue{
		ID:     id,
		Title:  title,
		Status: p.stateForFrontmatter(raw),
	}, nil
}

// UpdateStatus translates the given workflow state to its frontmatter
// representation and writes it to the issue file.
func (p *MarkdownProvider) UpdateStatus(id string, status string) error {
	return piece.UpdateStatus(id, p.frontmatterForState(status), p.fs)
}

// resolveUniqueFilename generates a unique filename, adding numeric suffix if needed
func (p *MarkdownProvider) resolveUniqueFilename(baseName string) (string, error) {
	filename := baseName + ".md"
	path := filepath.Join(p.issuesDir, filename)

	// Check if file exists
	if _, err := p.fs.Stat(path); err != nil {
		// File doesn't exist, use this name
		return filename, nil
	}

	// Add numeric suffix
	for i := 1; i <= 1000; i++ {
		filename = fmt.Sprintf("%s-%d.md", baseName, i)
		path = filepath.Join(p.issuesDir, filename)
		if _, err := p.fs.Stat(path); err != nil {
			return filename, nil
		}
	}

	return "", fmt.Errorf("too many issues with similar names")
}

// buildMarkdownContent creates the markdown file content with YAML
// frontmatter. The status string is the literal value to write
// (already translated from a workflow state name by the caller).
func buildMarkdownContent(input CreateInput, status string) []byte {
	var b strings.Builder

	// YAML frontmatter
	b.WriteString("---\n")
	b.WriteString(fmt.Sprintf("title: %s\n", escapeYAMLString(input.Title)))
	b.WriteString(fmt.Sprintf("status: %s\n", status))
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
	needsQuotes := strings.ContainsAny(s, ":#{}[]!|>\"'`@&*?\\")
	if needsQuotes {
		escaped := strings.ReplaceAll(s, `"`, `\"`)
		return `"` + escaped + `"`
	}
	return s
}

func containsStatus(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
