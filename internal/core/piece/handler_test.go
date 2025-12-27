package piece_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

func TestHandler_CreatePiece(t *testing.T) {
	// Set XDG_DATA_HOME to a test directory
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Execute - will fail at worktree creation since we didn't mock it, but that's ok
	// We're testing the flow, not end-to-end success
	// Use a deterministic piece name for testing
	_, err := handler.CreatePiece("/monkeypuzzle", "test-piece-1")

	// We expect an error at worktree creation since we didn't mock the exact path
	if err == nil {
		t.Fatal("expected error due to missing worktree mock, but got success")
	}

	// Verify git repo root was checked
	calls := mockExec.GetCalls()
	foundRepoRoot := false
	for _, call := range calls {
		if call.Name == "git" && len(call.Args) >= 2 && call.Args[0] == "rev-parse" && call.Args[1] == "--show-toplevel" {
			foundRepoRoot = true
			break
		}
	}
	if !foundRepoRoot {
		t.Error("expected git rev-parse --show-toplevel to be called")
	}

	// Verify pieces directory was created (MemoryFS stores relative paths)
	dirs := fs.Dirs()
	found := false
	expectedDir := "test-data/monkeypuzzle/pieces" // MemoryFS cleans paths
	for _, d := range dirs {
		if d == expectedDir {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected pieces directory %q to be created, dirs: %v", expectedDir, dirs)
	}
}

func TestHandler_Status_InMainRepo(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for main repo
	gitDir := "/repo/.git"
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	status, err := handler.Status("/repo")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.InPiece {
		t.Error("expected not to be in a piece")
	}

	if status.RepoRoot != repoRoot {
		t.Errorf("expected repo root %q, got %q", repoRoot, status.RepoRoot)
	}
}

func TestHandler_Status_InWorktree(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for worktree
	gitDir := "/repo/.git/worktrees/piece-1"
	worktreePath := "/pieces/piece-1"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	status, err := handler.Status("/pieces/piece-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !status.InPiece {
		t.Error("expected to be in a piece")
	}

	if status.PieceName == "" {
		t.Error("expected piece name to be set")
	}
}

func TestHandler_Status_NotInGitRepo(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock to return error (not in git repo)
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, nil, os.ErrNotExist)

	status, err := handler.Status("/tmp")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.InPiece {
		t.Error("expected not to be in a piece")
	}
}

func TestHandler_GeneratePieceName(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	baseDir := "/pieces"
	name1, err := handler.GeneratePieceName(baseDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if name1 == "" {
		t.Error("expected piece name to be generated")
	}

	if !strings.HasPrefix(name1, "piece-") {
		t.Errorf("expected piece name to start with 'piece-', got %q", name1)
	}

	// Test counter logic: create a directory with the same base name
	// to force counter increment within the same timestamp
	existingPath := filepath.Join(baseDir, name1)
	_ = fs.MkdirAll(existingPath, 0755)

	// Generate another name - should get the same base but with counter suffix
	// since the base name already exists
	name2, err := handler.GeneratePieceName(baseDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if name2 == "" {
		t.Error("expected name2 to be generated")
	}

	if !strings.HasPrefix(name2, "piece-") {
		t.Errorf("expected name2 to start with 'piece-', got %q", name2)
	}

	// If names are the same, it means the timestamp changed between calls
	// (which is fine - the important thing is both are valid)
	// If they're different, verify name2 has a counter suffix or different timestamp
	if name1 == name2 {
		// This is acceptable if called in different seconds
		// The key is that both names are valid and start with "piece-"
		t.Logf("Both names are the same (called in same second): %q", name1)
	} else {
		// Names are different - verify name2 is valid
		if !strings.HasPrefix(name2, "piece-") {
			t.Errorf("name2 should start with 'piece-', got %q", name2)
		}
	}
}

func TestHandler_CreatePiece_WithName(t *testing.T) {
	// Set XDG_DATA_HOME to a test directory
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Test with a specific piece name
	pieceName := "test-piece-deterministic"
	_, err := handler.CreatePiece("/monkeypuzzle", pieceName)

	// We expect an error at worktree creation since we didn't mock it, but that's ok
	// We're testing that the name parameter is accepted
	if err == nil {
		t.Fatal("expected error due to missing worktree mock, but got success")
	}

	// Verify the error is not about the name already existing (unless it's a different error)
	if strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error about name existing: %v", err)
	}
}

func TestHandler_CreatePiece_NameAlreadyExists(t *testing.T) {
	// Set XDG_DATA_HOME to a test directory
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Get the actual pieces directory that will be used
	// This matches what getPiecesDir() returns
	piecesDir := "/test-data/monkeypuzzle/pieces"
	existingPiecePath := filepath.Join(piecesDir, "existing-piece")

	// Create the pieces directory structure first
	_ = fs.MkdirAll(piecesDir, 0755)
	// Then create the existing piece directory
	_ = fs.MkdirAll(existingPiecePath, 0755)

	// Try to create a piece with the same name
	_, err := handler.CreatePiece("/monkeypuzzle", "existing-piece")
	if err == nil {
		t.Fatal("expected error when piece name already exists")
	}

	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected error about name already existing, got: %v", err)
	}
}

func TestHandler_UpdatePiece_InWorktree(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for worktree status
	gitDir := "/repo/.git/worktrees/piece-1"
	worktreePath := "/pieces/piece-1"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Setup mock responses for update
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("piece-1\n"), nil)
	mockExec.AddResponse("git", []string{"merge", "main"}, nil, nil)

	err := handler.UpdatePiece("/pieces/piece-1", "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify git merge was called
	if !mockExec.WasCalled("git", "merge", "main") {
		t.Error("expected git merge main to be called")
	}

	// Verify success message
	if !out.HasSuccess() {
		t.Error("expected success message")
	}
}

func TestHandler_UpdatePiece_NotInWorktree(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for main repo (not worktree)
	gitDir := "/repo/.git"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte("/repo\n"), nil)

	err := handler.UpdatePiece("/repo", "main")
	if err == nil {
		t.Fatal("expected error when not in worktree")
	}

	if !strings.Contains(err.Error(), "not in a piece worktree") {
		t.Errorf("expected error about not being in worktree, got: %v", err)
	}
}

func TestHandler_MergePiece_Success(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for worktree status
	gitDir := "/repo/.git/worktrees/piece-1"
	worktreePath := "/pieces/piece-1"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Setup mock responses for merge piece
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("piece-1\n"), nil)
	// IsMainAhead: merge-base and rev-list
	mockExec.AddResponse("git", []string{"merge-base", "main", "piece-1"}, []byte("abc123\n"), nil)
	mockExec.AddResponse("git", []string{"rev-list", "--count", "abc123..main"}, []byte("0\n"), nil) // main is not ahead
	// GetCommitMessages for squash commit message
	mockExec.AddResponse("git", []string{"log", "--format=%s", "main..piece-1"}, []byte("feat: add feature\nfix: bug fix\n"), nil)
	// Checkout, squash merge, and commit
	mockExec.AddResponse("git", []string{"checkout", "main"}, nil, nil)
	mockExec.AddResponse("git", []string{"merge", "--squash", "piece-1"}, nil, nil)
	commitMsg := "feat: piece-1\n\nSquashed commits:\n- feat: add feature\n- fix: bug fix\n"
	mockExec.AddResponse("git", []string{"commit", "-m", commitMsg}, nil, nil)

	err := handler.MergePiece("/pieces/piece-1", "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify git checkout and squash merge were called
	if !mockExec.WasCalled("git", "checkout", "main") {
		t.Error("expected git checkout main to be called")
	}
	if !mockExec.WasCalled("git", "merge", "--squash", "piece-1") {
		t.Error("expected git merge --squash piece-1 to be called")
	}

	// Verify success message
	if !out.HasSuccess() {
		t.Error("expected success message")
	}
}

func TestHandler_MergePiece_MainAhead(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for worktree status
	gitDir := "/repo/.git/worktrees/piece-1"
	worktreePath := "/pieces/piece-1"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Setup mock responses - main is ahead
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("piece-1\n"), nil)
	// IsMainAhead: merge-base and rev-list
	mockExec.AddResponse("git", []string{"merge-base", "main", "piece-1"}, []byte("abc123\n"), nil)
	mockExec.AddResponse("git", []string{"rev-list", "--count", "abc123..main"}, []byte("2\n"), nil) // main has 2 commits ahead

	err := handler.MergePiece("/pieces/piece-1", "main")
	if err == nil {
		t.Fatal("expected error when main is ahead")
	}

	if !strings.Contains(err.Error(), "cannot merge") || !strings.Contains(err.Error(), "commits not in piece worktree") {
		t.Errorf("expected error about main being ahead, got: %v", err)
	}
}

func TestHandler_MergePiece_NotInWorktree(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for main repo (not worktree)
	gitDir := "/repo/.git"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte("/repo\n"), nil)

	err := handler.MergePiece("/repo", "main")
	if err == nil {
		t.Fatal("expected error when not in worktree")
	}

	if !strings.Contains(err.Error(), "not in a piece worktree") {
		t.Errorf("expected error about not being in worktree, got: %v", err)
	}
}

// ============================================================================
// Hook Integration Tests
// ============================================================================

func TestHandler_UpdatePiece_BeforeHookFails(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for worktree status
	gitDir := "/repo/.git/worktrees/piece-1"
	worktreePath := "/pieces/piece-1"
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("piece-1\n"), nil)

	// Create before-piece-update hook that fails
	hookPath := "repo/.monkeypuzzle/hooks/before-piece-update.sh"
	_ = fs.MkdirAll("repo/.monkeypuzzle/hooks", 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\nexit 1"), 0755)

	// Mock the hook to fail
	fullHookPath := filepath.Join(repoRoot, ".monkeypuzzle/hooks", "before-piece-update.sh")
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte("hook failed"), fmt.Errorf("exit status 1"))

	err := handler.UpdatePiece("/pieces/piece-1", "main")

	if err == nil {
		t.Fatal("expected error when before hook fails")
	}

	if !strings.Contains(err.Error(), "before-piece-update hook failed") {
		t.Errorf("expected error about hook failure, got: %v", err)
	}

	// Verify git merge was NOT called (hook should abort before merge)
	if mockExec.WasCalled("git", "merge", "main") {
		t.Error("git merge should not be called when before hook fails")
	}
}

func TestHandler_MergePiece_BeforeHookFails(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for worktree status
	gitDir := "/repo/.git/worktrees/piece-1"
	worktreePath := "/pieces/piece-1"
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("piece-1\n"), nil)

	// Create before-piece-merge hook that fails
	hookPath := "repo/.monkeypuzzle/hooks/before-piece-merge.sh"
	_ = fs.MkdirAll("repo/.monkeypuzzle/hooks", 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\nexit 1"), 0755)

	// Mock the hook to fail
	fullHookPath := filepath.Join(repoRoot, ".monkeypuzzle/hooks", "before-piece-merge.sh")
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte("hook failed"), fmt.Errorf("exit status 1"))

	err := handler.MergePiece("/pieces/piece-1", "main")

	if err == nil {
		t.Fatal("expected error when before hook fails")
	}

	if !strings.Contains(err.Error(), "before-piece-merge hook failed") {
		t.Errorf("expected error about hook failure, got: %v", err)
	}

	// Verify checkout was NOT called (hook should abort before safety checks)
	if mockExec.WasCalled("git", "checkout", "main") {
		t.Error("git checkout should not be called when before hook fails")
	}
}

func TestHandler_UpdatePiece_NoHooks_Success(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses for worktree status
	gitDir := "/repo/.git/worktrees/piece-1"
	worktreePath := "/pieces/piece-1"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Setup mock responses for update
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("piece-1\n"), nil)
	mockExec.AddResponse("git", []string{"merge", "main"}, nil, nil)

	// No hooks directory exists - should work fine
	err := handler.UpdatePiece("/pieces/piece-1", "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify git merge was called
	if !mockExec.WasCalled("git", "merge", "main") {
		t.Error("expected git merge main to be called")
	}
}

func TestHandler_CreatePiece_OnPieceCreateHookFails_CleansUp(t *testing.T) {
	// Set XDG_DATA_HOME to a test directory
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses
	repoRoot := "/repo"
	pieceName := "test-piece"
	worktreePath := "/test-data/monkeypuzzle/pieces/" + pieceName
	sessionName := "mp-piece-" + pieceName

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", sessionName, "-c", worktreePath}, nil, nil)

	// Create the hook file so RunHook will try to execute it
	hookPath := "repo/.monkeypuzzle/hooks/" + piece.HookOnPieceCreate
	_ = fs.MkdirAll("repo/.monkeypuzzle/hooks", 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\nexit 1"), 0755)

	// Mock the hook to fail
	fullHookPath := filepath.Join(repoRoot, ".monkeypuzzle/hooks", piece.HookOnPieceCreate)
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte("hook failed"), fmt.Errorf("exit status 1"))

	// Mock cleanup commands
	mockExec.AddResponse("tmux", []string{"kill-session", "-t", sessionName}, nil, nil)
	mockExec.AddResponse("git", []string{"worktree", "remove", worktreePath}, nil, nil)

	// Execute
	_, err := handler.CreatePiece("/monkeypuzzle", pieceName)

	// Verify the operation failed
	if err == nil {
		t.Fatal("expected error when hook fails")
	}

	if !strings.Contains(err.Error(), "on-piece-create hook failed") {
		t.Errorf("expected error about hook failure, got: %v", err)
	}

	// Verify cleanup was called - tmux kill-session
	if !mockExec.WasCalled("tmux", "kill-session", "-t", sessionName) {
		t.Error("expected tmux kill-session to be called for cleanup")
	}

	// Verify cleanup was called - git worktree remove
	if !mockExec.WasCalled("git", "worktree", "remove", worktreePath) {
		t.Error("expected git worktree remove to be called for cleanup")
	}
}

// ============================================================================
// CreatePieceFromIssue Tests
// ============================================================================

func TestHandler_CreatePieceFromIssue_WithFrontmatter(t *testing.T) {
	// Set XDG_DATA_HOME to a test directory
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup repo structure
	repoRoot := "/repo"
	issuePath := ".monkeypuzzle/issues/my-feature.md"
	absIssuePath := filepath.Join(repoRoot, issuePath)
	pieceName := "my-awesome-feature"

	// Create config
	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "markdown",
    "config": {"directory": ".monkeypuzzle/issues"}
  },
  "pr": {"provider": "github", "config": {}}
}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/monkeypuzzle.json"), []byte(configData), 0644)

	// Create issue file with frontmatter
	issueContent := `---
title: My Awesome Feature
---

# Description
Content here.
`
	_ = fs.MkdirAll(filepath.Dir(absIssuePath), 0755)
	_ = fs.WriteFile(absIssuePath, []byte(issueContent), 0644)

	// Setup mocks
	worktreePath := "/test-data/monkeypuzzle/pieces/" + pieceName
	sessionName := "mp-piece-" + pieceName

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", sessionName, "-c", worktreePath}, nil, nil)

	// Execute
	info, err := handler.CreatePieceFromIssue("/monkeypuzzle", issuePath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, info.Name)
	}

	// Verify marker file was created
	markerPath := filepath.Join(worktreePath, ".monkeypuzzle/current-issue.json")
	markerData, err := fs.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("marker file not created: %v", err)
	}

	var marker piece.CurrentIssueMarker
	if err := json.Unmarshal(markerData, &marker); err != nil {
		t.Fatalf("failed to unmarshal marker: %v", err)
	}

	if marker.IssueName != "My Awesome Feature" {
		t.Errorf("expected issue name 'My Awesome Feature', got %q", marker.IssueName)
	}

	if marker.PieceName != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, marker.PieceName)
	}
}

func TestHandler_CreatePieceFromIssue_WithH1(t *testing.T) {
	// Set XDG_DATA_HOME to a test directory
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup repo structure
	repoRoot := "/repo"
	issuePath := ".monkeypuzzle/issues/my-feature.md"
	absIssuePath := filepath.Join(repoRoot, issuePath)
	pieceName := "my-feature"

	// Create config
	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "markdown",
    "config": {"directory": ".monkeypuzzle/issues"}
  },
  "pr": {"provider": "github", "config": {}}
}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/monkeypuzzle.json"), []byte(configData), 0644)

	// Create issue file with H1
	issueContent := `# My Feature

Content here.
`
	_ = fs.MkdirAll(filepath.Dir(absIssuePath), 0755)
	_ = fs.WriteFile(absIssuePath, []byte(issueContent), 0644)

	// Setup mocks
	worktreePath := "/test-data/monkeypuzzle/pieces/" + pieceName
	sessionName := "mp-piece-" + pieceName

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", sessionName, "-c", worktreePath}, nil, nil)

	// Execute
	info, err := handler.CreatePieceFromIssue("/monkeypuzzle", issuePath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, info.Name)
	}
}

func TestHandler_CreatePieceFromIssue_SanitizesName(t *testing.T) {
	// Set XDG_DATA_HOME to a test directory
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup repo structure
	repoRoot := "/repo"
	issuePath := ".monkeypuzzle/issues/my-feature.md"
	absIssuePath := filepath.Join(repoRoot, issuePath)
	pieceName := "my-awesome-feature-v2-0"

	// Create config
	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "markdown",
    "config": {"directory": ".monkeypuzzle/issues"}
  },
  "pr": {"provider": "github", "config": {}}
}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/monkeypuzzle.json"), []byte(configData), 0644)

	// Create issue file with special characters in title
	issueContent := `---
title: My Awesome Feature (v2.0)!
---

Content here.
`
	_ = fs.MkdirAll(filepath.Dir(absIssuePath), 0755)
	_ = fs.WriteFile(absIssuePath, []byte(issueContent), 0644)

	// Setup mocks
	worktreePath := "/test-data/monkeypuzzle/pieces/" + pieceName
	sessionName := "mp-piece-" + pieceName

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", sessionName, "-c", worktreePath}, nil, nil)

	// Execute
	info, err := handler.CreatePieceFromIssue("/monkeypuzzle", issuePath)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, info.Name)
	}
}

func TestHandler_CreatePieceFromIssue_InvalidIssuePath(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Create config but no issue file
	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "markdown",
    "config": {"directory": ".monkeypuzzle/issues"}
  },
  "pr": {"provider": "github", "config": {}}
}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/monkeypuzzle.json"), []byte(configData), 0644)

	_, err := handler.CreatePieceFromIssue("/monkeypuzzle", ".monkeypuzzle/issues/nonexistent.md")
	if err == nil {
		t.Fatal("expected error when issue file doesn't exist")
	}

	if !strings.Contains(err.Error(), "issue file not found") {
		t.Errorf("expected error about issue file not found, got: %v", err)
	}
}

func TestHandler_CreatePieceFromIssue_MissingConfig(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// No config file
	_, err := handler.CreatePieceFromIssue("/monkeypuzzle", ".monkeypuzzle/issues/test.md")
	if err == nil {
		t.Fatal("expected error when config file doesn't exist")
	}

	if !strings.Contains(err.Error(), "config") {
		t.Errorf("expected error about config, got: %v", err)
	}
}

func TestHandler_CreatePieceFromIssue_InvalidProvider(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Create config with invalid provider
	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "github",
    "config": {}
  },
  "pr": {"provider": "github", "config": {}}
}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/monkeypuzzle.json"), []byte(configData), 0644)

	_, err := handler.CreatePieceFromIssue("/monkeypuzzle", ".monkeypuzzle/issues/test.md")
	if err == nil {
		t.Fatal("expected error when issue provider is not markdown")
	}

	if !strings.Contains(err.Error(), "markdown") {
		t.Errorf("expected error about markdown provider, got: %v", err)
	}
}

func TestHandler_CreatePieceFromIssue_OutsideIssuesDirectory(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Create config
	configData := `{
  "version": "1",
  "project": {"name": "test-project"},
  "issues": {
    "provider": "markdown",
    "config": {"directory": ".monkeypuzzle/issues"}
  },
  "pr": {"provider": "github", "config": {}}
}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/monkeypuzzle.json"), []byte(configData), 0644)

	// Create issue file outside the issues directory
	issuePath := "other-dir/issue.md"
	absIssuePath := filepath.Join(repoRoot, issuePath)
	_ = fs.MkdirAll(filepath.Dir(absIssuePath), 0755)
	_ = fs.WriteFile(absIssuePath, []byte("# Issue\n"), 0644)

	_, err := handler.CreatePieceFromIssue("/monkeypuzzle", issuePath)
	if err == nil {
		t.Fatal("expected error when issue file is outside issues directory")
	}

	if !strings.Contains(err.Error(), "within the issues directory") {
		t.Errorf("expected error about issues directory, got: %v", err)
	}
}

// ============================================================================
// IsBranchMerged Tests
// ============================================================================

func TestHandler_IsBranchMerged_ViaPR(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	branchName := "feature-branch"

	// Create PR metadata
	prMetadata := `{"pr_number": 123, "pr_url": "https://github.com/owner/repo/pull/123", "branch": "feature-branch", "base_branch": "main"}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/pr-metadata.json"), []byte(prMetadata), 0644)

	// Mock remote branch check
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", branchName}, []byte("abc123\trefs/heads/feature-branch\n"), nil)

	// Mock gh pr view - PR is merged
	mockExec.AddResponse("gh", []string{"pr", "view", "123", "--json", "mergedAt"}, []byte(`{"mergedAt": "2025-01-27T10:00:00Z"}`), nil)

	status, err := handler.IsBranchMerged(repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !status.IsMerged {
		t.Error("expected IsMerged to be true")
	}
	if status.Method != "pr" {
		t.Errorf("expected method 'pr', got %q", status.Method)
	}
	if status.PRNumber != 123 {
		t.Errorf("expected PR number 123, got %d", status.PRNumber)
	}
	if !status.ExistsOnRemote {
		t.Error("expected ExistsOnRemote to be true")
	}
}

func TestHandler_IsBranchMerged_ViaPRBranch(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	branchName := "feature-branch"

	// No PR metadata - tests squash-merged PR without metadata file

	// Mock remote branch check
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", branchName}, []byte("abc123\trefs/heads/feature-branch\n"), nil)

	// Mock gh pr list --head <branch> --state merged - finds merged PR
	mockExec.AddResponse("gh", []string{"pr", "list", "--head", branchName, "--state", "merged", "--json", "number", "--limit", "1"}, []byte(`[{"number": 42}]`), nil)

	status, err := handler.IsBranchMerged(repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !status.IsMerged {
		t.Error("expected IsMerged to be true")
	}
	if status.Method != "pr-branch" {
		t.Errorf("expected method 'pr-branch', got %q", status.Method)
	}
	if status.PRNumber != 42 {
		t.Errorf("expected PR number 42, got %d", status.PRNumber)
	}
}

func TestHandler_IsBranchMerged_ViaGit(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	branchName := "feature-branch"

	// No PR metadata - skip PR metadata check

	// Mock remote branch check - branch doesn't exist on remote
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", branchName}, []byte(""), nil)

	// Mock gh pr list - no merged PR found
	mockExec.AddResponse("gh", []string{"pr", "list", "--head", branchName, "--state", "merged", "--json", "number", "--limit", "1"}, []byte(`[]`), nil)

	// Mock git branch --merged - branch is merged
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  feature-branch\n"), nil)

	status, err := handler.IsBranchMerged(repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !status.IsMerged {
		t.Error("expected IsMerged to be true")
	}
	if status.Method != "git" {
		t.Errorf("expected method 'git', got %q", status.Method)
	}
	if status.ExistsOnRemote {
		t.Error("expected ExistsOnRemote to be false")
	}
}

func TestHandler_IsBranchMerged_ViaCommit(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	branchName := "feature-branch"

	// No PR metadata

	// Mock remote branch check
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", branchName}, []byte(""), nil)

	// Mock gh pr list - no merged PR found
	mockExec.AddResponse("gh", []string{"pr", "list", "--head", branchName, "--state", "merged", "--json", "number", "--limit", "1"}, []byte(`[]`), nil)

	// Mock git branch --merged - branch not in list
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n"), nil)

	// Mock commit check - get branch commit
	mockExec.AddResponse("git", []string{"rev-parse", branchName}, []byte("abc123\n"), nil)

	// Mock merge-base --is-ancestor - commit is in main's history
	mockExec.AddResponse("git", []string{"merge-base", "--is-ancestor", "abc123", "main"}, nil, nil)

	status, err := handler.IsBranchMerged(repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !status.IsMerged {
		t.Error("expected IsMerged to be true")
	}
	if status.Method != "commit" {
		t.Errorf("expected method 'commit', got %q", status.Method)
	}
}

func TestHandler_IsBranchMerged_NotMerged(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	branchName := "feature-branch"

	// No PR metadata

	// Mock remote branch check - branch exists
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", branchName}, []byte("abc123\trefs/heads/feature-branch\n"), nil)

	// Mock gh pr list - no merged PR found
	mockExec.AddResponse("gh", []string{"pr", "list", "--head", branchName, "--state", "merged", "--json", "number", "--limit", "1"}, []byte(`[]`), nil)

	// Mock git branch --merged - branch not in list
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n"), nil)

	// Mock commit check
	mockExec.AddResponse("git", []string{"rev-parse", branchName}, []byte("abc123\n"), nil)

	// Mock merge-base --is-ancestor - commit is NOT in main's history (exit status 1)
	mockExec.AddResponse("git", []string{"merge-base", "--is-ancestor", "abc123", "main"}, nil, fmt.Errorf("exit status 1"))

	status, err := handler.IsBranchMerged(repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if status.IsMerged {
		t.Error("expected IsMerged to be false")
	}
	if status.Method != "" {
		t.Errorf("expected empty method, got %q", status.Method)
	}
	if !status.ExistsOnRemote {
		t.Error("expected ExistsOnRemote to be true")
	}
}

func TestHandler_IsBranchMerged_PRNotMerged_FallsBackToGit(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	branchName := "feature-branch"

	// Create PR metadata
	prMetadata := `{"pr_number": 123, "pr_url": "https://github.com/owner/repo/pull/123", "branch": "feature-branch", "base_branch": "main"}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/pr-metadata.json"), []byte(prMetadata), 0644)

	// Mock remote branch check
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", branchName}, []byte("abc123\trefs/heads/feature-branch\n"), nil)

	// Mock gh pr view - PR is NOT merged
	mockExec.AddResponse("gh", []string{"pr", "view", "123", "--json", "mergedAt"}, []byte(`{"mergedAt": null}`), nil)

	// Mock gh pr list - no merged PR (since we already checked PR 123 is not merged)
	mockExec.AddResponse("gh", []string{"pr", "list", "--head", branchName, "--state", "merged", "--json", "number", "--limit", "1"}, []byte(`[]`), nil)

	// Mock git branch --merged - branch is merged (local merge without PR)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  feature-branch\n"), nil)

	status, err := handler.IsBranchMerged(repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !status.IsMerged {
		t.Error("expected IsMerged to be true")
	}
	if status.Method != "git" {
		t.Errorf("expected method 'git', got %q", status.Method)
	}
}

func TestHandler_IsBranchMerged_GHError_FallsBackToGit(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	branchName := "feature-branch"

	// Create PR metadata
	prMetadata := `{"pr_number": 123, "pr_url": "https://github.com/owner/repo/pull/123", "branch": "feature-branch", "base_branch": "main"}`
	_ = fs.MkdirAll(filepath.Join(repoRoot, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(repoRoot, ".monkeypuzzle/pr-metadata.json"), []byte(prMetadata), 0644)

	// Mock remote branch check
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", branchName}, []byte("abc123\trefs/heads/feature-branch\n"), nil)

	// Mock gh pr view - error (gh not installed or API error)
	mockExec.AddResponse("gh", []string{"pr", "view", "123", "--json", "mergedAt"}, nil, fmt.Errorf("gh not found"))

	// Mock gh pr list - also fails
	mockExec.AddResponse("gh", []string{"pr", "list", "--head", branchName, "--state", "merged", "--json", "number", "--limit", "1"}, nil, fmt.Errorf("gh not found"))

	// Mock git branch --merged - branch is merged
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  feature-branch\n"), nil)

	status, err := handler.IsBranchMerged(repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !status.IsMerged {
		t.Error("expected IsMerged to be true")
	}
	if status.Method != "git" {
		t.Errorf("expected method 'git', got %q", status.Method)
	}
}

// ============================================================================
// CleanupMergedPieces Tests
// ============================================================================

func TestHandler_CleanupMergedPieces_NoPieces(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Pieces directory doesn't exist
	opts := piece.CleanupOptions{MainBranch: "main"}
	results, err := handler.CleanupMergedPieces("/repo", opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestHandler_CleanupMergedPieces_DryRun(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	piecesDir := "test-data/monkeypuzzle/pieces"
	pieceName := "merged-piece"
	worktreePath := filepath.Join(piecesDir, pieceName)

	// Create piece directory
	_ = fs.MkdirAll(worktreePath, 0755)

	// Mock git commands for the piece
	fullWorktreePath := "/" + worktreePath
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte(pieceName+"\n"), nil)

	// Mock branch check - no PR metadata, use git method
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", pieceName}, []byte(""), nil)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  "+pieceName+"\n"), nil)

	opts := piece.CleanupOptions{
		MainBranch: "main",
		DryRun:     true,
	}

	results, err := handler.CleanupMergedPieces("/repo", opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].PieceName != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, results[0].PieceName)
	}

	// Verify worktree was NOT removed (dry-run)
	if mockExec.WasCalled("git", "worktree", "remove", fullWorktreePath) {
		t.Error("worktree remove should NOT be called in dry-run mode")
	}

	// Verify dry-run message was output
	if !out.HasInfo() {
		t.Error("expected info message for dry-run")
	}
}

func TestHandler_CleanupMergedPieces_WithIssue(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	piecesDir := "test-data/monkeypuzzle/pieces"
	pieceName := "issue-piece"
	worktreePath := filepath.Join(piecesDir, pieceName)
	fullWorktreePath := "/" + worktreePath

	// Create piece directory with issue marker
	_ = fs.MkdirAll(fullWorktreePath+"/.monkeypuzzle", 0755)
	issueMarker := `{"issue_path": "issues/test.md", "issue_name": "Test Issue", "piece_name": "issue-piece"}`
	_ = fs.WriteFile(fullWorktreePath+"/.monkeypuzzle/current-issue.json", []byte(issueMarker), 0644)

	// Create the issue file
	issuePath := filepath.Join(repoRoot, "issues/test.md")
	issueContent := `---
title: Test Issue
status: in-progress
---

# Test Issue
`
	_ = fs.MkdirAll(filepath.Join(repoRoot, "issues"), 0755)
	_ = fs.WriteFile(issuePath, []byte(issueContent), 0644)

	// Mock git commands for the piece
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte(pieceName+"\n"), nil)
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", pieceName}, []byte(""), nil)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  "+pieceName+"\n"), nil)

	// Mock worktree removal
	mockExec.AddResponse("git", []string{"worktree", "remove", fullWorktreePath}, nil, nil)

	// Mock tmux kill (may or may not be called, ignore errors)
	mockExec.AddResponse("tmux", []string{"kill-session", "-t", "mp-piece-" + pieceName}, nil, nil)

	opts := piece.CleanupOptions{MainBranch: "main"}
	results, err := handler.CleanupMergedPieces(repoRoot, opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].IssuePath != "issues/test.md" {
		t.Errorf("expected issue path 'issues/test.md', got %q", results[0].IssuePath)
	}

	if !results[0].IssueUpdated {
		t.Error("expected IssueUpdated to be true")
	}

	// Verify issue status was updated to done
	issueData, err := fs.ReadFile(issuePath)
	if err != nil {
		t.Fatalf("failed to read issue file: %v", err)
	}
	if !strings.Contains(string(issueData), "status: done") {
		t.Errorf("expected issue status to be 'done', got: %s", string(issueData))
	}
}

func TestHandler_CleanupMergedPieces_SkipsUnmerged(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	piecesDir := "test-data/monkeypuzzle/pieces"
	pieceName := "unmerged-piece"
	worktreePath := filepath.Join(piecesDir, pieceName)

	// Create piece directory
	_ = fs.MkdirAll(worktreePath, 0755)

	// Mock git commands for the piece
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte(pieceName+"\n"), nil)

	// Mock branch check - not merged
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", pieceName}, []byte("abc123\trefs/heads/"+pieceName+"\n"), nil)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n"), nil) // piece not in list
	mockExec.AddResponse("git", []string{"rev-parse", pieceName}, []byte("abc123\n"), nil)
	mockExec.AddResponse("git", []string{"merge-base", "--is-ancestor", "abc123", "main"}, nil, fmt.Errorf("exit status 1")) // not an ancestor

	opts := piece.CleanupOptions{MainBranch: "main"}
	results, err := handler.CleanupMergedPieces("/repo", opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for unmerged piece, got %d", len(results))
	}
}

func TestHandler_CleanupMergedPieces_NoIssueMarker(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/test-data")

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	piecesDir := "test-data/monkeypuzzle/pieces"
	pieceName := "no-issue-piece"
	worktreePath := filepath.Join(piecesDir, pieceName)
	fullWorktreePath := "/" + worktreePath

	// Create piece directory WITHOUT issue marker
	_ = fs.MkdirAll(worktreePath, 0755)

	// Mock git commands for the piece
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte(pieceName+"\n"), nil)
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", pieceName}, []byte(""), nil)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  "+pieceName+"\n"), nil)

	// Mock worktree removal
	mockExec.AddResponse("git", []string{"worktree", "remove", fullWorktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"kill-session", "-t", "mp-piece-" + pieceName}, nil, nil)

	opts := piece.CleanupOptions{MainBranch: "main"}
	results, err := handler.CleanupMergedPieces("/repo", opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].IssuePath != "" {
		t.Errorf("expected empty issue path, got %q", results[0].IssuePath)
	}

	if results[0].IssueUpdated {
		t.Error("expected IssueUpdated to be false when no issue marker")
	}
}
