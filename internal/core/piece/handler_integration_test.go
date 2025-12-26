//go:build integration

package piece_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
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

	err = runner.RunHook(tmpDir, piece.HookOnPieceCreate, ctx)
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

	err = runner.RunHook(tmpDir, piece.HookBeforePieceMerge, piece.HookContext{
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

	err = runner.RunHook(tmpDir, piece.HookAfterPieceUpdate, piece.HookContext{
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

	err = runner.RunHook(tmpDir, piece.HookOnPieceCreate, piece.HookContext{
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

	err = runner.RunHook(tmpDir, piece.HookOnPieceCreate, piece.HookContext{
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

	err = runner.RunHook(tmpDir, piece.HookBeforePieceUpdate, ctx)
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

