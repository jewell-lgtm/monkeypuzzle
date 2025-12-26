package issue_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

func setupConfig(t *testing.T, fs *adapters.MemoryFS) {
	t.Helper()
	cfg := initcmd.Config{
		Version: "1",
		Project: initcmd.ProjectConfig{Name: "test"},
		Issues: initcmd.IssueConfig{
			Provider: "markdown",
			Config:   map[string]string{"directory": "issues"},
		},
		PR: initcmd.PRConfig{
			Provider: "github",
			Config:   map[string]string{},
		},
	}
	data, _ := json.Marshal(cfg)
	_ = fs.MkdirAll(".monkeypuzzle", 0755)
	_ = fs.WriteFile(".monkeypuzzle/monkeypuzzle.json", data, 0644)
}

func TestHandler_Run_CreatesIssueFile(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	setupConfig(t, fs)

	handler := issue.NewHandler(deps, "")

	input := issue.Input{
		Title:       "My Feature",
		Description: "Description here",
	}

	result, err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify file created
	data, err := fs.ReadFile(result.Path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}

	content := string(data)

	// Check frontmatter
	if !strings.Contains(content, "title: My Feature") {
		t.Error("expected title in frontmatter")
	}
	if !strings.Contains(content, "status: todo") {
		t.Error("expected status in frontmatter")
	}
	if !strings.Contains(content, "description: Description here") {
		t.Error("expected description in frontmatter")
	}

	// Check body
	if !strings.Contains(content, "# My Feature") {
		t.Error("expected H1 heading in body")
	}
}

func TestHandler_Run_SanitizesFilename(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	setupConfig(t, fs)

	handler := issue.NewHandler(deps, "")

	input := issue.Input{
		Title: "Add Feature X!",
	}

	result, err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.Filename != "add-feature-x.md" {
		t.Errorf("expected filename 'add-feature-x.md', got %q", result.Filename)
	}
}

func TestHandler_Run_DuplicateFilename_AddsNumericSuffix(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	setupConfig(t, fs)

	handler := issue.NewHandler(deps, "")

	// Create first issue
	input := issue.Input{Title: "My Feature"}
	result1, err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Create second issue with same title
	result2, err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result1.Filename != "my-feature.md" {
		t.Errorf("first file: expected 'my-feature.md', got %q", result1.Filename)
	}
	if result2.Filename != "my-feature-1.md" {
		t.Errorf("second file: expected 'my-feature-1.md', got %q", result2.Filename)
	}
}

func TestHandler_Run_ValidationError_MissingTitle(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	setupConfig(t, fs)

	handler := issue.NewHandler(deps, "")

	input := issue.Input{
		Title: "",
	}

	_, err := handler.Run(input)
	if err == nil {
		t.Error("expected validation error for empty title")
	}
}

func TestHandler_Run_OmitsEmptyDescription(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	setupConfig(t, fs)

	handler := issue.NewHandler(deps, "")

	input := issue.Input{
		Title: "Simple Issue",
	}

	result, err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	data, _ := fs.ReadFile(result.Path)
	content := string(data)

	if strings.Contains(content, "description:") {
		t.Error("expected no description field when empty")
	}
}

func TestHandler_Run_ErrorIfNotInitialized(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	// Note: no setupConfig call

	handler := issue.NewHandler(deps, "")

	input := issue.Input{
		Title: "My Feature",
	}

	_, err := handler.Run(input)
	if err == nil {
		t.Error("expected error when config not found")
	}
	if !strings.Contains(err.Error(), "mp init") {
		t.Errorf("expected error to mention 'mp init', got: %v", err)
	}
}

func TestHandler_Run_OutputsSuccessMessage(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	setupConfig(t, fs)

	handler := issue.NewHandler(deps, "")

	input := issue.Input{
		Title: "My Feature",
	}

	_, err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !out.HasSuccess() {
		t.Error("expected success message")
	}
}

func TestSchema(t *testing.T) {
	schema, err := issue.Schema()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var data map[string]string
	if err := json.Unmarshal(schema, &data); err != nil {
		t.Fatalf("invalid schema JSON: %v", err)
	}

	if _, ok := data["title"]; !ok {
		t.Error("expected 'title' in schema")
	}
	if _, ok := data["description"]; !ok {
		t.Error("expected 'description' in schema")
	}
}

func TestParseJSON(t *testing.T) {
	jsonData := `{"title":"My Feature","description":"Some description"}`

	input, err := issue.ParseJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if input.Title != "My Feature" {
		t.Errorf("expected title 'My Feature', got %q", input.Title)
	}
	if input.Description != "Some description" {
		t.Errorf("expected description 'Some description', got %q", input.Description)
	}
}

func TestValidate(t *testing.T) {
	valid := issue.Input{
		Title: "My Feature",
	}

	if err := issue.Validate(valid); err != nil {
		t.Errorf("expected valid input, got error: %v", err)
	}

	invalid := issue.Input{
		Title: "",
	}

	if err := issue.Validate(invalid); err == nil {
		t.Error("expected validation error for empty title")
	}
}

func TestWithDefaults(t *testing.T) {
	input := issue.Input{
		Title:       "  My Feature  ",
		Description: "  Some description  ",
	}

	result := issue.WithDefaults(input)

	if result.Title != "My Feature" {
		t.Errorf("expected trimmed title, got %q", result.Title)
	}
	if result.Description != "Some description" {
		t.Errorf("expected trimmed description, got %q", result.Description)
	}
}
