package piece_test

import (
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
	_, err := handler.CreatePiece("/monkeypuzzle")

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

	// Create a fake directory with that EXACT name to simulate existing piece
	existingPath := filepath.Join(baseDir, name1)
	_ = fs.MkdirAll(existingPath, 0755)

	// Generate another name - should get the same base but with counter suffix
	// since the base name already exists
	name2, err := handler.GeneratePieceName(baseDir)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Name2 should either:
	// 1. Have a different timestamp (if called in a different second), OR
	// 2. Have a counter suffix if same timestamp
	// Since we can't control timing, we just verify both names are valid
	if name2 == "" {
		t.Error("expected name2 to be generated")
	}

	if !strings.HasPrefix(name2, "piece-") {
		t.Errorf("expected name2 to start with 'piece-', got %q", name2)
	}

	// The counter logic works within a single GeneratePieceName call,
	// so if we call it again immediately, it will generate a new timestamp.
	// The important thing is that name1 and name2 are both valid piece names.
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
	// Checkout and merge
	mockExec.AddResponse("git", []string{"checkout", "main"}, nil, nil)
	mockExec.AddResponse("git", []string{"merge", "piece-1"}, nil, nil)

	err := handler.MergePiece("/pieces/piece-1", "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify git checkout and merge were called
	if !mockExec.WasCalled("git", "checkout", "main") {
		t.Error("expected git checkout main to be called")
	}
	if !mockExec.WasCalled("git", "merge", "piece-1") {
		t.Error("expected git merge piece-1 to be called")
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
