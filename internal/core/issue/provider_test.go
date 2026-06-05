package issue_test

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

func TestMarkdownProvider_Create(t *testing.T) {
	fs := adapters.NewMemoryFS()
	provider := issue.NewMarkdownProvider(fs, "/issues")

	input := issue.CreateInput{
		Title:       "Test Issue",
		Description: "Test description",
	}

	result, err := provider.Create(input)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if result.Title != "Test Issue" {
		t.Errorf("expected title 'Test Issue', got %q", result.Title)
	}
	if result.ID != "/issues/test-issue.md" {
		t.Errorf("expected ID '/issues/test-issue.md', got %q", result.ID)
	}

	// Frontmatter must not contain a status field.
	content, _ := fs.ReadFile("/issues/test-issue.md")
	if got := string(content); containsLine(got, "status:") {
		t.Errorf("created issue should not contain a status field, got:\n%s", got)
	}
}

// containsLine reports whether any line of s begins with prefix (after trimming).
func containsLine(s, prefix string) bool {
	for _, line := range splitLines(s) {
		if len(line) >= len(prefix) && line[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func TestMarkdownProvider_Get(t *testing.T) {
	fs := adapters.NewMemoryFS()
	_ = fs.MkdirAll("/issues", 0755)
	// A legacy file that still carries a status field must read without error
	// (the status field is simply ignored now).
	_ = fs.WriteFile("/issues/my-issue.md", []byte("---\ntitle: My Issue\nstatus: in-progress\n---\n"), 0644)

	provider := issue.NewMarkdownProvider(fs, "/issues")

	result, err := provider.Get("/issues/my-issue.md")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.Title != "My Issue" {
		t.Errorf("expected title 'My Issue', got %q", result.Title)
	}
}

func TestMarkdownProvider_List(t *testing.T) {
	fs := adapters.NewMemoryFS()
	_ = fs.MkdirAll("/issues", 0755)
	// Mix of files, some with a legacy status field, some without.
	_ = fs.WriteFile("/issues/a.md", []byte("---\ntitle: Alpha\nstatus: todo\n---\n"), 0644)
	_ = fs.WriteFile("/issues/b.md", []byte("---\ntitle: Beta\n---\n"), 0644)
	_ = fs.WriteFile("/issues/c.md", []byte("---\ntitle: Gamma\nstatus: done\n---\n"), 0644)

	provider := issue.NewMarkdownProvider(fs, "/issues")

	all, err := provider.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 issues, got %d", len(all))
	}
}

func TestProvider_Interface(t *testing.T) {
	// Verify MarkdownProvider implements Provider interface
	var _ issue.Provider = (*issue.MarkdownProvider)(nil)
}
