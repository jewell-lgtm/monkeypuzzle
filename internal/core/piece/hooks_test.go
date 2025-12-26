package piece_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestHookRunner_RunHook_HookNotExists(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// No hook file exists, should return nil (no error)
	err := runner.RunHook("/repo", piece.HookOnPieceCreate, piece.HookContext{
		PieceName:    "test-piece",
		WorktreePath: "/pieces/test-piece",
		RepoRoot:     "/repo",
	})

	if err != nil {
		t.Errorf("expected no error when hook doesn't exist, got: %v", err)
	}

	// Verify no exec calls were made
	calls := mockExec.GetCalls()
	if len(calls) > 0 {
		t.Errorf("expected no exec calls, got: %v", calls)
	}
}

func TestHookRunner_RunHook_HooksDirNotExists(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// .monkeypuzzle/hooks directory doesn't exist
	err := runner.RunHook("/repo", piece.HookBeforePieceMerge, piece.HookContext{
		PieceName:  "test-piece",
		MainBranch: "main",
	})

	if err != nil {
		t.Errorf("expected no error when hooks directory doesn't exist, got: %v", err)
	}
}

func TestHookRunner_RunHook_HookNotExecutable(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// Create hook file without executable permission (0644)
	hooksDir := ".monkeypuzzle/hooks"
	hookPath := filepath.Join(hooksDir, piece.HookOnPieceCreate)
	_ = fs.MkdirAll(hooksDir, 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\necho test"), 0644) // Not executable

	err := runner.RunHook("/", piece.HookOnPieceCreate, piece.HookContext{
		PieceName: "test-piece",
	})

	if err != nil {
		t.Errorf("expected no error when hook is not executable, got: %v", err)
	}

	// Should have a warning message
	if !out.HasWarning() {
		t.Error("expected warning about non-executable hook")
	}
}

func TestHookRunner_RunHook_HookExecutionFails(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// Create executable hook file
	hooksDir := ".monkeypuzzle/hooks"
	hookPath := filepath.Join(hooksDir, piece.HookOnPieceCreate)
	_ = fs.MkdirAll(hooksDir, 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\necho test"), 0755) // Executable

	// Mock exec to return error
	fullHookPath := filepath.Join("/", hooksDir, piece.HookOnPieceCreate)
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte("hook error output"), errors.New("exit status 1"))

	err := runner.RunHook("/", piece.HookOnPieceCreate, piece.HookContext{
		PieceName: "test-piece",
	})

	if err == nil {
		t.Fatal("expected error when hook fails")
	}

	if !strings.Contains(err.Error(), "hook") && !strings.Contains(err.Error(), "failed") {
		t.Errorf("expected error about hook failure, got: %v", err)
	}
}

func TestHookRunner_RunHook_Success(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// Create executable hook file
	hooksDir := ".monkeypuzzle/hooks"
	hookPath := filepath.Join(hooksDir, piece.HookOnPieceCreate)
	_ = fs.MkdirAll(hooksDir, 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\necho test"), 0755) // Executable

	// Mock exec to return success
	fullHookPath := filepath.Join("/", hooksDir, piece.HookOnPieceCreate)
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte("hook output\n"), nil)

	err := runner.RunHook("/", piece.HookOnPieceCreate, piece.HookContext{
		PieceName:    "test-piece",
		WorktreePath: "/pieces/test-piece",
		RepoRoot:     "/repo",
		SessionName:  "mp-piece-test-piece",
	})

	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify bash was called
	if !mockExec.WasCalled("bash", fullHookPath) {
		t.Error("expected bash to be called with hook path")
	}
}

func TestHookRunner_RunHook_PassesEnvironmentVariables(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// Create executable hook file at repo/.monkeypuzzle/hooks/
	// MemoryFS stores paths without leading slash, so "repo/.monkeypuzzle/hooks/..."
	hooksDir := "repo/.monkeypuzzle/hooks"
	hookPath := filepath.Join(hooksDir, piece.HookBeforePieceMerge)
	_ = fs.MkdirAll(hooksDir, 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\necho $MP_PIECE_NAME"), 0755)

	// Mock exec to return success
	fullHookPath := filepath.Join("/repo", ".monkeypuzzle/hooks", piece.HookBeforePieceMerge)
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte(""), nil)

	ctx := piece.HookContext{
		PieceName:    "my-piece",
		WorktreePath: "/pieces/my-piece",
		RepoRoot:     "/repo",
		MainBranch:   "main",
		SessionName:  "mp-piece-my-piece",
	}

	err := runner.RunHook("/repo", piece.HookBeforePieceMerge, ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify the call was made with environment variables
	calls := mockExec.GetCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one exec call")
	}

	lastCall := calls[len(calls)-1]
	if lastCall.Env == nil {
		t.Fatal("expected environment variables to be set")
	}

	// Check for required env vars
	envMap := make(map[string]string)
	for _, e := range lastCall.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	if envMap["MP_PIECE_NAME"] != "my-piece" {
		t.Errorf("expected MP_PIECE_NAME=my-piece, got: %s", envMap["MP_PIECE_NAME"])
	}
	if envMap["MP_WORKTREE_PATH"] != "/pieces/my-piece" {
		t.Errorf("expected MP_WORKTREE_PATH=/pieces/my-piece, got: %s", envMap["MP_WORKTREE_PATH"])
	}
	if envMap["MP_REPO_ROOT"] != "/repo" {
		t.Errorf("expected MP_REPO_ROOT=/repo, got: %s", envMap["MP_REPO_ROOT"])
	}
	if envMap["MP_MAIN_BRANCH"] != "main" {
		t.Errorf("expected MP_MAIN_BRANCH=main, got: %s", envMap["MP_MAIN_BRANCH"])
	}
	if envMap["MP_SESSION_NAME"] != "mp-piece-my-piece" {
		t.Errorf("expected MP_SESSION_NAME=mp-piece-my-piece, got: %s", envMap["MP_SESSION_NAME"])
	}
}

func TestHookRunner_AllHookTypes(t *testing.T) {
	// Verify all hook type constants are valid
	hooks := []string{
		piece.HookOnPieceCreate,
		piece.HookBeforePieceMerge,
		piece.HookAfterPieceMerge,
		piece.HookBeforePieceUpdate,
		piece.HookAfterPieceUpdate,
	}

	for _, h := range hooks {
		if !strings.HasSuffix(h, ".sh") {
			t.Errorf("hook %s should end with .sh", h)
		}
	}
}

func TestHooksDir_Constant(t *testing.T) {
	// Verify hooks directory constant
	if piece.HooksDir != ".monkeypuzzle/hooks" {
		t.Errorf("expected HooksDir to be .monkeypuzzle/hooks, got: %s", piece.HooksDir)
	}
}

// Helper to check if a call includes certain environment variables
func envContains(env []string, key, value string) bool {
	target := key + "=" + value
	for _, e := range env {
		if e == target {
			return true
		}
	}
	return false
}

func TestHookRunner_RunHook_EmptyContext(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// Create executable hook file
	hooksDir := ".monkeypuzzle/hooks"
	hookPath := filepath.Join(hooksDir, piece.HookOnPieceCreate)
	_ = fs.MkdirAll(hooksDir, 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\necho test"), 0755)

	fullHookPath := filepath.Join("/", hooksDir, piece.HookOnPieceCreate)
	mockExec.AddResponse("bash", []string{fullHookPath}, nil, nil)

	// Run with empty context - should still work
	err := runner.RunHook("/", piece.HookOnPieceCreate, piece.HookContext{})

	if err != nil {
		t.Errorf("expected no error with empty context, got: %v", err)
	}
}

func TestHookRunner_BuildEnv_FiltersExistingMPVariables(t *testing.T) {
	// Set some MP_* variables in the environment before running
	t.Setenv("MP_PIECE_NAME", "old-piece")
	t.Setenv("MP_WORKTREE_PATH", "/old/path")
	t.Setenv("MP_CUSTOM_VAR", "should-be-removed")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	runner := piece.NewHookRunner(deps)

	// Create executable hook file
	hooksDir := "repo/.monkeypuzzle/hooks"
	hookPath := filepath.Join(hooksDir, piece.HookOnPieceCreate)
	_ = fs.MkdirAll(hooksDir, 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\necho test"), 0755)

	// Mock exec to return success
	fullHookPath := filepath.Join("/repo", ".monkeypuzzle/hooks", piece.HookOnPieceCreate)
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte(""), nil)

	ctx := piece.HookContext{
		PieceName:    "new-piece",
		WorktreePath: "/new/path",
		RepoRoot:     "/repo",
	}

	err := runner.RunHook("/repo", piece.HookOnPieceCreate, ctx)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Check the environment variables passed to the exec call
	calls := mockExec.GetCalls()
	if len(calls) == 0 {
		t.Fatal("expected at least one exec call")
	}

	lastCall := calls[len(calls)-1]
	if lastCall.Env == nil {
		t.Fatal("expected environment variables to be set")
	}

	// Count MP_* variables and check values
	mpVars := make(map[string]string)
	mpCount := make(map[string]int)
	for _, e := range lastCall.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], "MP_") {
			mpVars[parts[0]] = parts[1]
			mpCount[parts[0]]++
		}
	}

	// Verify no duplicates
	for key, count := range mpCount {
		if count > 1 {
			t.Errorf("found duplicate environment variable %s (count: %d)", key, count)
		}
	}

	// Verify our values take precedence over old ones
	if mpVars["MP_PIECE_NAME"] != "new-piece" {
		t.Errorf("expected MP_PIECE_NAME=new-piece, got: %s", mpVars["MP_PIECE_NAME"])
	}
	if mpVars["MP_WORKTREE_PATH"] != "/new/path" {
		t.Errorf("expected MP_WORKTREE_PATH=/new/path, got: %s", mpVars["MP_WORKTREE_PATH"])
	}

	// Verify old custom MP_* variable was removed
	if _, exists := mpVars["MP_CUSTOM_VAR"]; exists {
		t.Error("MP_CUSTOM_VAR should have been filtered out")
	}
}

// Suppresses the unused variable warning for envContains and os
var _ = envContains
var _ = os.ErrNotExist
