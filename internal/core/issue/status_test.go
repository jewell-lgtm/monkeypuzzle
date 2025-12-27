package issue_test

import (
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

// Integration test: happy path workflow
func TestStatus_IntegrationWorkflow(t *testing.T) {
	fs := adapters.NewMemoryFS()

	// Create issue with todo status
	content := `---
title: My Feature
status: todo
---

# My Feature

Description here.
`
	if err := fs.WriteFile("issue.md", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	// Parse status
	status, err := issue.ParseStatus("issue.md", fs)
	if err != nil {
		t.Fatalf("ParseStatus failed: %v", err)
	}
	if status != "todo" {
		t.Errorf("expected 'todo', got %q", status)
	}

	// Update to in-progress
	if err := issue.UpdateStatus("issue.md", "in-progress", fs); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	// Verify update
	status, err = issue.ParseStatus("issue.md", fs)
	if err != nil {
		t.Fatalf("ParseStatus after update failed: %v", err)
	}
	if status != "in-progress" {
		t.Errorf("expected 'in-progress', got %q", status)
	}

	// Verify other content preserved
	data, _ := fs.ReadFile("issue.md")
	text := string(data)
	if !contains(text, "title: My Feature") {
		t.Error("title should be preserved")
	}
	if !contains(text, "# My Feature") {
		t.Error("heading should be preserved")
	}
	if !contains(text, "Description here.") {
		t.Error("description should be preserved")
	}

	// Update to done
	if err := issue.UpdateStatus("issue.md", "done", fs); err != nil {
		t.Fatalf("UpdateStatus to done failed: %v", err)
	}
	status, _ = issue.ParseStatus("issue.md", fs)
	if status != "done" {
		t.Errorf("expected 'done', got %q", status)
	}
}

func contains(text, substr string) bool {
	return len(text) >= len(substr) && (text == substr || len(substr) == 0 ||
		(len(text) > 0 && containsSubstr(text, substr)))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Unit tests: edge cases

func TestValidateStatus(t *testing.T) {
	tests := []struct {
		status string
		valid  bool
	}{
		{"todo", true},
		{"in-progress", true},
		{"done", true},
		{"TODO", false},       // case sensitive
		{"In-Progress", false}, // case sensitive
		{"pending", false},
		{"", false},
		{"completed", false},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			if got := issue.ValidateStatus(tt.status); got != tt.valid {
				t.Errorf("ValidateStatus(%q) = %v, want %v", tt.status, got, tt.valid)
			}
		})
	}
}

func TestParseStatus_DefaultWhenMissing(t *testing.T) {
	fs := adapters.NewMemoryFS()
	content := `---
title: No Status Field
---

# No Status Field
`
	_ = fs.WriteFile("issue.md", []byte(content), 0644)

	status, err := issue.ParseStatus("issue.md", fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "todo" {
		t.Errorf("expected default 'todo', got %q", status)
	}
}

func TestParseStatus_NoFrontmatter(t *testing.T) {
	fs := adapters.NewMemoryFS()
	content := `# Just a heading

No frontmatter here.
`
	_ = fs.WriteFile("issue.md", []byte(content), 0644)

	status, err := issue.ParseStatus("issue.md", fs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "todo" {
		t.Errorf("expected default 'todo', got %q", status)
	}
}

func TestParseStatus_InvalidStatus(t *testing.T) {
	fs := adapters.NewMemoryFS()
	content := `---
title: Bad Status
status: invalid-status
---
`
	_ = fs.WriteFile("issue.md", []byte(content), 0644)

	_, err := issue.ParseStatus("issue.md", fs)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestParseStatus_QuotedValue(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "double quotes",
			content: `---
status: "in-progress"
---
`,
			want: "in-progress",
		},
		{
			name: "single quotes",
			content: `---
status: 'done'
---
`,
			want: "done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := adapters.NewMemoryFS()
			_ = fs.WriteFile("issue.md", []byte(tt.content), 0644)

			status, err := issue.ParseStatus("issue.md", fs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.want {
				t.Errorf("got %q, want %q", status, tt.want)
			}
		})
	}
}

func TestParseStatus_FileNotFound(t *testing.T) {
	fs := adapters.NewMemoryFS()

	_, err := issue.ParseStatus("nonexistent.md", fs)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestUpdateStatus_PreservesOtherFields(t *testing.T) {
	fs := adapters.NewMemoryFS()
	content := `---
title: Complex Issue
status: todo
description: Some description
labels: bug, urgent
---

# Complex Issue

Body content here.
`
	_ = fs.WriteFile("issue.md", []byte(content), 0644)

	if err := issue.UpdateStatus("issue.md", "done", fs); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	data, _ := fs.ReadFile("issue.md")
	text := string(data)

	checks := []string{
		"title: Complex Issue",
		"description: Some description",
		"labels: bug, urgent",
		"status: done",
		"# Complex Issue",
		"Body content here.",
	}

	for _, check := range checks {
		if !containsSubstr(text, check) {
			t.Errorf("expected %q in output, got:\n%s", check, text)
		}
	}
}

func TestUpdateStatus_AddsFieldWhenMissing(t *testing.T) {
	fs := adapters.NewMemoryFS()
	content := `---
title: No Status
---

# No Status
`
	_ = fs.WriteFile("issue.md", []byte(content), 0644)

	if err := issue.UpdateStatus("issue.md", "in-progress", fs); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	data, _ := fs.ReadFile("issue.md")
	text := string(data)

	if !containsSubstr(text, "status: in-progress") {
		t.Errorf("expected status field added, got:\n%s", text)
	}
	if !containsSubstr(text, "title: No Status") {
		t.Errorf("expected title preserved, got:\n%s", text)
	}
}

func TestUpdateStatus_AddsWhenNoFrontmatter(t *testing.T) {
	fs := adapters.NewMemoryFS()
	content := `# Just Content

No frontmatter.
`
	_ = fs.WriteFile("issue.md", []byte(content), 0644)

	if err := issue.UpdateStatus("issue.md", "done", fs); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	data, _ := fs.ReadFile("issue.md")
	text := string(data)

	if !containsSubstr(text, "---\nstatus: done\n---") {
		t.Errorf("expected frontmatter added, got:\n%s", text)
	}
	if !containsSubstr(text, "# Just Content") {
		t.Errorf("expected original content preserved, got:\n%s", text)
	}
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	fs := adapters.NewMemoryFS()
	content := `---
status: todo
---
`
	_ = fs.WriteFile("issue.md", []byte(content), 0644)

	err := issue.UpdateStatus("issue.md", "invalid", fs)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestUpdateStatus_FileNotFound(t *testing.T) {
	fs := adapters.NewMemoryFS()

	err := issue.UpdateStatus("nonexistent.md", "done", fs)
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseStatus_CaseInsensitiveFieldName(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name: "lowercase",
			content: `---
status: done
---
`,
			want: "done",
		},
		{
			name: "uppercase",
			content: `---
STATUS: done
---
`,
			want: "done",
		},
		{
			name: "mixed case",
			content: `---
Status: in-progress
---
`,
			want: "in-progress",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := adapters.NewMemoryFS()
			_ = fs.WriteFile("issue.md", []byte(tt.content), 0644)

			status, err := issue.ParseStatus("issue.md", fs)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if status != tt.want {
				t.Errorf("got %q, want %q", status, tt.want)
			}
		})
	}
}
