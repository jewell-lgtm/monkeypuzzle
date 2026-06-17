package piece_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

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
		{
			name:     "long prompt capped to five words",
			input:    "add dark mode to the settings page and persist it",
			expected: "add-dark-mode-to-the",
		},
		{
			name:     "long words capped by length before word count",
			input:    "refactor the authentication middleware configuration system",
			expected: "refactor-the-authentication-middleware",
		},
		{
			name:     "single oversized word hard-cut to length",
			input:    "supercalifragilisticexpialidocioussupercalifragilistic",
			expected: "supercalifragilisticexpialidocioussuperc", // first 40 chars
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

	cfg := initcmd.Config{
		Version: "1",
		Project: initcmd.ProjectConfig{Name: "test-project"},
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
	_ = fs.MkdirAll(filepath.Join(repoRoot, initcmd.DirName), 0755)
	_ = fs.WriteFile(configPath, data, 0644)

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
}

func TestReadConfig_NotFound(t *testing.T) {
	fs := adapters.NewMemoryFS()
	repoRoot := "/repo"

	_, err := piece.ReadConfig(repoRoot, fs)
	if err == nil {
		t.Fatal("expected error when config file doesn't exist")
	}
}
