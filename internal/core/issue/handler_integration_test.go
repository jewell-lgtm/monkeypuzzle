//go:build integration

package issue_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

func TestIntegration_ListIssues_ReturnsAllIgnoringLegacyStatus(t *testing.T) {
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

	// List all issues; legacy status fields must be ignored, not error.
	issues, err := handler.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues failed: %v", err)
	}

	// All 4 issues should be returned regardless of any legacy status field.
	if len(issues) != 4 {
		t.Errorf("expected 4 issues, got %d", len(issues))
		for _, i := range issues {
			t.Logf("  - %s", i.Title)
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

	// List all issues
	issues, err := handler.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues failed: %v", err)
	}

	// Should return all 2 issues
	if len(issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(issues))
	}
}

func TestIntegration_SearchIssues_FuzzyMatchAndLimit(t *testing.T) {
	// Create temp directory for test
	tmpDir, err := os.MkdirTemp("", "mp-issue-search-*")
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

	// Create issues directory
	issuesDir := filepath.Join(tmpDir, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	// Create issues with various titles for fuzzy matching
	issues := []struct {
		filename string
		content  string
	}{
		{"add-auth.md", "---\ntitle: Add authentication feature\nstatus: todo\n---\n"},
		{"fix-auth-bug.md", "---\ntitle: Fix authentication bug\nstatus: in-progress\n---\n"},
		{"add-logging.md", "---\ntitle: Add logging system\nstatus: todo\n---\n"},
		{"update-docs.md", "---\ntitle: Update documentation\nstatus: done\n---\n"},
	}

	for _, iss := range issues {
		if err := os.WriteFile(filepath.Join(issuesDir, iss.filename), []byte(iss.content), 0644); err != nil {
			t.Fatalf("failed to write issue %s: %v", iss.filename, err)
		}
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
	}
	handler := issue.NewHandler(deps, tmpDir)

	// Test 1: Search with fuzzy query "auth" should match authentication issues
	result, err := handler.SearchIssues(issue.SearchInput{Query: "auth"})
	if err != nil {
		t.Fatalf("SearchIssues failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 issues matching 'auth', got %d", len(result))
	}

	// Test 3: Search with limit
	result, err = handler.SearchIssues(issue.SearchInput{Limit: 2})
	if err != nil {
		t.Fatalf("SearchIssues with limit failed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 issues with limit, got %d", len(result))
	}

	// Test 4: Empty search returns all (up to default limit)
	result, err = handler.SearchIssues(issue.SearchInput{})
	if err != nil {
		t.Fatalf("SearchIssues empty failed: %v", err)
	}
	if len(result) != 4 {
		t.Errorf("expected 4 issues with empty search, got %d", len(result))
	}
}
