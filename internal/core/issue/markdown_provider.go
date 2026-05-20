package issue

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// MarkdownProvider resolves issues stored as markdown files in a local directory.
// Authoring/listing/searching is the user's job (their editor, ripgrep, etc.) —
// mp only needs to read enough to derive a piece name from an issue ID.
type MarkdownProvider struct {
	fs        core.FS
	issuesDir string // Absolute path to issues directory
}

// NewMarkdownProvider creates a provider for markdown-based issues.
func NewMarkdownProvider(fs core.FS, issuesDir string) *MarkdownProvider {
	return &MarkdownProvider{fs: fs, issuesDir: issuesDir}
}

// Get reads the title from the markdown file's YAML frontmatter.
// id is the file path (absolute or relative to issuesDir).
func (p *MarkdownProvider) Get(id string) (Issue, error) {
	title, err := piece.ExtractIssueName(id, p.fs)
	if err != nil {
		return Issue{}, fmt.Errorf("failed to read issue %s: %w", id, err)
	}
	return Issue{
		ID:    id,
		Title: title,
		Open:  true, // markdown provider has no opened/closed concept — always Open
	}, nil
}
