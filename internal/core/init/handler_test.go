package init_test

import (
	"context"
	"encoding/json"
	"strings"
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
		Name:       "test-project",
		PRProvider: "github",
	}

	_, err := handler.Run(input, "")
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
	if cfg.PR.Provider != "github" {
		t.Errorf("expected pr provider 'github', got %q", cfg.PR.Provider)
	}
	if cfg.Version != "1" {
		t.Errorf("expected version '1', got %q", cfg.Version)
	}
}

func TestHandler_Run_OutputsSuccessMessage(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	handler := initcmd.NewHandler(deps)

	input := initcmd.Input{
		Name:       "test-project",
		PRProvider: "github",
	}

	_, err := handler.Run(input, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !out.HasSuccess() {
		t.Error("expected success message")
	}
}

func TestValidate_InvalidInputs(t *testing.T) {
	// Note: Validation is done in input layer, not handler.
	// Handler.Run() expects pre-validated input.
	tests := []struct {
		name  string
		input initcmd.Input
	}{
		{
			name:  "missing name",
			input: initcmd.Input{PRProvider: "github"},
		},
		{
			name:  "missing pr provider",
			input: initcmd.Input{Name: "test"},
		},
		{
			name:  "invalid pr provider",
			input: initcmd.Input{Name: "test", PRProvider: "bitbucket"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := initcmd.Validate(tt.input)
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
	if handler.ConfigExists("") {
		t.Error("expected config to not exist initially")
	}

	// Create config
	input := initcmd.Input{
		Name:       "test",
		PRProvider: "github",
	}
	if _, err := handler.Run(input, ""); err != nil {
		t.Fatalf("failed to create config: %v", err)
	}

	// Now config exists
	if !handler.ConfigExists("") {
		t.Error("expected config to exist after creation")
	}
}

func TestSchema(t *testing.T) {
	schema, err := initcmd.Schema("/path/to/myproject")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(schema, &data); err != nil {
		t.Fatalf("invalid schema JSON: %v", err)
	}

	if data["name"] != "myproject" {
		t.Errorf("expected name 'myproject', got %q", data["name"])
	}
	if data["pr_provider"] != "github" {
		t.Errorf("expected pr_provider 'github', got %q", data["pr_provider"])
	}
	if data["create_skill"] != true {
		t.Errorf("expected create_skill true, got %v", data["create_skill"])
	}
}

func TestParseJSON(t *testing.T) {
	jsonData := `{"name":"foo","pr_provider":"github"}`

	input, err := initcmd.ParseJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if input.Name != "foo" {
		t.Errorf("expected name 'foo', got %q", input.Name)
	}
	if input.PRProvider != "github" {
		t.Errorf("expected pr_provider 'github', got %q", input.PRProvider)
	}
}

func TestValidate(t *testing.T) {
	valid := initcmd.Input{
		Name:       "test",
		PRProvider: "github",
	}

	if err := initcmd.Validate(valid); err != nil {
		t.Errorf("expected valid input, got error: %v", err)
	}
}

// Integration test: mp init creates .monkeypuzzle/.gitignore
func TestHandler_Run_CreatesNestedGitignore(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	deps := core.Deps{FS: fs, Output: out}
	handler := initcmd.NewHandler(deps)

	input := initcmd.Input{
		Name:       "test-project",
		PRProvider: "github",
	}

	_, err := handler.Run(input, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check .monkeypuzzle/.gitignore was created
	data, err := fs.ReadFile(".monkeypuzzle/.gitignore")
	if err != nil {
		t.Fatalf(".monkeypuzzle/.gitignore not created: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "piece-metadata.json") {
		t.Errorf("expected .gitignore to contain piece-metadata.json, got: %s", content)
	}
}

// TestHandler_EnsureExclude: mp's piece-state paths land in the git common
// dir's info/exclude (so every worktree ignores them), idempotently, and the
// call is a no-op outside a git repo or without an exec.
func TestHandler_EnsureExclude(t *testing.T) {
	const common = "/repo/.git"
	exclude := common + "/info/exclude"

	t.Run("writes entries and is idempotent", func(t *testing.T) {
		fs := adapters.NewMemoryFS()
		exec := adapters.NewMockExec()
		exec.AddResponse("git", []string{"rev-parse", "--git-common-dir"}, []byte(common+"\n"), nil)
		if err := fs.MkdirAll(common+"/info", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := fs.WriteFile(exclude, []byte("*.swp\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		h := initcmd.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: exec})

		for i := 0; i < 2; i++ {
			if err := h.EnsureExclude(context.Background(), "/repo", ".monkeypuzzle"); err != nil {
				t.Fatalf("EnsureExclude #%d: %v", i+1, err)
			}
		}
		data, err := fs.ReadFile(exclude)
		if err != nil {
			t.Fatalf("read exclude: %v", err)
		}
		got := string(data)
		if !strings.HasPrefix(got, "*.swp\n") {
			t.Errorf("existing entries must be preserved, got:\n%s", got)
		}
		for _, want := range []string{".monkeypuzzle/piece-metadata.json", ".monkeypuzzle/pr-metadata.json", ".monkeypuzzle/pieces/", ".monkeypuzzle/logs/"} {
			if strings.Count(got, want+"\n") != 1 {
				t.Errorf("want exactly one %q line, got:\n%s", want, got)
			}
		}
	})

	t.Run("custom mp dir", func(t *testing.T) {
		fs := adapters.NewMemoryFS()
		exec := adapters.NewMockExec()
		exec.AddResponse("git", []string{"rev-parse", "--git-common-dir"}, []byte(common+"\n"), nil)
		h := initcmd.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: exec})
		if err := h.EnsureExclude(context.Background(), "/repo", ".DONOTCOMMIT/mp"); err != nil {
			t.Fatal(err)
		}
		data, _ := fs.ReadFile(exclude)
		if !strings.Contains(string(data), ".DONOTCOMMIT/mp/piece-metadata.json\n") {
			t.Errorf("entries must use the configured dir, got:\n%s", data)
		}
	})

	t.Run("no-op outside a git repo", func(t *testing.T) {
		fs := adapters.NewMemoryFS()
		exec := adapters.NewMockExec() // rev-parse unmocked → error → not a repo
		h := initcmd.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: exec})
		if err := h.EnsureExclude(context.Background(), "/nowhere", ""); err != nil {
			t.Fatalf("expected nil outside a repo, got %v", err)
		}
		if _, err := fs.ReadFile(exclude); err == nil {
			t.Error("nothing should be written outside a git repo")
		}
	})

	t.Run("no-op without exec", func(t *testing.T) {
		h := initcmd.NewHandler(core.Deps{FS: adapters.NewMemoryFS(), Output: adapters.NewBufferOutput()})
		if err := h.EnsureExclude(context.Background(), "/repo", ""); err != nil {
			t.Fatalf("expected nil without exec, got %v", err)
		}
	})
}
