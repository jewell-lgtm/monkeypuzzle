//go:build integration

package init_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
)

// Integration tests for init command using real filesystem.
// Run with: go test -tags=integration ./internal/core/init/...

func TestIntegration_Init_CreatesFullStructure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mp-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to temp directory for relative paths
	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
	}
	handler := initcmd.NewHandler(deps)

	input := initcmd.Input{
		Name:          "test-project",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}

	if _, err := handler.Run(input, tmpDir); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify .monkeypuzzle directory
	info, err := os.Stat(".monkeypuzzle")
	if err != nil {
		t.Fatalf(".monkeypuzzle not created: %v", err)
	}
	if !info.IsDir() {
		t.Error(".monkeypuzzle is not a directory")
	}

	// Verify config file
	configData, err := os.ReadFile(".monkeypuzzle/monkeypuzzle.json")
	if err != nil {
		t.Fatalf("config not created: %v", err)
	}

	var cfg initcmd.Config
	if err := json.Unmarshal(configData, &cfg); err != nil {
		t.Fatalf("invalid config JSON: %v", err)
	}

	if cfg.Project.Name != "test-project" {
		t.Errorf("project name = %q, want 'test-project'", cfg.Project.Name)
	}
	if cfg.Issues.Provider != "markdown" {
		t.Errorf("issue provider = %q, want 'markdown'", cfg.Issues.Provider)
	}
	if cfg.Issues.Config["directory"] != "issues" {
		t.Errorf("issues directory = %q, want 'issues'", cfg.Issues.Config["directory"])
	}
	if cfg.PR.Provider != "github" {
		t.Errorf("pr provider = %q, want 'github'", cfg.PR.Provider)
	}
	if cfg.Version != "1" {
		t.Errorf("version = %q, want '1'", cfg.Version)
	}

	// Verify issues directory
	info, err = os.Stat("issues")
	if err != nil {
		t.Fatalf("issues directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("issues is not a directory")
	}

	// Verify .monkeypuzzle/.gitignore contains piece-specific entries
	gitignoreData, err := os.ReadFile(".monkeypuzzle/.gitignore")
	if err != nil {
		t.Fatalf(".gitignore not created: %v", err)
	}
	gitignore := string(gitignoreData)
	for _, entry := range []string{"current-issue.json", "piece-metadata.json", "pr-metadata.json"} {
		if !strings.Contains(gitignore, entry) {
			t.Errorf(".monkeypuzzle/.gitignore should contain %s", entry)
		}
	}
}

func TestIntegration_Init_EnsureGitignore_Standalone(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mp-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	// Create .monkeypuzzle dir manually
	if err := os.MkdirAll(".monkeypuzzle", 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
	}
	handler := initcmd.NewHandler(deps)

	// Call standalone method
	if err := handler.EnsureGitignore(""); err != nil {
		t.Fatalf("EnsureGitignore failed: %v", err)
	}

	// Verify file created
	gitignore, _ := os.ReadFile(".monkeypuzzle/.gitignore")
	if !strings.Contains(string(gitignore), "piece-metadata.json") {
		t.Error("should contain piece-metadata.json")
	}
}

func TestIntegration_Init_ConfigExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mp-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
	}
	handler := initcmd.NewHandler(deps)

	// Initially no config
	if handler.ConfigExists() {
		t.Error("config should not exist initially")
	}

	// Create config
	input := initcmd.Input{
		Name:          "test",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}
	if _, err := handler.Run(input, tmpDir); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Now config exists
	if !handler.ConfigExists() {
		t.Error("config should exist after init")
	}
}

func TestIntegration_Init_DirectoryPermissions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mp-init-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	oldWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
	}
	handler := initcmd.NewHandler(deps)

	input := initcmd.Input{
		Name:          "test",
		IssueProvider: "markdown",
		PRProvider:    "github",
	}
	if _, err := handler.Run(input, tmpDir); err != nil {
		t.Fatalf("init failed: %v", err)
	}

	// Verify directory permissions (0755)
	info, _ := os.Stat(".monkeypuzzle")
	perm := info.Mode().Perm()
	if perm != 0755 {
		t.Errorf(".monkeypuzzle permissions = %o, want 0755", perm)
	}

	info, _ = os.Stat("issues")
	perm = info.Mode().Perm()
	if perm != 0755 {
		t.Errorf("issues permissions = %o, want 0755", perm)
	}

	// Verify file permissions (0644)
	info, _ = os.Stat(".monkeypuzzle/monkeypuzzle.json")
	perm = info.Mode().Perm()
	if perm != 0644 {
		t.Errorf("config permissions = %o, want 0644", perm)
	}

	info, _ = os.Stat(filepath.Join(".monkeypuzzle", ".gitignore"))
	perm = info.Mode().Perm()
	if perm != 0644 {
		t.Errorf(".gitignore permissions = %o, want 0644", perm)
	}
}
