//go:build integration

package issue_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestIntegration_ListIssues_FiltersTodoStatus(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "mp-issue-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create monkeypuzzle config
	configDir := filepath.Join(tmpDir, ".monkeypuzzle")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configContent := `{"version":"1","project":{"name":"test"},"issues":{"provider":"markdown","config":{"directory":"issues"}},"pr":{"provider":"github","config":{}}}`
	if err := os.WriteFile(filepath.Join(configDir, "monkeypuzzle.json"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create issues directory with issues of different statuses
	issuesDir := filepath.Join(tmpDir, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	todoIssue := `---
title: Todo Feature
status: todo
---

# Todo Feature
`
	inProgressIssue := `---
title: In Progress Feature
status: in-progress
---

# In Progress Feature
`
	doneIssue := `---
title: Done Feature
status: done
---

# Done Feature
`
	noStatusIssue := `---
title: No Status Feature
---

# No Status Feature
`

	if err := os.WriteFile(filepath.Join(issuesDir, "todo-feature.md"), []byte(todoIssue), 0644); err != nil {
		t.Fatalf("failed to write todo issue: %v", err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "in-progress-feature.md"), []byte(inProgressIssue), 0644); err != nil {
		t.Fatalf("failed to write in-progress issue: %v", err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "done-feature.md"), []byte(doneIssue), 0644); err != nil {
		t.Fatalf("failed to write done issue: %v", err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "no-status-feature.md"), []byte(noStatusIssue), 0644); err != nil {
		t.Fatalf("failed to write no-status issue: %v", err)
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
	}
	handler := issue.NewHandler(deps, tmpDir)

	// List only todo issues
	issues, err := handler.ListIssues([]string{piece.StatusTodo})
	if err != nil {
		t.Fatalf("ListIssues failed: %v", err)
	}

	// Should return 2 issues: "Todo Feature" and "No Status Feature" (defaults to todo)
	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
		for _, i := range issues {
			t.Logf("  - %s (%s)", i.Title, i.Status)
		}
	}

	// Verify titles (sorted alphabetically)
	expectedTitles := []string{"No Status Feature", "Todo Feature"}
	for i, expected := range expectedTitles {
		if i >= len(issues) {
			break
		}
		if issues[i].Title != expected {
			t.Errorf("issue %d: expected title %q, got %q", i, expected, issues[i].Title)
		}
	}

	// Verify all returned issues have todo status
	for _, iss := range issues {
		if iss.Status != piece.StatusTodo {
			t.Errorf("expected status 'todo', got %q for issue %q", iss.Status, iss.Title)
		}
	}
}

func TestIntegration_ListIssues_EmptyFilter_ReturnsAll(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "mp-issue-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create monkeypuzzle config
	configDir := filepath.Join(tmpDir, ".monkeypuzzle")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	configContent := `{"version":"1","project":{"name":"test"},"issues":{"provider":"markdown","config":{"directory":"issues"}},"pr":{"provider":"github","config":{}}}`
	if err := os.WriteFile(filepath.Join(configDir, "monkeypuzzle.json"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create issues directory with issues of different statuses
	issuesDir := filepath.Join(tmpDir, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	todoIssue := `---
title: Alpha Feature
status: todo
---
`
	doneIssue := `---
title: Beta Feature
status: done
---
`
	if err := os.WriteFile(filepath.Join(issuesDir, "alpha.md"), []byte(todoIssue), 0644); err != nil {
		t.Fatalf("failed to write issue: %v", err)
	}
	if err := os.WriteFile(filepath.Join(issuesDir, "beta.md"), []byte(doneIssue), 0644); err != nil {
		t.Fatalf("failed to write issue: %v", err)
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
	}
	handler := issue.NewHandler(deps, tmpDir)

	// List all issues (empty filter)
	issues, err := handler.ListIssues(nil)
	if err != nil {
		t.Fatalf("ListIssues failed: %v", err)
	}

	// Should return all 2 issues
	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}
}
