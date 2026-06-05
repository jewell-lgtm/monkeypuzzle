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
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
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
		SessionName:  "mp/proj/test",
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
	if !strings.Contains(output, "Session: mp/proj/test") {
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
	info, err := handler.CreatePieceFromIssue(context.Background(), issueRefFromFile(t, deps.FS, relIssuePath), piece.CreatePieceOptions{})
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

	if marker.Issue.Title != "My Awesome Feature" {
		t.Errorf("expected issue name 'My Awesome Feature', got %q", marker.Issue.Title)
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
	info, err := handler.CreatePieceFromIssue(context.Background(), issueRefFromFile(t, deps.FS, relIssuePath), piece.CreatePieceOptions{})
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
	info, err := handler.CreatePieceFromIssue(context.Background(), issueRefFromFile(t, deps.FS, relIssuePath), piece.CreatePieceOptions{})
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
	info, err := handler.CreatePieceFromIssue(context.Background(), issueRefFromFile(t, deps.FS, relIssuePath), piece.CreatePieceOptions{})
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

	// Create piece from issue (status sync goes through the issue-sync subscriber)
	deps := depsWithMarkdownSync(t)
	handler := piece.NewHandler(deps)

	relIssuePath := ".monkeypuzzle/issues/my-feature.md"
	_, err = handler.CreatePieceFromIssue(context.Background(), issueRefFromFile(t, deps.FS, relIssuePath), piece.CreatePieceOptions{})
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
	_, err = handler.CreatePieceFromIssue(context.Background(), issueRefFromFile(t, deps.FS, relIssuePath), piece.CreatePieceOptions{})
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
	_, err = handler.CreatePiece(context.Background(), "piece-one", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	_, err = handler.CreatePiece(context.Background(), "piece-two", piece.CreatePieceOptions{})
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
	info, err := handler.CreatePiece(context.Background(), "auto-switch-test", piece.CreatePieceOptions{})
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

	// Create handler (status sync goes through the issue-sync subscriber)
	deps := depsWithMarkdownSync(t)
	handler := piece.NewHandler(deps)

	// Create piece from an issue ref
	input := piece.NewPieceInput{
		Issue: issueRefFromFile(t, deps.FS, ".monkeypuzzle/issues/test-feature.md"),
	}

	info, err := handler.CreatePieceWithInput(context.Background(), input, piece.CreatePieceOptions{})
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

	info, err := handler.CreatePieceWithInput(context.Background(), input, piece.CreatePieceOptions{})
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

	// Create handler backed by a real tmux multiplexer (session creation happens
	// when the piece is switched to).
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	tmux := adapters.NewTmuxMultiplexer(adapters.NewOSExec())
	if !tmux.IsInstalled(context.Background()) {
		t.Fatal("tmux.IsInstalled() returned false, but we already verified tmux exists")
	}
	handler := piece.NewHandlerWithMultiplexer(deps, adapters.NewTmuxMultiplexer(deps.Exec))

	info, err := handler.CreatePiece(context.Background(), "auto-tmux-test", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}
	// Switching to the piece creates (and attaches) its session.
	if _, err := handler.SwitchPiece(context.Background(), info.Name); err != nil {
		t.Fatalf("SwitchPiece failed: %v", err)
	}

	// Verify session was created
	if !tmux.Exists(context.Background(), info.SessionName) {
		t.Errorf("expected tmux session %q to exist", info.SessionName)
	}

	// Clean up: kill the session
	_ = tmux.Kill(context.Background(), info.SessionName)
	// Also clean up main session if created
	mainSession := "mp/" + filepath.Base(tmpDir)
	_ = tmux.Kill(context.Background(), mainSession)
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
	_, err = handlerA.CreatePiece(context.Background(), "piece-in-a", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece in repo A failed: %v", err)
	}

	// Create piece in repo B
	if err := os.Chdir(repoB); err != nil {
		t.Fatalf("failed to chdir to repo B: %v", err)
	}
	handlerB := piece.NewHandler(deps)
	_, err = handlerB.CreatePiece(context.Background(), "piece-in-b", piece.CreatePieceOptions{})
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

func TestIntegration_AdoptPiece_HappyPath(t *testing.T) {
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

	// Create a feature branch manually (not through mp)
	cmd := exec.Command("git", "checkout", "-b", "my-feature-branch")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b failed: %v\n%s", err, out)
	}

	// Make a commit on the feature branch
	testFile := filepath.Join(tmpDir, "feature.txt")
	if err := os.WriteFile(testFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("failed to write feature file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "feature commit")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Switch back to main — AdoptPiece no longer auto-checks-out main.
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main failed: %v\n%s", err, out)
	}

	// Create handler and adopt the branch as a piece
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	info, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "my-feature-branch"})
	if err != nil {
		t.Fatalf("AdoptPiece failed: %v", err)
	}

	// Verify piece name matches branch name
	if info.Name != "my-feature-branch" {
		t.Errorf("expected piece name 'my-feature-branch', got %q", info.Name)
	}

	// Verify worktree was created
	if _, err := os.Stat(info.WorktreePath); err != nil {
		t.Errorf("worktree not created at %s: %v", info.WorktreePath, err)
	}

	// Verify piece-metadata.json exists with correct parent
	metadataPath := filepath.Join(info.WorktreePath, ".monkeypuzzle", "piece-metadata.json")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatalf("metadata file not found: %v", err)
	}

	var metadata piece.PieceMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatalf("failed to unmarshal metadata: %v", err)
	}

	if metadata.Parent != "main" {
		t.Errorf("expected parent 'main', got %q", metadata.Parent)
	}

	// Verify piece shows up in list
	git := adapters.NewGit(adapters.NewOSExec())
	repoRoot, _ := git.RepoRoot(context.Background(), tmpDir)
	pieces, err := handler.ListPieces(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("ListPieces failed: %v", err)
	}

	var foundPiece bool
	for _, p := range pieces {
		if p.Name == "my-feature-branch" {
			foundPiece = true
			break
		}
	}
	if !foundPiece {
		t.Error("adopted piece not found in ListPieces")
	}
}

// TestIntegration_AdoptPiece_BranchCheckedOutInMainRepo covers the natural flow
// where the user has been working on a branch directly in the main repo and then
// tries to adopt it. Git refuses a second worktree for an already-checked-out
// branch, so AdoptPiece must fail with a clear, actionable message — and must not
// silently move the main repo's checkout or leave a stray worktree behind.
func TestIntegration_AdoptPiece_BranchCheckedOutInMainRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	setupGitRepo(t, tmpDir)
	setupMonkeypuzzleConfig(t, tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create a feature branch and commit, leaving it checked out in the main repo.
	cmd := exec.Command("git", "checkout", "-b", "my-feature-branch")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b failed: %v\n%s", err, out)
	}
	testFile := filepath.Join(tmpDir, "feature.txt")
	if err := os.WriteFile(testFile, []byte("feature content"), 0644); err != nil {
		t.Fatalf("failed to write feature file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "feature commit")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Adopt while my-feature-branch is still HEAD of the main repo.
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	_, err = handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "my-feature-branch"})
	if err == nil {
		t.Fatal("expected AdoptPiece to fail when the branch is checked out in the main repo")
	}

	// The error should name the branch and the main repo, and not be the bare
	// git "exit status 128".
	msg := err.Error()
	if strings.Contains(msg, "exit status 128") {
		t.Errorf("expected a friendly message, got raw git error: %v", err)
	}
	for _, want := range []string{"my-feature-branch", "checked out", tmpDir} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %v", want, err)
		}
	}

	git := adapters.NewGit(adapters.NewOSExec())

	// The main repo must be left untouched on the feature branch (no silent switch).
	mainBranch, err := git.CurrentBranch(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("failed to get main repo branch: %v", err)
	}
	if mainBranch != "my-feature-branch" {
		t.Errorf("expected main repo left on 'my-feature-branch', got %q", mainBranch)
	}

	// No stray worktree should have been created for the failed adopt.
	piecePath := filepath.Join(tmpDir, ".monkeypuzzle", "pieces", "my-feature-branch")
	if _, err := os.Stat(piecePath); !os.IsNotExist(err) {
		t.Errorf("expected no worktree at %s, stat err = %v", piecePath, err)
	}
}

func TestIntegration_AdoptPiece_ErrorOnMainBranch(t *testing.T) {
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

	// Initialize git repo (stays on main branch)
	setupGitRepo(t, tmpDir)
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

	// Try to adopt while on main branch - should fail
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	_, err = handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{})
	if err == nil {
		t.Fatal("expected error when adopting main branch")
	}
	if !strings.Contains(err.Error(), "main") && !strings.Contains(err.Error(), "master") {
		t.Errorf("expected error about main/master branch, got: %v", err)
	}
}

func TestIntegration_AdoptPiece_WithCustomName(t *testing.T) {
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

	// Commit the monkeypuzzle config so working dir is clean
	cmd := exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "add monkeypuzzle config")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	// Create a feature branch
	cmd = exec.Command("git", "checkout", "-b", "ugly-branch-name")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b failed: %v\n%s", err, out)
	}

	// Switch back to main — AdoptPiece no longer auto-checks-out main.
	cmd = exec.Command("git", "checkout", "main")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout main failed: %v\n%s", err, out)
	}

	// Adopt with custom name
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	info, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{
		Branch: "ugly-branch-name",
		Name:   "nice-piece-name",
	})
	if err != nil {
		t.Fatalf("AdoptPiece failed: %v", err)
	}

	// Verify custom name was used
	if info.Name != "nice-piece-name" {
		t.Errorf("expected piece name 'nice-piece-name', got %q", info.Name)
	}
}

func TestIntegration_AdoptPiece_CreatesTmuxSessions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	if _, err := exec.LookPath("tmux"); err != nil {
		t.Skip("tmux not available")
	}
	t.Skip("AdoptPiece no longer creates tmux sessions itself; session orchestration moved to the `mp adopt` CLI command")

	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	setupGitRepo(t, tmpDir)
	setupMonkeypuzzleConfig(t, tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Create feature branch with commit
	cmd := exec.Command("git", "checkout", "-b", "adopt-tmux-test")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout -b failed: %v\n%s", err, out)
	}

	testFile := filepath.Join(tmpDir, "feature.txt")
	if err := os.WriteFile(testFile, []byte("feature"), 0644); err != nil {
		t.Fatalf("failed to write feature file: %v", err)
	}
	cmd = exec.Command("git", "add", ".")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "feature commit")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)
	tmux := adapters.NewTmuxMultiplexer(adapters.NewOSExec())

	info, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{})
	if err != nil {
		t.Fatalf("AdoptPiece failed: %v", err)
	}

	// Verify piece session created
	if !tmux.Exists(context.Background(), info.SessionName) {
		t.Errorf("expected piece tmux session %q to exist", info.SessionName)
	}

	// Verify main repo session created
	mainSession := "mp/" + filepath.Base(tmpDir)
	if !tmux.Exists(context.Background(), mainSession) {
		t.Errorf("expected main repo tmux session %q to exist", mainSession)
	}

	// Cleanup
	_ = tmux.Kill(context.Background(), info.SessionName)
	_ = tmux.Kill(context.Background(), mainSession)
}

func TestIntegration_SwitchPiece_Main(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	tmpDir, err := os.MkdirTemp("", "mp-integration-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Resolve symlinks for macOS /var -> /private/var
	tmpDir, _ = filepath.EvalSymlinks(tmpDir)

	setupGitRepo(t, tmpDir)
	setupMonkeypuzzleConfig(t, tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	// Switch to "main" should work even with no pieces
	result, err := handler.SwitchPiece(context.Background(), "main")
	if err != nil {
		t.Fatalf("SwitchPiece(main) failed: %v", err)
	}

	if result.Piece.Name != "main" {
		t.Errorf("expected piece name 'main', got %q", result.Piece.Name)
	}

	// Compare resolved paths to handle macOS symlinks
	resultPath, _ := filepath.EvalSymlinks(result.Piece.WorktreePath)
	if resultPath != tmpDir {
		t.Errorf("expected worktree path %q, got %q", tmpDir, resultPath)
	}

	// Method should be "path" since no tmux session
	if result.Method != "path" {
		t.Errorf("expected method 'path', got %q", result.Method)
	}

	// "master" should also work
	result, err = handler.SwitchPiece(context.Background(), "master")
	if err != nil {
		t.Fatalf("SwitchPiece(master) failed: %v", err)
	}

	if result.Piece.Name != "master" {
		t.Errorf("expected piece name 'master', got %q", result.Piece.Name)
	}
}

// issueRefFromFile builds a markdown IssueRef from a path, resolving the title
// the same way the markdown provider does (frontmatter -> H1 -> filename).
func issueRefFromFile(t *testing.T, fs core.FS, relPath string) piece.IssueRef {
	t.Helper()
	title, err := piece.ExtractIssueName(relPath, fs)
	if err != nil {
		t.Fatalf("ExtractIssueName(%q): %v", relPath, err)
	}
	return piece.IssueRef{Provider: "markdown", ID: relPath, Title: title}
}

// depsWithMarkdownSync returns Deps wired so that issue-sync events emitted by
// piece operations are applied to the markdown provider (mirrors what the CLI does).
// The current working directory must be the repo root.
func depsWithMarkdownSync(_ *testing.T) core.Deps {
	sig := core.NewIssueSyncSignal()
	deps := core.NewDepsWithSync(adapters.NewOSFS(""), adapters.NewBufferOutput(), adapters.NewOSExec(), nil, nil, sig)
	sig.Sub(func(ev core.IssueSyncEvent) {
		_ = issue.NewHandler(deps, "").SyncStatus(ev.IssueID, ev.NewStatus)
	})
	return deps
}

// recordingMux is a test multiplexer that records calls. It reports as a
// managed (non-noop) multiplexer with a configurable InSession value, so
// tests can exercise the "switch to main before killing" branch without
// needing a real tmux server.
type recordingMux struct {
	inSession bool
	switchTos []switchToCall
	killed    []string
	sessions  map[string]bool
}

type switchToCall struct {
	sessionName string
	workDir     string
}

func newRecordingMux(inSession bool) *recordingMux {
	return &recordingMux{inSession: inSession, sessions: map[string]bool{}}
}

func (r *recordingMux) SwitchTo(_ context.Context, sessionName, workDir string) error {
	r.switchTos = append(r.switchTos, switchToCall{sessionName: sessionName, workDir: workDir})
	r.sessions[sessionName] = true
	return nil
}

func (r *recordingMux) Kill(_ context.Context, sessionName string) error {
	r.killed = append(r.killed, sessionName)
	delete(r.sessions, sessionName)
	return nil
}

func (r *recordingMux) Exists(_ context.Context, sessionName string) bool {
	return r.sessions[sessionName]
}

func (r *recordingMux) InSession() bool                    { return r.inSession }
func (r *recordingMux) IsInstalled(_ context.Context) bool { return true }

func TestIntegration_AbandonPiece_SwitchesToMainSessionWhenInsideWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	tmpDir, err := os.MkdirTemp("", "mp-abandon-switch-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	setupGitRepo(t, tmpDir)
	setupMonkeypuzzleConfig(t, tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	mux := newRecordingMux(true)
	handler := piece.NewHandlerWithMultiplexer(deps, mux)

	info, err := handler.CreatePiece(context.Background(), "switch-on-abandon", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	// Pretend the active client is attached to the piece's session.
	mux.sessions[info.SessionName] = true

	// Move into the worktree before abandoning — this is the case where
	// killing the piece session would otherwise strand the user.
	if err := os.Chdir(info.WorktreePath); err != nil {
		t.Fatalf("failed to chdir into worktree: %v", err)
	}

	_, err = handler.AbandonPiece(context.Background(), "switch-on-abandon", piece.AbandonOptions{Force: true})
	if err != nil {
		t.Fatalf("AbandonPiece failed: %v", err)
	}

	if len(mux.switchTos) != 1 {
		t.Fatalf("expected exactly one SwitchTo call, got %d: %#v", len(mux.switchTos), mux.switchTos)
	}
	got := mux.switchTos[0]
	wantSession := "mp/test-project"
	if got.sessionName != wantSession {
		t.Errorf("SwitchTo session = %q, want %q", got.sessionName, wantSession)
	}
	wantWorkDir, _ := filepath.EvalSymlinks(tmpDir)
	gotWorkDir, _ := filepath.EvalSymlinks(got.workDir)
	if gotWorkDir != wantWorkDir {
		t.Errorf("SwitchTo workDir = %q, want %q", got.workDir, tmpDir)
	}

	// The switch must happen before the piece session is killed, otherwise
	// the user is briefly stranded.
	if len(mux.killed) == 0 || mux.killed[0] != info.SessionName {
		t.Errorf("expected piece session %q to be killed; killed = %v", info.SessionName, mux.killed)
	}
}

func TestIntegration_AbandonPiece_DoesNotSwitchWhenNotInSession(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDataHome, err := os.MkdirTemp("", "mp-data-*")
	if err != nil {
		t.Fatalf("failed to create temp data dir: %v", err)
	}
	t.Cleanup(func() {
		os.RemoveAll(tmpDataHome)
		paths.ResetDataDir()
	})
	paths.SetDataDir(tmpDataHome)

	tmpDir, err := os.MkdirTemp("", "mp-abandon-noswitch-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	setupGitRepo(t, tmpDir)
	setupMonkeypuzzleConfig(t, tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	// Not in a managed session — the helper should not call SwitchTo even
	// though we are inside the worktree being removed.
	mux := newRecordingMux(false)
	handler := piece.NewHandlerWithMultiplexer(deps, mux)

	info, err := handler.CreatePiece(context.Background(), "no-switch", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	if err := os.Chdir(info.WorktreePath); err != nil {
		t.Fatalf("failed to chdir into worktree: %v", err)
	}

	_, err = handler.AbandonPiece(context.Background(), "no-switch", piece.AbandonOptions{Force: true})
	if err != nil {
		t.Fatalf("AbandonPiece failed: %v", err)
	}

	if len(mux.switchTos) != 0 {
		t.Errorf("expected no SwitchTo calls when InSession() is false, got %#v", mux.switchTos)
	}
}
