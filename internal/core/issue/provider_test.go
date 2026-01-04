package issue_test

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
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
	if result.Status != piece.StatusTodo {
		t.Errorf("expected status 'todo', got %q", result.Status)
	}
	if result.ID != "/issues/test-issue.md" {
		t.Errorf("expected ID '/issues/test-issue.md', got %q", result.ID)
	}
}

func TestMarkdownProvider_Get(t *testing.T) {
	fs := adapters.NewMemoryFS()
	_ = fs.MkdirAll("/issues", 0755)
	_ = fs.WriteFile("/issues/my-issue.md", []byte("---\ntitle: My Issue\nstatus: in-progress\n---\n"), 0644)

	provider := issue.NewMarkdownProvider(fs, "/issues")

	result, err := provider.Get("/issues/my-issue.md")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if result.Title != "My Issue" {
		t.Errorf("expected title 'My Issue', got %q", result.Title)
	}
	if result.Status != "in-progress" {
		t.Errorf("expected status 'in-progress', got %q", result.Status)
	}
}

func TestMarkdownProvider_UpdateStatus(t *testing.T) {
	fs := adapters.NewMemoryFS()
	_ = fs.MkdirAll("/issues", 0755)
	_ = fs.WriteFile("/issues/my-issue.md", []byte("---\ntitle: My Issue\nstatus: todo\n---\n"), 0644)

	provider := issue.NewMarkdownProvider(fs, "/issues")

	err := provider.UpdateStatus("/issues/my-issue.md", "done")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify status was updated
	result, _ := provider.Get("/issues/my-issue.md")
	if result.Status != "done" {
		t.Errorf("expected status 'done', got %q", result.Status)
	}
}

func TestMarkdownProvider_List(t *testing.T) {
	fs := adapters.NewMemoryFS()
	_ = fs.MkdirAll("/issues", 0755)
	_ = fs.WriteFile("/issues/a.md", []byte("---\ntitle: Alpha\nstatus: todo\n---\n"), 0644)
	_ = fs.WriteFile("/issues/b.md", []byte("---\ntitle: Beta\nstatus: done\n---\n"), 0644)
	_ = fs.WriteFile("/issues/c.md", []byte("---\ntitle: Gamma\nstatus: todo\n---\n"), 0644)

	provider := issue.NewMarkdownProvider(fs, "/issues")

	// List all
	all, err := provider.List(nil)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 issues, got %d", len(all))
	}

	// Filter by status
	todoOnly, err := provider.List([]string{"todo"})
	if err != nil {
		t.Fatalf("List filtered failed: %v", err)
	}
	if len(todoOnly) != 2 {
		t.Errorf("expected 2 todo issues, got %d", len(todoOnly))
	}
}

func TestProvider_Interface(t *testing.T) {
	// Verify MarkdownProvider implements Provider interface
	var _ issue.Provider = (*issue.MarkdownProvider)(nil)
}
