//go:build integration

package piece_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

// Integration tests for hooks that use real filesystem and shell scripts.
// Run with: go test -tags=integration ./internal/core/piece/...

func TestIntegration_HookRunner_ExecutesRealScript(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mp-hook-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create hooks directory
	hooksDir := filepath.Join(tmpDir, ".monkeypuzzle", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	// Create a hook script that writes to a file
	outputFile := filepath.Join(tmpDir, "hook-output.txt")
	hookScript := `#!/bin/bash
echo "Piece: $MP_PIECE_NAME" > "` + outputFile + `"
echo "Worktree: $MP_WORKTREE_PATH" >> "` + outputFile + `"
echo "RepoRoot: $MP_REPO_ROOT" >> "` + outputFile + `"
echo "MainBranch: $MP_MAIN_BRANCH" >> "` + outputFile + `"
echo "Session: $MP_SESSION_NAME" >> "` + outputFile + `"
`
	hookPath := filepath.Join(hooksDir, piece.HookOnPieceCreate)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatalf("failed to write hook script: %v", err)
	}

	// Run the hook
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	runner := piece.NewHookRunner(deps)

	ctx := piece.HookContext{
		PieceName:    "test-piece",
		WorktreePath: "/tmp/test-worktree",
		RepoRoot:     "/tmp/test-repo",
		MainBranch:   "main",
		SessionName:  "mp-piece-test",
	}

	err = runner.RunHook(context.Background(), tmpDir, piece.HookOnPieceCreate, ctx)
	if err != nil {
		t.Fatalf("hook execution failed: %v", err)
	}

	// Verify the hook wrote the expected output
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read hook output: %v", err)
	}

	output := string(content)
	if !strings.Contains(output, "Piece: test-piece") {
		t.Errorf("expected MP_PIECE_NAME in output, got: %s", output)
	}
	if !strings.Contains(output, "Worktree: /tmp/test-worktree") {
		t.Errorf("expected MP_WORKTREE_PATH in output, got: %s", output)
	}
	if !strings.Contains(output, "RepoRoot: /tmp/test-repo") {
		t.Errorf("expected MP_REPO_ROOT in output, got: %s", output)
	}
	if !strings.Contains(output, "MainBranch: main") {
		t.Errorf("expected MP_MAIN_BRANCH in output, got: %s", output)
	}
	if !strings.Contains(output, "Session: mp-piece-test") {
		t.Errorf("expected MP_SESSION_NAME in output, got: %s", output)
	}
}

func TestIntegration_HookRunner_FailingScript(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mp-hook-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create hooks directory
	hooksDir := filepath.Join(tmpDir, ".monkeypuzzle", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	// Create a hook script that fails
	hookScript := `#!/bin/bash
echo "Running pre-merge checks..."
echo "ERROR: Tests failed!" >&2
exit 1
`
	hookPath := filepath.Join(hooksDir, piece.HookBeforePieceMerge)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatalf("failed to write hook script: %v", err)
	}

	// Run the hook
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	runner := piece.NewHookRunner(deps)

	err = runner.RunHook(context.Background(), tmpDir, piece.HookBeforePieceMerge, piece.HookContext{
		PieceName: "test-piece",
	})

	if err == nil {
		t.Fatal("expected error from failing hook")
	}

	if !strings.Contains(err.Error(), "hook") {
		t.Errorf("expected error about hook failure, got: %v", err)
	}
}

func TestIntegration_HookRunner_NonExecutableScript(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "mp-hook-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create hooks directory
	hooksDir := filepath.Join(tmpDir, ".monkeypuzzle", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	// Create a hook script without executable permission
	hookScript := `#!/bin/bash
echo "This should not run"
`
	hookPath := filepath.Join(hooksDir, piece.HookAfterPieceUpdate)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0644); err != nil { // 0644, not 0755
		t.Fatalf("failed to write hook script: %v", err)
	}

	// Run the hook
	out := adapters.NewBufferOutput()
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: out,
		Exec:   adapters.NewOSExec(),
	}
	runner := piece.NewHookRunner(deps)

	err = runner.RunHook(context.Background(), tmpDir, piece.HookAfterPieceUpdate, piece.HookContext{
		PieceName: "test-piece",
	})

	if err != nil {
		t.Errorf("expected no error for non-executable hook, got: %v", err)
	}

	// Should have a warning
	if !out.HasWarning() {
		t.Error("expected warning about non-executable hook")
	}
}

func TestIntegration_HookRunner_MissingHook(t *testing.T) {
	// Create temp directory with hooks dir but no hooks
	tmpDir, err := os.MkdirTemp("", "mp-hook-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create hooks directory (empty)
	hooksDir := filepath.Join(tmpDir, ".monkeypuzzle", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	// Run the hook
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	runner := piece.NewHookRunner(deps)

	err = runner.RunHook(context.Background(), tmpDir, piece.HookOnPieceCreate, piece.HookContext{
		PieceName: "test-piece",
	})

	if err != nil {
		t.Errorf("expected no error for missing hook, got: %v", err)
	}
}

func TestIntegration_HookRunner_MissingHooksDir(t *testing.T) {
	// Create temp directory without hooks dir
	tmpDir, err := os.MkdirTemp("", "mp-hook-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Run the hook
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	runner := piece.NewHookRunner(deps)

	err = runner.RunHook(context.Background(), tmpDir, piece.HookOnPieceCreate, piece.HookContext{
		PieceName: "test-piece",
	})

	if err != nil {
		t.Errorf("expected no error for missing hooks dir, got: %v", err)
	}
}

func TestIntegration_FullPieceUpdateFlow_WithHooks(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure git for the test
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config email failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config name failed: %v\n%s", err, out)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Create main branch
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch failed: %v\n%s", err, out)
	}

	// Create hooks directory and before-piece-update hook
	hooksDir := filepath.Join(tmpDir, ".monkeypuzzle", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	hookOutputFile := filepath.Join(tmpDir, "hook-ran.txt")
	hookScript := `#!/bin/bash
echo "before-update ran for $MP_PIECE_NAME" > "` + hookOutputFile + `"
`
	hookPath := filepath.Join(hooksDir, piece.HookBeforePieceUpdate)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatalf("failed to write hook script: %v", err)
	}

	// Create a worktree
	piecesDir := filepath.Join(tmpDir, "pieces")
	if err := os.MkdirAll(piecesDir, 0755); err != nil {
		t.Fatalf("failed to create pieces dir: %v", err)
	}

	worktreePath := filepath.Join(piecesDir, "test-piece")
	cmd = exec.Command("git", "worktree", "add", "-b", "test-piece", worktreePath)
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git worktree add failed: %v\n%s", err, out)
	}

	// Run the hook directly with HookRunner
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	runner := piece.NewHookRunner(deps)

	ctx := piece.HookContext{
		PieceName:    "test-piece",
		WorktreePath: worktreePath,
		RepoRoot:     tmpDir,
		MainBranch:   "main",
	}

	err = runner.RunHook(context.Background(), tmpDir, piece.HookBeforePieceUpdate, ctx)
	if err != nil {
		t.Fatalf("hook execution failed: %v", err)
	}

	// Verify hook ran
	content, err := os.ReadFile(hookOutputFile)
	if err != nil {
		t.Fatalf("hook output file not created: %v", err)
	}

	if !strings.Contains(string(content), "before-update ran for test-piece") {
		t.Errorf("unexpected hook output: %s", string(content))
	}
}

func TestIntegration_CreatePieceFromIssue_WithFrontmatter(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure git for the test
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config email failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config name failed: %v\n%s", err, out)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Create main branch
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch failed: %v\n%s", err, out)
	}

	// Create monkeypuzzle config
	mpDir := filepath.Join(tmpDir, ".monkeypuzzle")
	if err := os.MkdirAll(mpDir, 0755); err != nil {
		t.Fatalf("failed to create .monkeypuzzle dir: %v", err)
	}

	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "markdown",
    "config": {"directory": ".monkeypuzzle/issues"}
  },
  "pr": {"provider": "github", "config": {}}
}`
	configPath := filepath.Join(mpDir, "monkeypuzzle.json")
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create issue file with frontmatter
	issuesDir := filepath.Join(mpDir, "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	issueContent := `---
title: My Awesome Feature
status: open
---

# Description

This is a great feature.
`
	issuePath := filepath.Join(issuesDir, "my-feature.md")
	if err := os.WriteFile(issuePath, []byte(issueContent), 0644); err != nil {
		t.Fatalf("failed to write issue file: %v", err)
	}

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create piece from issue
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	relIssuePath := ".monkeypuzzle/issues/my-feature.md"
	info, err := handler.CreatePieceFromIssue(context.Background(), tmpDir, relIssuePath, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceFromIssue failed: %v", err)
	}

	// Verify piece name is sanitized
	expectedName := "my-awesome-feature"
	if info.Name != expectedName {
		t.Errorf("expected piece name %q, got %q", expectedName, info.Name)
	}

	// Verify marker file exists
	markerPath := filepath.Join(info.WorktreePath, ".monkeypuzzle", "current-issue.json")
	markerData, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("marker file not found: %v", err)
	}

	var marker piece.CurrentIssueMarker
	if err := json.Unmarshal(markerData, &marker); err != nil {
		t.Fatalf("failed to unmarshal marker: %v", err)
	}

	if marker.IssueName != "My Awesome Feature" {
		t.Errorf("expected issue name 'My Awesome Feature', got %q", marker.IssueName)
	}

	if marker.PieceName != expectedName {
		t.Errorf("expected piece name %q, got %q", expectedName, marker.PieceName)
	}
}

func TestIntegration_CreatePieceFromIssue_WithH1Heading(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo (same setup as above)
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Create issue file with H1 (no frontmatter)
	issuesDir := filepath.Join(tmpDir, ".monkeypuzzle", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	issueContent := `# My Feature

This is a great feature.
`
	issuePath := filepath.Join(issuesDir, "my-feature.md")
	if err := os.WriteFile(issuePath, []byte(issueContent), 0644); err != nil {
		t.Fatalf("failed to write issue file: %v", err)
	}

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create piece from issue
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	relIssuePath := ".monkeypuzzle/issues/my-feature.md"
	info, err := handler.CreatePieceFromIssue(context.Background(), tmpDir, relIssuePath, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceFromIssue failed: %v", err)
	}

	// Verify piece name is from H1
	expectedName := "my-feature"
	if info.Name != expectedName {
		t.Errorf("expected piece name %q, got %q", expectedName, info.Name)
	}
}

func TestIntegration_CreatePieceFromIssue_WithFilenameFallback(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Create issue file with no frontmatter or H1
	issuesDir := filepath.Join(tmpDir, ".monkeypuzzle", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	issueContent := `Just some content.
No frontmatter.
No H1 heading.
`
	issuePath := filepath.Join(issuesDir, "my-feature.md")
	if err := os.WriteFile(issuePath, []byte(issueContent), 0644); err != nil {
		t.Fatalf("failed to write issue file: %v", err)
	}

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create piece from issue
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	relIssuePath := ".monkeypuzzle/issues/my-feature.md"
	info, err := handler.CreatePieceFromIssue(context.Background(), tmpDir, relIssuePath, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceFromIssue failed: %v", err)
	}

	// Verify piece name is from filename
	expectedName := "my-feature"
	if info.Name != expectedName {
		t.Errorf("expected piece name %q, got %q", expectedName, info.Name)
	}
}

func TestIntegration_CreatePieceFromIssue_SanitizesName(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Create issue file with special characters in title
	issuesDir := filepath.Join(tmpDir, ".monkeypuzzle", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	issueContent := `---
title: My Awesome Feature (v2.0)!
---

Content here.
`
	issuePath := filepath.Join(issuesDir, "my-feature.md")
	if err := os.WriteFile(issuePath, []byte(issueContent), 0644); err != nil {
		t.Fatalf("failed to write issue file: %v", err)
	}

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create piece from issue
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	relIssuePath := ".monkeypuzzle/issues/my-feature.md"
	info, err := handler.CreatePieceFromIssue(context.Background(), tmpDir, relIssuePath, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceFromIssue failed: %v", err)
	}

	// Verify piece name is sanitized
	expectedName := "my-awesome-feature-v2-0"
	if info.Name != expectedName {
		t.Errorf("expected piece name %q, got %q", expectedName, info.Name)
	}
}

// Helper functions for integration tests

func setupGitRepo(t *testing.T, tmpDir string) {
	// Initialize a git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure git for the test
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config email failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config name failed: %v\n%s", err, out)
	}

	// Create initial commit
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "initial commit")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Create main branch
	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch failed: %v\n%s", err, out)
	}
}

func setupMonkeypuzzleConfig(t *testing.T, tmpDir string) {
	// Create monkeypuzzle config
	mpDir := filepath.Join(tmpDir, ".monkeypuzzle")
	if err := os.MkdirAll(mpDir, 0755); err != nil {
		t.Fatalf("failed to create .monkeypuzzle dir: %v", err)
	}

	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "markdown",
    "config": {"directory": ".monkeypuzzle/issues"}
  },
  "pr": {"provider": "github", "config": {}}
}`
	configPath := filepath.Join(mpDir, "monkeypuzzle.json")
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}
}

func TestIntegration_CreatePieceFromIssue_UpdatesStatusToInProgress(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Create issue file with todo status
	issuesDir := filepath.Join(tmpDir, ".monkeypuzzle", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	issueContent := `---
title: My Feature
status: todo
---

# My Feature

Description here.
`
	issuePath := filepath.Join(issuesDir, "my-feature.md")
	if err := os.WriteFile(issuePath, []byte(issueContent), 0644); err != nil {
		t.Fatalf("failed to write issue file: %v", err)
	}

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create piece from issue
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	relIssuePath := ".monkeypuzzle/issues/my-feature.md"
	_, err = handler.CreatePieceFromIssue(context.Background(), tmpDir, relIssuePath, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceFromIssue failed: %v", err)
	}

	// Verify issue status was updated to in-progress
	updatedContent, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("failed to read updated issue: %v", err)
	}

	if !strings.Contains(string(updatedContent), "status: in-progress") {
		t.Errorf("expected status to be updated to in-progress, got:\n%s", string(updatedContent))
	}
}

func TestIntegration_CreatePieceFromIssue_SkipsUpdateIfNotTodo(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Create issue file with done status (should not be changed)
	issuesDir := filepath.Join(tmpDir, ".monkeypuzzle", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	issueContent := `---
title: Completed Feature
status: done
---

# Completed Feature
`
	issuePath := filepath.Join(issuesDir, "completed-feature.md")
	if err := os.WriteFile(issuePath, []byte(issueContent), 0644); err != nil {
		t.Fatalf("failed to write issue file: %v", err)
	}

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create piece from issue
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	relIssuePath := ".monkeypuzzle/issues/completed-feature.md"
	_, err = handler.CreatePieceFromIssue(context.Background(), tmpDir, relIssuePath, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceFromIssue failed: %v", err)
	}

	// Verify issue status was NOT changed (still done)
	updatedContent, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("failed to read issue: %v", err)
	}

	if !strings.Contains(string(updatedContent), "status: done") {
		t.Errorf("expected status to remain 'done', got:\n%s", string(updatedContent))
	}
}

func TestIntegration_ListPieces_And_SwitchPiece(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	// Get repo root for ListPieces
	git := adapters.NewGit(adapters.NewOSExec())
	repoRoot, err := git.RepoRoot(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("failed to get repo root: %v", err)
	}

	// Initially no pieces
	pieces, err := handler.ListPieces(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("ListPieces failed: %v", err)
	}
	if len(pieces) != 0 {
		t.Errorf("expected 0 pieces, got %d", len(pieces))
	}

	// Create two pieces
	_, err = handler.CreatePiece(context.Background(), tmpDir, "piece-one", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	_, err = handler.CreatePiece(context.Background(), tmpDir, "piece-two", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	// List pieces
	pieces, err = handler.ListPieces(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("ListPieces failed: %v", err)
	}
	if len(pieces) != 2 {
		t.Errorf("expected 2 pieces, got %d", len(pieces))
	}

	// Verify piece names exist
	var foundOne, foundTwo bool
	for _, p := range pieces {
		if p.Name == "piece-one" {
			foundOne = true
		}
		if p.Name == "piece-two" {
			foundTwo = true
		}
	}
	if !foundOne {
		t.Error("expected to find piece-one")
	}
	if !foundTwo {
		t.Error("expected to find piece-two")
	}

	// Switch to piece (will fallback to path since no tmux in CI)
	result, err := handler.SwitchPiece(context.Background(), "piece-one")
	if err != nil {
		t.Fatalf("SwitchPiece failed: %v", err)
	}

	if result.Piece.Name != "piece-one" {
		t.Errorf("expected piece name 'piece-one', got %q", result.Piece.Name)
	}

	// Method should be "path" since no tmux session
	if result.Method != "path" {
		t.Errorf("expected method 'path', got %q", result.Method)
	}

	// Try to switch to non-existent piece
	_, err = handler.SwitchPiece(context.Background(), "non-existent")
	if err == nil {
		t.Error("expected error for non-existent piece")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestIntegration_CreatePiece_ThenSwitch(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	// Create piece
	info, err := handler.CreatePiece(context.Background(), tmpDir, "auto-switch-test", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	// Immediately switch to the created piece (simulating auto-switch)
	result, err := handler.SwitchPiece(context.Background(), info.Name)
	if err != nil {
		t.Fatalf("SwitchPiece after CreatePiece failed: %v", err)
	}

	// Verify the switch result
	if result.Piece.Name != "auto-switch-test" {
		t.Errorf("expected piece name 'auto-switch-test', got %q", result.Piece.Name)
	}

	// Method should be "path" since no tmux session in CI
	// (tmux session creation is non-fatal, so it may not exist)
	if result.Method != "path" && result.Method != "tmux-attach" {
		t.Errorf("expected method 'path' or 'tmux-attach', got %q", result.Method)
	}
}

func TestIntegration_CreatePieceWithInput_FromIssue(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Create issue file
	issuesDir := filepath.Join(tmpDir, ".monkeypuzzle", "issues")
	if err := os.MkdirAll(issuesDir, 0755); err != nil {
		t.Fatalf("failed to create issues dir: %v", err)
	}

	issueContent := `---
title: Test Feature
status: todo
---

# Test Feature

Description here.
`
	issuePath := filepath.Join(issuesDir, "test-feature.md")
	if err := os.WriteFile(issuePath, []byte(issueContent), 0644); err != nil {
		t.Fatalf("failed to write issue file: %v", err)
	}

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	// Create piece using NewPieceInput with IssuePath
	input := piece.NewPieceInput{
		IssuePath: ".monkeypuzzle/issues/test-feature.md",
	}

	info, err := handler.CreatePieceWithInput(context.Background(), tmpDir, input, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceWithInput failed: %v", err)
	}

	// Verify piece was created with correct name
	if info.Name != "test-feature" {
		t.Errorf("expected piece name 'test-feature', got %q", info.Name)
	}

	// Verify issue status was updated to in-progress
	updatedContent, err := os.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("failed to read updated issue: %v", err)
	}

	if !strings.Contains(string(updatedContent), "status: in-progress") {
		t.Errorf("expected status to be updated to in-progress, got:\n%s", string(updatedContent))
	}
}

func TestIntegration_CreatePieceWithInput_WithName(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	// Create piece using NewPieceInput with Name
	input := piece.NewPieceInput{
		Name: "my-manual-piece",
	}

	info, err := handler.CreatePieceWithInput(context.Background(), tmpDir, input, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePieceWithInput failed: %v", err)
	}

	// Verify piece was created with correct name
	if info.Name != "my-manual-piece" {
		t.Errorf("expected piece name 'my-manual-piece', got %q", info.Name)
	}
}

func TestIntegration_CreatePiece_AutoStartsTmuxIfNoSessionsRunning(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Skip if tmux is not available
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create temp directory for test repo
	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Initialize git repo
	setupGitRepo(t, tmpDir)

	// Create monkeypuzzle config
	setupMonkeypuzzleConfig(t, tmpDir)

	// Change to repo directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create handler
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	// Check if tmux adapter can detect it's installed
	tmux := adapters.NewTmux(adapters.NewOSExec())
	if !tmux.IsInstalled(context.Background()) {
		t.Fatal("tmux.IsInstalled() returned false, but we already verified tmux exists")
	}

	// Create piece - the session should be created
	info, err := handler.CreatePiece(context.Background(), tmpDir, "auto-tmux-test", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	// Verify session was created
	if !tmux.HasSession(context.Background(), info.SessionName) {
		t.Errorf("expected tmux session %q to exist", info.SessionName)
	}

	// Clean up: kill the session
	_ = tmux.KillSession(context.Background(), info.SessionName)
	// Also clean up main session if created
	mainSession := "mp-" + filepath.Base(tmpDir)
	_ = tmux.KillSession(context.Background(), mainSession)
}

func TestIntegration_RepoSpecificPieces_Isolation(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Override data dir for tests
	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	// Create TWO separate repos
	repoA, err := os.MkdirTemp("", "mp-repo-a-*")
	if err != nil {
		t.Fatalf("failed to create repo A: %v", err)
	}
	defer os.RemoveAll(repoA)

	repoB, err := os.MkdirTemp("", "mp-repo-b-*")
	if err != nil {
		t.Fatalf("failed to create repo B: %v", err)
	}
	defer os.RemoveAll(repoB)

	// Initialize both repos
	setupGitRepo(t, repoA)
	setupGitRepo(t, repoB)
	setupMonkeypuzzleConfig(t, repoA)
	setupMonkeypuzzleConfig(t, repoB)

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}

	// Save original working directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	// Create piece in repo A
	if err := os.Chdir(repoA); err != nil {
		t.Fatalf("failed to chdir to repo A: %v", err)
	}
	handlerA := piece.NewHandler(deps)
	_, err = handlerA.CreatePiece(context.Background(), repoA, "piece-in-a", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece in repo A failed: %v", err)
	}

	// Create piece in repo B
	if err := os.Chdir(repoB); err != nil {
		t.Fatalf("failed to chdir to repo B: %v", err)
	}
	handlerB := piece.NewHandler(deps)
	_, err = handlerB.CreatePiece(context.Background(), repoB, "piece-in-b", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece in repo B failed: %v", err)
	}

	// List pieces from repo A - should only see piece-in-a
	if err := os.Chdir(repoA); err != nil {
		t.Fatalf("failed to chdir to repo A: %v", err)
	}
	piecesA, err := handlerA.ListPieces(context.Background(), repoA)
	if err != nil {
		t.Fatalf("ListPieces from repo A failed: %v", err)
	}

	if len(piecesA) != 1 {
		t.Errorf("repo A: expected 1 piece, got %d", len(piecesA))
	}
	if len(piecesA) > 0 && piecesA[0].Name != "piece-in-a" {
		t.Errorf("repo A: expected piece 'piece-in-a', got %q", piecesA[0].Name)
	}

	// List pieces from repo B - should only see piece-in-b
	if err := os.Chdir(repoB); err != nil {
		t.Fatalf("failed to chdir to repo B: %v", err)
	}
	piecesB, err := handlerB.ListPieces(context.Background(), repoB)
	if err != nil {
		t.Fatalf("ListPieces from repo B failed: %v", err)
	}

	if len(piecesB) != 1 {
		t.Errorf("repo B: expected 1 piece, got %d", len(piecesB))
	}
	if len(piecesB) > 0 && piecesB[0].Name != "piece-in-b" {
		t.Errorf("repo B: expected piece 'piece-in-b', got %q", piecesB[0].Name)
	}
}
