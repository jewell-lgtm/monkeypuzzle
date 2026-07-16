//go:build integration

package piece_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestIntegration_HookRunner_RunHookDetached_WritesLog(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "mp-hook-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	hooksDir := filepath.Join(tmpDir, ".monkeypuzzle", "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		t.Fatalf("failed to create hooks dir: %v", err)
	}

	// A hook that takes a moment, then writes a marker. If RunHookDetached
	// blocked, the call below would take ~0.3s; either way the log must appear.
	hookScript := `#!/bin/bash
sleep 0.2
echo "piece=$MP_PIECE_NAME"
`
	hookPath := filepath.Join(hooksDir, piece.HookOnPieceCreate)
	if err := os.WriteFile(hookPath, []byte(hookScript), 0755); err != nil {
		t.Fatalf("failed to write hook script: %v", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	runner := piece.NewHookRunner(deps)

	if err := runner.RunHookDetached(tmpDir, piece.HookOnPieceCreate, piece.HookContext{
		PieceName: "test-piece",
	}); err != nil {
		t.Fatalf("RunHookDetached failed: %v", err)
	}

	logPath := filepath.Join(tmpDir, ".monkeypuzzle", "logs", "on-piece-create-test-piece.log")

	// The hook runs in the background, so poll for its output to land.
	var content []byte
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err = os.ReadFile(logPath)
		if err == nil && strings.Contains(string(content), "piece=test-piece") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(string(content), "piece=test-piece") {
		t.Fatalf("expected hook log %q to contain hook output, got: %q (err: %v)", logPath, content, err)
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

// Helper functions for integration tests

// runGit runs a git command in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

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
  "pr": {"provider": "github", "config": {}}
}`
	configPath := filepath.Join(mpDir, "monkeypuzzle.json")
	if err := os.WriteFile(configPath, []byte(configData), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
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

func TestIntegration_CreatePiece_FromInsideWorktree_AnchorsAtMainRoot(t *testing.T) {
	// Regression: `mp create` run from inside a piece worktree used
	// `git rev-parse --show-toplevel`, which returns the worktree root, so the
	// new piece was nested under the old one (<worktree>/.monkeypuzzle/pieces/...)
	// and registered in the wrong registry — the follow-up switch couldn't find it.

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

	// Create the first piece from the main repo root
	first, err := handler.CreatePiece(context.Background(), "outer-piece", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece (outer) failed: %v", err)
	}

	// Create the second piece from *inside* the first piece's worktree
	if err := os.Chdir(first.WorktreePath); err != nil {
		t.Fatalf("failed to change into worktree: %v", err)
	}
	second, err := handler.CreatePiece(context.Background(), "inner-piece", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece (inner) failed: %v", err)
	}

	// The new piece must live under the main repo's pieces dir, not the worktree's
	wantPath, _ := filepath.EvalSymlinks(filepath.Join(tmpDir, ".monkeypuzzle", "pieces", "inner-piece"))
	gotPath, _ := filepath.EvalSymlinks(second.WorktreePath)
	if gotPath != wantPath {
		t.Errorf("expected worktree at %q, got %q", wantPath, gotPath)
	}
	nested := filepath.Join(first.WorktreePath, ".monkeypuzzle", "pieces", "inner-piece")
	if _, err := os.Stat(nested); err == nil {
		t.Errorf("piece was nested inside the outer worktree at %q", nested)
	}

	// And the follow-up switch (still inside the outer worktree) must find it
	if _, err := handler.SwitchPiece(context.Background(), second.Name); err != nil {
		t.Fatalf("SwitchPiece after CreatePiece from worktree failed: %v", err)
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

// TestIntegration_AdoptPiece_DirtyMainRepo verifies that adopting a branch other
// than the one checked out in the main repo succeeds even when the main working
// tree has uncommitted changes. Adopt creates a separate worktree via
// `git worktree add`, so the main checkout's dirty files are untouched and safe.
func TestIntegration_AdoptPiece_DirtyMainRepo(t *testing.T) {
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

	// Create a feature branch with a commit, then return to main.
	runGit(t, tmpDir, "checkout", "-b", "my-feature-branch")
	if err := os.WriteFile(filepath.Join(tmpDir, "feature.txt"), []byte("feature content"), 0644); err != nil {
		t.Fatalf("failed to write feature file: %v", err)
	}
	runGit(t, tmpDir, "add", ".")
	runGit(t, tmpDir, "commit", "-m", "feature commit")
	runGit(t, tmpDir, "checkout", "main")

	// Dirty the main working tree — adopt must not require this to be clean.
	dirtyFile := filepath.Join(tmpDir, "scratch.txt")
	if err := os.WriteFile(dirtyFile, []byte("work in progress"), 0644); err != nil {
		t.Fatalf("failed to write dirty file: %v", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	info, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "my-feature-branch"})
	if err != nil {
		t.Fatalf("AdoptPiece failed with dirty main repo: %v", err)
	}
	if _, err := os.Stat(info.WorktreePath); err != nil {
		t.Errorf("worktree not created at %s: %v", info.WorktreePath, err)
	}

	// The main checkout's uncommitted change must be preserved, untouched.
	if data, err := os.ReadFile(dirtyFile); err != nil {
		t.Errorf("dirty main file was removed by adopt: %v", err)
	} else if string(data) != "work in progress" {
		t.Errorf("dirty main file content changed by adopt: got %q", string(data))
	}
}

// TestIntegration_AdoptPiece_BranchCheckedOutInMainRepo covers the natural flow
// where the user has been working on a branch directly in the main repo and then
// adopts it. AdoptPiece frees the branch by resetting the main worktree back to
// its main branch, carrying any uncommitted work into the new piece worktree.
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

	// Leave uncommitted work-in-progress on the branch in the main worktree; it
	// should travel into the adopted piece, not block the adopt.
	wipFile := filepath.Join(tmpDir, "wip.txt")
	if err := os.WriteFile(wipFile, []byte("in flight"), 0644); err != nil {
		t.Fatalf("failed to write wip file: %v", err)
	}

	// Adopt while my-feature-branch is still HEAD of the main repo.
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)

	info, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "my-feature-branch"})
	if err != nil {
		t.Fatalf("AdoptPiece failed for a branch checked out in the main repo: %v", err)
	}

	git := adapters.NewGit(adapters.NewOSExec())

	// The main worktree must have been reset back to main, freeing the branch.
	mainBranch, err := git.CurrentBranch(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("failed to get main repo branch: %v", err)
	}
	if mainBranch != "main" {
		t.Errorf("expected main worktree reset to 'main', got %q", mainBranch)
	}

	// The branch must now live in its own piece worktree.
	if _, err := os.Stat(info.WorktreePath); err != nil {
		t.Errorf("worktree not created at %s: %v", info.WorktreePath, err)
	}

	// The WIP must have followed the branch into the piece, and be gone from main.
	if _, err := os.Stat(filepath.Join(tmpDir, "wip.txt")); !os.IsNotExist(err) {
		t.Errorf("WIP file should no longer be in the main worktree, stat err = %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(info.WorktreePath, "wip.txt")); err != nil {
		t.Errorf("WIP file did not travel into the piece worktree: %v", err)
	} else if string(data) != "in flight" {
		t.Errorf("WIP file content changed: got %q", string(data))
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

// recordingMux is a test multiplexer that records calls. It reports as a
// managed (non-noop) multiplexer with a configurable InSession value, so
// tests can exercise the "switch to main before killing" branch without
// needing a real tmux server.
type recordingMux struct {
	inSession bool
	switchTos []switchToCall
	killed    []string
	sessions  map[string]bool
	// onKill, if set, fires at the start of Kill — lets a test observe on-disk
	// state at the moment the session would be torn down (in production the kill
	// terminates the calling process, so anything ordered after it never runs).
	onKill func(sessionName string)
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
	if r.onKill != nil {
		r.onKill(sessionName)
	}
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

func TestIntegration_AbandonPiece_DoesNotSwitchOrKillWhenWorktreeRemovalFails(t *testing.T) {
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

	tmpDir, err := os.MkdirTemp("", "mp-abandon-fail-visible-*")
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

	info, err := handler.CreatePiece(context.Background(), "dirty-abandon", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}
	mux.sessions[info.SessionName] = true

	if err := os.WriteFile(filepath.Join(info.WorktreePath, "untracked.txt"), []byte("dirty\n"), 0644); err != nil {
		t.Fatalf("failed to dirty worktree: %v", err)
	}
	if err := os.Chdir(info.WorktreePath); err != nil {
		t.Fatalf("failed to chdir into worktree: %v", err)
	}

	_, err = handler.AbandonPiece(context.Background(), "dirty-abandon", piece.AbandonOptions{})
	if err == nil {
		t.Fatal("expected AbandonPiece to fail for a dirty worktree without force")
	}
	if !strings.Contains(err.Error(), "failed to remove worktree") {
		t.Fatalf("expected worktree removal error, got %v", err)
	}
	if len(mux.switchTos) != 0 {
		t.Fatalf("expected no SwitchTo calls after failed worktree removal, got %#v", mux.switchTos)
	}
	if len(mux.killed) != 0 {
		t.Fatalf("expected no Kill calls after failed worktree removal, got %#v", mux.killed)
	}
	if _, err := os.Stat(info.WorktreePath); err != nil {
		t.Fatalf("expected failed abandon to leave worktree in place; stat err = %v", err)
	}
}

// TestIntegration_AbandonPiece_RemovesWorktreeBeforeKillingSession is a
// regression test for the bug where abandoning a piece from inside its own tmux
// session left the worktree on disk. AbandonPiece used to kill the session
// before removing the worktree; in production the kill terminates the abandon
// process (it runs in one of the session's panes), so the worktree removal
// never happened and the piece kept appearing in the picker (ListPieces reads
// directory entries, so a leftover directory == a listed piece). The fix
// removes the worktree first and kills the session last. We assert the
// invariant directly: by the time the session is killed, the worktree must
// already be gone.
func TestIntegration_AbandonPiece_RemovesWorktreeBeforeKillingSession(t *testing.T) {
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

	tmpDir, err := os.MkdirTemp("", "mp-abandon-order-*")
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

	info, err := handler.CreatePiece(context.Background(), "abandon-order", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	// Pretend the active client is attached to the piece's session, and observe
	// whether the worktree still exists at the moment the session is killed.
	mux.sessions[info.SessionName] = true
	var killObserved bool
	var worktreeExistedAtKill bool
	mux.onKill = func(string) {
		killObserved = true
		if _, statErr := os.Stat(info.WorktreePath); statErr == nil {
			worktreeExistedAtKill = true
		}
	}

	// Abandon from inside the worktree — the case that used to strand the piece.
	if err := os.Chdir(info.WorktreePath); err != nil {
		t.Fatalf("failed to chdir into worktree: %v", err)
	}

	if _, err := handler.AbandonPiece(context.Background(), "abandon-order", piece.AbandonOptions{Force: true}); err != nil {
		t.Fatalf("AbandonPiece failed: %v", err)
	}

	if !killObserved {
		t.Fatal("expected the piece session to be killed")
	}
	if worktreeExistedAtKill {
		t.Error("worktree still existed when the session was killed: removal must happen before the kill, otherwise abandoning from inside the session (where the kill terminates this process) never removes the worktree")
	}

	// End state: the worktree directory is actually gone, so the picker won't
	// list it.
	if _, err := os.Stat(info.WorktreePath); !os.IsNotExist(err) {
		t.Errorf("expected worktree %s to be removed; stat err = %v", info.WorktreePath, err)
	}
}

// TestIntegration_FlattenPieces_RemovesAllWorktreesBeforeKillingSessions guards
// the same ordering hazard as the abandon regression above, but for flatten's
// loop. Flatten used to kill each piece's session inline before removing its
// worktree; run from inside one of the pieces, that kill would terminate the
// flatten process mid-loop and strand the remaining pieces. The fix removes all
// worktrees first and kills sessions afterwards. We assert no worktree still
// exists at the moment any session is killed.
func TestIntegration_FlattenPieces_RemovesAllWorktreesBeforeKillingSessions(t *testing.T) {
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

	tmpDir, err := os.MkdirTemp("", "mp-flatten-order-*")
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

	pieceA, err := handler.CreatePiece(context.Background(), "flatten-a", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece(flatten-a) failed: %v", err)
	}
	pieceB, err := handler.CreatePiece(context.Background(), "flatten-b", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece(flatten-b) failed: %v", err)
	}

	// Both pieces have live sessions; record whether any worktree still exists
	// at the moment a session is killed.
	mux.sessions[pieceA.SessionName] = true
	mux.sessions[pieceB.SessionName] = true
	worktrees := []string{pieceA.WorktreePath, pieceB.WorktreePath}
	var killObserved bool
	var worktreeExistedAtKill bool
	mux.onKill = func(string) {
		killObserved = true
		for _, wt := range worktrees {
			if _, statErr := os.Stat(wt); statErr == nil {
				worktreeExistedAtKill = true
			}
		}
	}

	// Flatten from inside one of the pieces — the case that used to abort the
	// loop when that piece's session was killed.
	if err := os.Chdir(pieceA.WorktreePath); err != nil {
		t.Fatalf("failed to chdir into worktree: %v", err)
	}

	res, err := handler.FlattenPieces(context.Background(), "", piece.FlattenOptions{Force: true})
	if err != nil {
		t.Fatalf("FlattenPieces failed: %v", err)
	}

	if !killObserved {
		t.Fatal("expected piece sessions to be killed")
	}
	if worktreeExistedAtKill {
		t.Error("a worktree still existed when a session was killed: flatten must remove every worktree before killing any session, or running it from inside a piece strands the rest")
	}
	if res.Count != 2 {
		t.Errorf("expected 2 pieces flattened, got %d (removed=%d failed=%d)", res.Count, len(res.Removed), len(res.Failed))
	}
	for _, wt := range worktrees {
		if _, err := os.Stat(wt); !os.IsNotExist(err) {
			t.Errorf("expected worktree %s to be removed; stat err = %v", wt, err)
		}
	}
}

// TestIntegration_MergePiece_RecordsMergedMarker_MultiCommit proves the fix
// end-to-end with real git: a MULTI-commit piece squash-merged by MergePiece
// must (a) persist the durable Merged marker in piece metadata, and (b) be
// reported merged via method "recorded" by IsBranchMerged. git-cherry /
// branch --merged cannot detect such a squash, so the recorded marker is the
// only signal cleanup/done can rely on.
func TestIntegration_MergePiece_RecordsMergedMarker_MultiCommit(t *testing.T) {
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

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	handler := piece.NewHandler(deps)
	ctx := context.Background()

	info, err := handler.CreatePiece(ctx, "multi-commit-piece", piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}
	worktree := info.WorktreePath

	// Make THREE distinct commits in the piece worktree so the squash collapses
	// them into one combined commit whose patch-id matches none of them.
	for i := 1; i <= 3; i++ {
		fname := filepath.Join(worktree, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(fname, []byte(fmt.Sprintf("content %d", i)), 0644); err != nil {
			t.Fatalf("write file%d: %v", i, err)
		}
		runGit(t, worktree, "add", ".")
		runGit(t, worktree, "commit", "-m", fmt.Sprintf("commit %d", i))
	}

	// Merge the piece (squash) from inside its worktree.
	res, err := handler.MergePiece(ctx, worktree, piece.MergeInput{MainBranch: "main"})
	if err != nil {
		t.Fatalf("MergePiece failed: %v", err)
	}
	if res.Status != "merged" {
		t.Fatalf("expected status merged, got %q", res.Status)
	}

	// (a) The durable marker is persisted in piece metadata.
	meta, err := piece.ReadPieceMetadata(worktree, deps.FS)
	if err != nil {
		t.Fatalf("ReadPieceMetadata: %v", err)
	}
	if !meta.Merged {
		t.Fatalf("expected piece metadata Merged=true after MergePiece, got false")
	}

	// (b) IsBranchMerged reports merged via the recorded marker.
	status, err := handler.IsBranchMerged(ctx, worktree, res.PieceBranch, "main")
	if err != nil {
		t.Fatalf("IsBranchMerged: %v", err)
	}
	if !status.IsMerged {
		t.Fatalf("expected IsMerged=true, got false (method %q)", status.Method)
	}
	if status.Method != "recorded" {
		t.Errorf("expected method %q, got %q", "recorded", status.Method)
	}
}

// setupAdoptTestRepo creates a git repo with mp config, isolates the data dir,
// and chdirs into the repo for the duration of the test. Returns the repo path
// and a ready piece handler.
func setupAdoptTestRepo(t *testing.T) (string, *piece.Handler) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	paths.SetDataDir(t.TempDir())
	t.Cleanup(paths.ResetDataDir)

	tmpDir := t.TempDir()
	setupGitRepo(t, tmpDir)
	setupMonkeypuzzleConfig(t, tmpDir)

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}
	return tmpDir, piece.NewHandler(deps)
}

// TestIntegration_AdoptPiece_BranchInForeignWorktree covers adopting a branch
// that is checked out in a worktree mp doesn't manage — the shape an agent's
// worktree isolation (e.g. Claude Code) leaves behind. Adopt relocates the
// worktree into the pieces dir, carrying uncommitted changes along.
func TestIntegration_AdoptPiece_BranchInForeignWorktree(t *testing.T) {
	tmpDir, handler := setupAdoptTestRepo(t)

	foreignPath := filepath.Join(t.TempDir(), "agent-worktree")
	runGit(t, tmpDir, "worktree", "add", "-b", "agent-spike", foreignPath)

	// Uncommitted work in the foreign worktree must survive the adoption.
	wipFile := filepath.Join(foreignPath, "wip.txt")
	if err := os.WriteFile(wipFile, []byte("uncommitted"), 0644); err != nil {
		t.Fatalf("failed to write wip file: %v", err)
	}

	info, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "agent-spike"})
	if err != nil {
		t.Fatalf("AdoptPiece failed for branch in foreign worktree: %v", err)
	}

	wantPath := filepath.Join(tmpDir, ".monkeypuzzle", "pieces", "agent-spike")
	if info.WorktreePath != wantPath {
		t.Errorf("expected worktree at %s, got %s", wantPath, info.WorktreePath)
	}
	if data, err := os.ReadFile(filepath.Join(info.WorktreePath, "wip.txt")); err != nil {
		t.Errorf("uncommitted change did not travel with the worktree: %v", err)
	} else if string(data) != "uncommitted" {
		t.Errorf("wip file content changed: %q", data)
	}
	if _, err := os.Stat(foreignPath); !os.IsNotExist(err) {
		t.Errorf("foreign worktree still present at %s (err=%v)", foreignPath, err)
	}
}

// TestIntegration_AdoptPiece_PrunableWorktree covers adopting a branch whose
// registered worktree directory has been deleted out from under git (e.g. a
// cleaned-up agent sandbox). Adopt prunes the stale record and proceeds.
func TestIntegration_AdoptPiece_PrunableWorktree(t *testing.T) {
	tmpDir, handler := setupAdoptTestRepo(t)

	stalePath := filepath.Join(t.TempDir(), "stale-worktree")
	runGit(t, tmpDir, "worktree", "add", "-b", "stale-branch", stalePath)
	if err := os.RemoveAll(stalePath); err != nil {
		t.Fatalf("failed to delete worktree dir: %v", err)
	}

	info, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "stale-branch"})
	if err != nil {
		t.Fatalf("AdoptPiece failed for branch with prunable worktree: %v", err)
	}
	if _, err := os.Stat(info.WorktreePath); err != nil {
		t.Errorf("worktree not created at %s: %v", info.WorktreePath, err)
	}
}

// TestIntegration_AdoptPiece_BranchAlreadyAPiece verifies that adopting a
// branch checked out in an existing piece worktree fails with a pointer to
// `mp switch` rather than relocating the piece.
func TestIntegration_AdoptPiece_BranchAlreadyAPiece(t *testing.T) {
	_, handler := setupAdoptTestRepo(t)

	if _, err := handler.CreatePiece(context.Background(), "piecey", piece.CreatePieceOptions{}); err != nil {
		t.Fatalf("CreatePiece failed: %v", err)
	}

	_, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "piecey", Name: "other"})
	if err == nil {
		t.Fatal("expected AdoptPiece to fail for a branch that is already a piece")
	}
	if !strings.Contains(err.Error(), "already a piece") {
		t.Errorf("expected 'already a piece' error, got: %v", err)
	}
}

// TestIntegration_AdoptPiece_LockedWorktree verifies that a branch held by a
// locked worktree is rejected with an actionable unlock hint.
func TestIntegration_AdoptPiece_LockedWorktree(t *testing.T) {
	tmpDir, handler := setupAdoptTestRepo(t)

	lockedPath := filepath.Join(t.TempDir(), "locked-worktree")
	runGit(t, tmpDir, "worktree", "add", "-b", "locked-branch", lockedPath)
	runGit(t, tmpDir, "worktree", "lock", lockedPath)
	t.Cleanup(func() { runGit(t, tmpDir, "worktree", "unlock", lockedPath) })

	_, err := handler.AdoptPiece(context.Background(), piece.AdoptPieceInput{Branch: "locked-branch"})
	if err == nil {
		t.Fatal("expected AdoptPiece to fail for a branch in a locked worktree")
	}
	if !strings.Contains(err.Error(), "worktree unlock") {
		t.Errorf("expected unlock hint in error, got: %v", err)
	}
}
