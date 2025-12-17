package init_test

import (
	"encoding/json"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
)

func TestHandler_Run_CreatesConfig(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	handler := initcmd.NewHandler(deps)

	input := initcmd.Input{
		Name:          "test-project",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}

	err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check config file was created
	data, err := fs.ReadFile(".monkeypuzzle/monkeypuzzle.json")
	if err != nil {
		t.Fatalf("config file not created: %v", err)
	}

	var cfg initcmd.Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("invalid config JSON: %v", err)
	}

	if cfg.Project.Name != "test-project" {
		t.Errorf("expected project name 'test-project', got %q", cfg.Project.Name)
	}
	if cfg.Issues.Provider != "markdown" {
		t.Errorf("expected issue provider 'markdown', got %q", cfg.Issues.Provider)
	}
	if cfg.PR.Provider != "github" {
		t.Errorf("expected pr provider 'github', got %q", cfg.PR.Provider)
	}
	if cfg.Version != "1" {
		t.Errorf("expected version '1', got %q", cfg.Version)
	}
}

func TestHandler_Run_CreatesIssuesDirectory(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	handler := initcmd.NewHandler(deps)

	input := initcmd.Input{
		Name:          "test-project",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}

	err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check issues directory was created
	dirs := fs.Dirs()
	hasIssuesDir := false
	for _, d := range dirs {
		if d == ".monkeypuzzle/issues" {
			hasIssuesDir = true
			break
		}
	}
	if !hasIssuesDir {
		t.Errorf("issues directory not created, dirs: %v", dirs)
	}
}

func TestHandler_Run_OutputsSuccessMessage(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	handler := initcmd.NewHandler(deps)

	input := initcmd.Input{
		Name:          "test-project",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}

	err := handler.Run(input)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !out.HasSuccess() {
		t.Error("expected success message")
	}
}

func TestHandler_Run_ValidationError(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	handler := initcmd.NewHandler(deps)

	tests := []struct {
		name  string
		input initcmd.Input
	}{
		{
			name:  "missing name",
			input: initcmd.Input{IssueProvider: "markdown", PRProvider: "github"},
		},
		{
			name:  "missing issue provider",
			input: initcmd.Input{Name: "test", PRProvider: "github"},
		},
		{
			name:  "missing pr provider",
			input: initcmd.Input{Name: "test", IssueProvider: "markdown"},
		},
		{
			name:  "invalid issue provider",
			input: initcmd.Input{Name: "test", IssueProvider: "jira", PRProvider: "github"},
		},
		{
			name:  "invalid pr provider",
			input: initcmd.Input{Name: "test", IssueProvider: "markdown", PRProvider: "gitlab"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Run(tt.input)
			if err == nil {
				t.Error("expected validation error")
			}
		})
	}
}

func TestHandler_ConfigExists(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	handler := initcmd.NewHandler(deps)

	// Initially no config
	if handler.ConfigExists() {
		t.Error("expected config to not exist initially")
	}

	// Create config
	input := initcmd.Input{
		Name:          "test",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}
	if err := handler.Run(input); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	// Now config exists
	if !handler.ConfigExists() {
		t.Error("expected config to exist after creation")
	}
}

func TestSchema(t *testing.T) {
	schema, err := initcmd.Schema("/path/to/myproject")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var data map[string]string
	if err := json.Unmarshal(schema, &data); err != nil {
		t.Fatalf("invalid schema JSON: %v", err)
	}

	if data["name"] != "myproject" {
		t.Errorf("expected name 'myproject', got %q", data["name"])
	}
	if data["issue_provider"] != "markdown" {
		t.Errorf("expected issue_provider 'markdown', got %q", data["issue_provider"])
	}
	if data["pr_provider"] != "github" {
		t.Errorf("expected pr_provider 'github', got %q", data["pr_provider"])
	}
}

func TestParseJSON(t *testing.T) {
	jsonData := `{"name":"foo","issue_provider":"markdown","pr_provider":"github"}`

	input, err := initcmd.ParseJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if input.Name != "foo" {
		t.Errorf("expected name 'foo', got %q", input.Name)
	}
	if input.IssueProvider != "markdown" {
		t.Errorf("expected issue_provider 'markdown', got %q", input.IssueProvider)
	}
	if input.PRProvider != "github" {
		t.Errorf("expected pr_provider 'github', got %q", input.PRProvider)
	}
}

func TestValidate(t *testing.T) {
	valid := initcmd.Input{
		Name:          "test",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}

	if err := initcmd.Validate(valid); err != nil {
		t.Errorf("expected valid input, got error: %v", err)
	}
}
