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
	"github.com/jewell-lgtm/monkeypuzzle/pkg/fuzzy"
)

const defaultFilePerm = 0644

// MarkdownProvider implements Provider using markdown files
type MarkdownProvider struct {
	fs        core.FS
	issuesDir string // Absolute path to issues directory
}

// NewMarkdownProvider creates a provider for markdown-based issues
func NewMarkdownProvider(fs core.FS, issuesDir string) *MarkdownProvider {
	return &MarkdownProvider{
		fs:        fs,
		issuesDir: issuesDir,
	}
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

	// Build markdown content
	content := buildMarkdownContent(input)

	// Write file
	filePath := filepath.Join(p.issuesDir, filename)
	if err := p.fs.WriteFile(filePath, content, defaultFilePerm); err != nil {
		return Issue{}, fmt.Errorf("failed to write issue file: %w", err)
	}

	return Issue{
		ID:          filePath,
		Title:       input.Title,
		Description: input.Description,
	}, nil
}

// List returns issues from the issues directory
func (p *MarkdownProvider) List() ([]Issue, error) {
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
	title, err := piece.ExtractIssueName(id, p.fs)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to extract title: %w", err)
	}

	return Issue{
		ID:    id,
		Title: title,
	}, nil
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

// buildMarkdownContent creates the markdown file content with YAML frontmatter
func buildMarkdownContent(input CreateInput) []byte {
	var b strings.Builder

	// YAML frontmatter
	b.WriteString("---\n")
	fmt.Fprintf(&b, "title: %s\n", escapeYAMLString(input.Title))
	if input.Description != "" {
		fmt.Fprintf(&b, "description: %s\n", escapeYAMLString(input.Description))
	}
	b.WriteString("---\n\n")

	// Markdown body
	fmt.Fprintf(&b, "# %s\n", input.Title)
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
