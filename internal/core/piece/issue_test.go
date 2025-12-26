package piece_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestExtractIssueName_FromFrontmatter(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "test-issue.md"

	content := `---
title: My Awesome Feature
status: open
---

# Description

This is a great feature.
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "My Awesome Feature" {
		t.Errorf("expected 'My Awesome Feature', got: %q", name)
	}
}

func TestExtractIssueName_FromFrontmatter_WithQuotes(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "test-issue.md"

	content := `---
title: "My Awesome Feature"
status: open
---

# Description
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "My Awesome Feature" {
		t.Errorf("expected 'My Awesome Feature', got: %q", name)
	}
}

func TestExtractIssueName_FromH1(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "test-issue.md"

	content := `# My Awesome Feature

This is a great feature.
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "My Awesome Feature" {
		t.Errorf("expected 'My Awesome Feature', got: %q", name)
	}
}

func TestExtractIssueName_FromH1_WithWhitespace(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "test-issue.md"

	content := `   #   My Awesome Feature   

This is a great feature.
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "My Awesome Feature" {
		t.Errorf("expected 'My Awesome Feature', got: %q", name)
	}
}

func TestExtractIssueName_FromFilename(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "my-awesome-feature.md"

	content := `This is a great feature.
No frontmatter or H1.
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "my-awesome-feature" {
		t.Errorf("expected 'my-awesome-feature', got: %q", name)
	}
}

func TestExtractIssueName_Priority_FrontmatterOverH1(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "test-issue.md"

	content := `---
title: Frontmatter Title
---

# H1 Title

Content here.
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "Frontmatter Title" {
		t.Errorf("expected 'Frontmatter Title' (from frontmatter), got: %q", name)
	}
}

func TestExtractIssueName_Priority_H1OverFilename(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "filename-title.md"

	content := `# H1 Title

Content here.
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "H1 Title" {
		t.Errorf("expected 'H1 Title' (from H1), got: %q", name)
	}
}

func TestExtractIssueName_NoFrontmatterOrH1_UsesFilename(t *testing.T) {
	fs := adapters.NewMemoryFS()
	issuePath := "my-feature.md"

	content := `Just some content.
No frontmatter.
No H1 heading.
`
	fs.WriteFile(issuePath, []byte(content), 0644)

	name, err := piece.ExtractIssueName(issuePath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if name != "my-feature" {
		t.Errorf("expected 'my-feature' (from filename), got: %q", name)
	}
}

func TestSanitizePieceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "My Feature",
			expected: "my-feature",
		},
		{
			name:     "with special chars",
			input:    "My Awesome Feature!",
			expected: "my-awesome-feature",
		},
		{
			name:     "with invalid filesystem chars",
			input:    "My/Feature: Test",
			expected: "my-feature-test",
		},
		{
			name:     "with underscores",
			input:    "my_feature_test",
			expected: "my-feature-test",
		},
		{
			name:     "with multiple spaces",
			input:    "My   Feature   Test",
			expected: "my-feature-test",
		},
		{
			name:     "with punctuation",
			input:    "My Feature (v2.0)",
			expected: "my-feature-v2-0",
		},
		{
			name:     "all lowercase",
			input:    "my feature",
			expected: "my-feature",
		},
		{
			name:     "with numbers",
			input:    "Feature 123",
			expected: "feature-123",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "piece",
		},
		{
			name:     "only special chars",
			input:    "!!!",
			expected: "piece",
		},
		{
			name:     "leading/trailing hyphens",
			input:    "-My Feature-",
			expected: "my-feature",
		},
		{
			name:     "multiple consecutive hyphens",
			input:    "My---Feature",
			expected: "my-feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := piece.SanitizePieceName(tt.input)
			if result != tt.expected {
				t.Errorf("input %q: expected %q, got %q", tt.input, tt.expected, result)
			}
		})
	}
}

func TestReadConfig(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repoRoot := "/repo"

	// Create config file
	cfg := initcmd.Config{
		Version: "1",
		Project: initcmd.ProjectConfig{Name: "test-project"},
		Issues: initcmd.IssueConfig{
			Provider: "markdown",
			Config: map[string]string{
				"directory": ".monkeypuzzle/issues",
			},
		},
		PR: initcmd.PRConfig{
			Provider: "github",
			Config:   make(map[string]string),
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	configPath := filepath.Join(repoRoot, initcmd.DirName, initcmd.ConfigFile)
	fs.MkdirAll(filepath.Join(repoRoot, initcmd.DirName), 0755)
	fs.WriteFile(configPath, data, 0644)

	// Read config
	readCfg, err := piece.ReadConfig(repoRoot, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if readCfg.Version != cfg.Version {
		t.Errorf("expected version %q, got %q", cfg.Version, readCfg.Version)
	}

	if readCfg.Project.Name != cfg.Project.Name {
		t.Errorf("expected project name %q, got %q", cfg.Project.Name, readCfg.Project.Name)
	}

	if readCfg.Issues.Provider != cfg.Issues.Provider {
		t.Errorf("expected issue provider %q, got %q", cfg.Issues.Provider, readCfg.Issues.Provider)
	}

	dir, ok := readCfg.Issues.Config["directory"]
	if !ok {
		t.Fatal("expected issues directory in config")
	}

	if dir != ".monkeypuzzle/issues" {
		t.Errorf("expected directory %q, got %q", ".monkeypuzzle/issues", dir)
	}
}

func TestReadConfig_NotFound(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repoRoot := "/repo"

	_, err := piece.ReadConfig(repoRoot, fs)
	if err == nil {
		t.Fatal("expected error when config file doesn't exist")
	}
}

func TestResolveIssuePath_Absolute(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repoRoot := "/repo"
	absPath := "/absolute/path/to/issue.md"

	fs.WriteFile(absPath, []byte("content"), 0644)

	resolved, err := piece.ResolveIssuePath(repoRoot, absPath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resolved != absPath {
		t.Errorf("expected %q, got %q", absPath, resolved)
	}
}

func TestResolveIssuePath_Relative(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repoRoot := "/repo"
	relPath := ".monkeypuzzle/issues/test.md"
	absPath := filepath.Join(repoRoot, relPath)

	fs.MkdirAll(filepath.Dir(absPath), 0755)
	fs.WriteFile(absPath, []byte("content"), 0644)

	resolved, err := piece.ResolveIssuePath(repoRoot, relPath, fs)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if resolved != absPath {
		t.Errorf("expected %q, got %q", absPath, resolved)
	}
}

func TestResolveIssuePath_NotFound(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repoRoot := "/repo"
	issuePath := ".monkeypuzzle/issues/nonexistent.md"

	_, err := piece.ResolveIssuePath(repoRoot, issuePath, fs)
	if err == nil {
		t.Fatal("expected error when issue file doesn't exist")
	}
}
