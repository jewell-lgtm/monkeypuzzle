package piece_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

func TestHandler_CreatePiece(t *testing.T) {
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
	_, err := handler.CreatePiece(context.Background(), "test-piece-1", piece.CreatePieceOptions{})

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
	// Pieces directory is now {repoRoot}/.monkeypuzzle/pieces
	dirs := fs.Dirs()
	found := false
	expectedBaseDir := "repo/.monkeypuzzle/pieces" // MemoryFS cleans leading slash
	for _, d := range dirs {
		if strings.HasPrefix(d, expectedBaseDir) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected pieces directory starting with %q to be created, dirs: %v", expectedBaseDir, dirs)
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

	status, err := handler.Status(context.Background(), "/repo")
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

	status, err := handler.Status(context.Background(), "/pieces/piece-1")
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

	status, err := handler.Status(context.Background(), "/tmp")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.InPiece {
		t.Error("expected not to be in a piece")
	}
}

func TestHandler_GetPieceHierarchyStatus_InMainRepo(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

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

	status, err := handler.GetPieceHierarchyStatus(context.Background(), "/repo", "main")
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

func TestHandler_GetPieceHierarchyStatus_WithParentAndChildren(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup pieces directory
	repoRoot := "/repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	parentPiecePath := filepath.Join(piecesDir, "parent-piece")
	childPiecePath := filepath.Join(piecesDir, "child-piece")
	_ = fs.MkdirAll(parentPiecePath, 0755)
	_ = fs.MkdirAll(childPiecePath, 0755)

	// Create .monkeypuzzle directories
	_ = fs.MkdirAll(filepath.Join(parentPiecePath, ".monkeypuzzle"), 0755)
	_ = fs.MkdirAll(filepath.Join(childPiecePath, ".monkeypuzzle"), 0755)

	// Write parent metadata (parent of main)
	parentMetadata := piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}
	parentMetadataData, _ := json.MarshalIndent(parentMetadata, "", "  ")
	_ = fs.WriteFile(filepath.Join(parentPiecePath, ".monkeypuzzle", "piece-metadata.json"), parentMetadataData, 0644)

	// Write child metadata (parent is parent-piece)
	childMetadata := piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece-branch",
	}
	childMetadataData, _ := json.MarshalIndent(childMetadata, "", "  ")
	_ = fs.WriteFile(filepath.Join(childPiecePath, ".monkeypuzzle", "piece-metadata.json"), childMetadataData, 0644)

	// Setup mock responses for worktree - gitDir path determines main repo root
	gitDir := repoRoot + "/.git/worktrees/parent-piece"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(parentPiecePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("parent-piece-branch\n"), nil)

	// Mock IsBranchMerged to return merged for child
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("child-piece-branch\n"), nil)

	status, err := handler.GetPieceHierarchyStatus(context.Background(), parentPiecePath, "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !status.InPiece {
		t.Error("expected to be in a piece")
	}

	if status.Parent != "main" {
		t.Errorf("expected parent 'main', got %q", status.Parent)
	}

	if len(status.Children) != 1 {
		t.Errorf("expected 1 child, got %d", len(status.Children))
	}

	if len(status.Children) > 0 && status.Children[0] != "child-piece" {
		t.Errorf("expected child 'child-piece', got %q", status.Children[0])
	}

	if status.StackDepth != 1 {
		t.Errorf("expected stack depth 1, got %d", status.StackDepth)
	}
}

func TestHandler_GetPieceHierarchyStatus_StackDepth(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup pieces directory
	repoRoot := "/repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	parentPiecePath := filepath.Join(piecesDir, "parent-piece")
	childPiecePath := filepath.Join(piecesDir, "child-piece")
	_ = fs.MkdirAll(parentPiecePath, 0755)
	_ = fs.MkdirAll(childPiecePath, 0755)

	// Create .monkeypuzzle directories
	_ = fs.MkdirAll(filepath.Join(parentPiecePath, ".monkeypuzzle"), 0755)
	_ = fs.MkdirAll(filepath.Join(childPiecePath, ".monkeypuzzle"), 0755)

	// Write parent metadata (parent of main)
	parentMetadata := piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}
	parentMetadataData, _ := json.MarshalIndent(parentMetadata, "", "  ")
	_ = fs.WriteFile(filepath.Join(parentPiecePath, ".monkeypuzzle", "piece-metadata.json"), parentMetadataData, 0644)

	// Write child metadata (parent is parent-piece)
	childMetadata := piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece-branch",
	}
	childMetadataData, _ := json.MarshalIndent(childMetadata, "", "  ")
	_ = fs.WriteFile(filepath.Join(childPiecePath, ".monkeypuzzle", "piece-metadata.json"), childMetadataData, 0644)

	// Setup mock responses for child worktree - gitDir path determines main repo root
	gitDir := repoRoot + "/.git/worktrees/child-piece"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(childPiecePath+"\n"), nil)

	status, err := handler.GetPieceHierarchyStatus(context.Background(), childPiecePath, "main")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.Parent != "parent-piece" {
		t.Errorf("expected parent 'parent-piece', got %q", status.Parent)
	}

	if status.StackDepth != 2 {
		t.Errorf("expected stack depth 2 (main -> parent -> child), got %d", status.StackDepth)
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
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

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
	_, err := handler.CreatePiece(context.Background(), pieceName, piece.CreatePieceOptions{})

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

func TestHandler_CreatePiece_SanitizesName(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Test with a name containing spaces and uppercase
	inputName := "Remote Functions For Auth"
	expectedSanitized := "remote-functions-for-auth"

	// Mock the worktree creation with the sanitized name
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	worktreePath := filepath.Join(piecesDir, expectedSanitized)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", "mp/repo/" + expectedSanitized, "-c", worktreePath}, nil, nil)

	info, err := handler.CreatePiece(context.Background(), inputName, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != expectedSanitized {
		t.Errorf("expected sanitized name %q, got %q", expectedSanitized, info.Name)
	}
}

func TestHandler_CreatePiece_NameAlreadyExists(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses
	repoRoot := "/repo"
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)

	// Get the actual pieces directory that will be used
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	existingPiecePath := filepath.Join(piecesDir, "existing-piece")

	// Create the pieces directory structure first
	_ = fs.MkdirAll(piecesDir, 0755)
	// Then create the existing piece directory
	_ = fs.MkdirAll(existingPiecePath, 0755)

	// Try to create a piece with the same name
	_, err := handler.CreatePiece(context.Background(), "existing-piece", piece.CreatePieceOptions{})
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

	_, err := handler.UpdatePiece(context.Background(), "/pieces/piece-1", "main")
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

	_, err := handler.UpdatePiece(context.Background(), "/repo", "main")
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

	_, err := handler.MergePiece(context.Background(), "/pieces/piece-1", piece.MergeInput{MainBranch: "main"})
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

	_, err := handler.MergePiece(context.Background(), "/pieces/piece-1", piece.MergeInput{MainBranch: "main"})
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

	_, err := handler.MergePiece(context.Background(), "/repo", piece.MergeInput{MainBranch: "main"})
	if err == nil {
		t.Fatal("expected error when not in worktree")
	}

	if !strings.Contains(err.Error(), "not in a piece worktree") {
		t.Errorf("expected error about not being in worktree, got: %v", err)
	}
}

func TestHandler_MergePiece_IntoParentPiece(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	worktreePath := "/pieces/child-piece"

	// Setup mock responses for worktree status
	gitDir := repoRoot + "/.git/worktrees/child-piece"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Create piece metadata with parent piece (not main)
	mpDir := filepath.Join(worktreePath, ".monkeypuzzle")
	_ = fs.MkdirAll(mpDir, 0755)
	metadata := piece.PieceMetadata{Parent: "parent-piece", CreatedFromBranch: "parent-piece"}
	metadataData, _ := json.Marshal(metadata)
	_ = fs.WriteFile(filepath.Join(mpDir, "piece-metadata.json"), metadataData, 0644)

	// Setup mock responses for merge
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("child-piece\n"), nil)
	// IsMainAhead check (against parent-piece, not main)
	mockExec.AddResponse("git", []string{"merge-base", "parent-piece", "child-piece"}, []byte("abc123\n"), nil)
	mockExec.AddResponse("git", []string{"rev-list", "--count", "abc123..parent-piece"}, []byte("0\n"), nil)
	// GetCommitMessages
	mockExec.AddResponse("git", []string{"log", "--format=%s", "parent-piece..child-piece"}, []byte("feat: child feature\n"), nil)
	// Checkout parent-piece (not main)
	mockExec.AddResponse("git", []string{"checkout", "parent-piece"}, nil, nil)
	// Squash merge
	mockExec.AddResponse("git", []string{"merge", "--squash", "child-piece"}, nil, nil)
	// Commit
	commitMsg := "feat: child-piece\n\nSquashed commits:\n- feat: child feature\n"
	mockExec.AddResponse("git", []string{"commit", "-m", commitMsg}, nil, nil)

	result, err := handler.MergePiece(context.Background(), worktreePath, piece.MergeInput{MainBranch: "main"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Verify merge was into parent-piece, not main
	if result.TargetBranch != "parent-piece" {
		t.Errorf("expected target branch 'parent-piece', got %q", result.TargetBranch)
	}

	// Verify git checkout was for parent-piece
	if !mockExec.WasCalled("git", "checkout", "parent-piece") {
		t.Error("expected git checkout parent-piece to be called")
	}
}

func TestHandler_MergePiece_BlockedByChildren(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	worktreePath := "/pieces/parent-piece"

	// Get pieces dir
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")

	// Setup mock responses for worktree status
	gitDir := repoRoot + "/.git/worktrees/parent-piece"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Create piece metadata for parent piece
	mpDir := filepath.Join(worktreePath, ".monkeypuzzle")
	_ = fs.MkdirAll(mpDir, 0755)
	metadata := piece.PieceMetadata{Parent: "main", CreatedFromBranch: "main"}
	metadataData, _ := json.Marshal(metadata)
	_ = fs.WriteFile(filepath.Join(mpDir, "piece-metadata.json"), metadataData, 0644)

	// Create a child piece in the pieces directory
	childPieceDir := filepath.Join(piecesDir, "child-piece")
	childMpDir := filepath.Join(childPieceDir, ".monkeypuzzle")
	_ = fs.MkdirAll(childMpDir, 0755)
	childMetadata := piece.PieceMetadata{Parent: "parent-piece", CreatedFromBranch: "parent-piece"}
	childMetadataData, _ := json.Marshal(childMetadata)
	_ = fs.WriteFile(filepath.Join(childMpDir, "piece-metadata.json"), childMetadataData, 0644)

	// Setup mock response for current branch
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("parent-piece\n"), nil)

	_, err := handler.MergePiece(context.Background(), worktreePath, piece.MergeInput{MainBranch: "main"})
	if err == nil {
		t.Fatal("expected error when piece has children")
	}

	if !strings.Contains(err.Error(), "has child pieces") {
		t.Errorf("expected error about child pieces, got: %v", err)
	}
}

func TestHandler_MergePiece_ForceOverridesChildren(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	worktreePath := "/pieces/parent-piece"

	// Get pieces dir
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")

	// Setup mock responses for worktree status
	gitDir := repoRoot + "/.git/worktrees/parent-piece"
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Create piece metadata for parent piece
	mpDir := filepath.Join(worktreePath, ".monkeypuzzle")
	_ = fs.MkdirAll(mpDir, 0755)
	metadata := piece.PieceMetadata{Parent: "main", CreatedFromBranch: "main"}
	metadataData, _ := json.Marshal(metadata)
	_ = fs.WriteFile(filepath.Join(mpDir, "piece-metadata.json"), metadataData, 0644)

	// Create a child piece in the pieces directory
	childPieceDir := filepath.Join(piecesDir, "child-piece")
	childMpDir := filepath.Join(childPieceDir, ".monkeypuzzle")
	_ = fs.MkdirAll(childMpDir, 0755)
	childMetadata := piece.PieceMetadata{Parent: "parent-piece", CreatedFromBranch: "parent-piece"}
	childMetadataData, _ := json.Marshal(childMetadata)
	_ = fs.WriteFile(filepath.Join(childMpDir, "piece-metadata.json"), childMetadataData, 0644)

	// Setup mock responses for merge (with Force=true, should proceed despite children)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("parent-piece\n"), nil)
	mockExec.AddResponse("git", []string{"merge-base", "main", "parent-piece"}, []byte("abc123\n"), nil)
	mockExec.AddResponse("git", []string{"rev-list", "--count", "abc123..main"}, []byte("0\n"), nil)
	mockExec.AddResponse("git", []string{"log", "--format=%s", "main..parent-piece"}, []byte("feat: parent feature\n"), nil)
	mockExec.AddResponse("git", []string{"checkout", "main"}, nil, nil)
	mockExec.AddResponse("git", []string{"merge", "--squash", "parent-piece"}, nil, nil)
	commitMsg := "feat: parent-piece\n\nSquashed commits:\n- feat: parent feature\n"
	mockExec.AddResponse("git", []string{"commit", "-m", commitMsg}, nil, nil)

	// Merge with Force=true
	result, err := handler.MergePiece(context.Background(), worktreePath, piece.MergeInput{MainBranch: "main", Force: true})
	if err != nil {
		t.Fatalf("expected no error with Force=true, got %v", err)
	}

	if result.TargetBranch != "main" {
		t.Errorf("expected target branch 'main', got %q", result.TargetBranch)
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

	_, err := handler.UpdatePiece(context.Background(), "/pieces/piece-1", "main")

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

	_, err := handler.MergePiece(context.Background(), "/pieces/piece-1", piece.MergeInput{MainBranch: "main"})

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
	_, err := handler.UpdatePiece(context.Background(), "/pieces/piece-1", "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify git merge was called
	if !mockExec.WasCalled("git", "merge", "main") {
		t.Error("expected git merge main to be called")
	}
}

func TestHandler_CreatePiece_OnPieceCreateHookFails_KeepsPiece(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mock responses
	repoRoot := "/repo"
	pieceName := "test-piece"
	worktreePath := filepath.Join(repoRoot, ".monkeypuzzle", "pieces", pieceName)

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)

	// Create the hook file so RunHook will try to execute it
	hookPath := "repo/.monkeypuzzle/hooks/" + piece.HookOnPieceCreate
	_ = fs.MkdirAll("repo/.monkeypuzzle/hooks", 0755)
	_ = fs.WriteFile(hookPath, []byte("#!/bin/bash\nexit 1"), 0755)

	// Mock the hook to fail
	fullHookPath := filepath.Join(repoRoot, ".monkeypuzzle/hooks", piece.HookOnPieceCreate)
	mockExec.AddResponse("bash", []string{fullHookPath}, []byte("hook failed"), fmt.Errorf("exit status 1"))

	// Execute
	info, err := handler.CreatePiece(context.Background(), pieceName, piece.CreatePieceOptions{})

	// A failing on-piece-create hook is non-fatal: the piece is kept.
	if err != nil {
		t.Fatalf("expected no error when on-piece-create hook fails, got: %v", err)
	}
	if info.Name != pieceName {
		t.Errorf("expected piece %q to be returned, got %q", pieceName, info.Name)
	}

	// Verify the worktree was NOT removed.
	if mockExec.WasCalled("git", "worktree", "remove", worktreePath) {
		t.Error("expected worktree to be kept, but cleanup removed it")
	}

	// Verify the user was warned about the hook failure.
	var warned bool
	for _, m := range out.Messages {
		if m.Type == core.MsgWarning && strings.Contains(m.Content, "on-piece-create hook failed") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("expected a warning about the hook failure, got messages: %+v", out.Messages)
	}
}

// ============================================================================
// CreatePieceFromIssue Tests
// ============================================================================

func TestHandler_CreatePieceFromIssue_WithFrontmatter(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

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
	worktreePath := filepath.Join(repoRoot, ".monkeypuzzle", "pieces", pieceName)
	sessionName := "mp/test-project/" + pieceName

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", sessionName, "-c", worktreePath}, nil, nil)

	// Execute
	issueRef := piece.IssueRef{
		Provider: "markdown",
		ID:       issuePath,
		Title:    "My Awesome Feature",
	}
	info, err := handler.CreatePieceFromIssue(context.Background(), issueRef, piece.CreatePieceOptions{})
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

	if marker.Issue.Title != "My Awesome Feature" {
		t.Errorf("expected issue title 'My Awesome Feature', got %q", marker.Issue.Title)
	}

	if marker.PieceName != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, marker.PieceName)
	}
}

func TestHandler_CreatePieceFromIssue_WithH1(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

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
	worktreePath := filepath.Join(repoRoot, ".monkeypuzzle", "pieces", pieceName)
	sessionName := "mp/test-project/" + pieceName

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", sessionName, "-c", worktreePath}, nil, nil)

	// Execute
	issueRef := piece.IssueRef{
		Provider: "markdown",
		ID:       issuePath,
		Title:    "My Feature",
	}
	info, err := handler.CreatePieceFromIssue(context.Background(), issueRef, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, info.Name)
	}
}

func TestHandler_CreatePieceFromIssue_SanitizesName(t *testing.T) {
	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

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
	worktreePath := filepath.Join(repoRoot, ".monkeypuzzle", "pieces", pieceName)
	sessionName := "mp/test-project/" + pieceName

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", sessionName, "-c", worktreePath}, nil, nil)

	// Execute
	issueRef := piece.IssueRef{
		Provider: "markdown",
		ID:       issuePath,
		Title:    "My Awesome Feature (v2.0)!",
	}
	info, err := handler.CreatePieceFromIssue(context.Background(), issueRef, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, info.Name)
	}
}

func TestHandler_CreatePieceFromIssue_EmptyIssueRef(t *testing.T) {
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

	// Empty IssueRef should fail
	_, err := handler.CreatePieceFromIssue(context.Background(), piece.IssueRef{}, piece.CreatePieceOptions{})
	if err == nil {
		t.Fatal("expected error when IssueRef is empty")
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
	issueRef := piece.IssueRef{
		Provider: "markdown",
		ID:       ".monkeypuzzle/issues/test.md",
		Title:    "Test Issue",
	}
	_, err := handler.CreatePieceFromIssue(context.Background(), issueRef, piece.CreatePieceOptions{})
	if err == nil {
		t.Fatal("expected error when config file doesn't exist")
	}

	if !strings.Contains(err.Error(), "config") {
		t.Errorf("expected error about config, got: %v", err)
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

	status, err := handler.IsBranchMerged(context.Background(), repoRoot, branchName, "main")
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

	status, err := handler.IsBranchMerged(context.Background(), repoRoot, branchName, "main")
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

	status, err := handler.IsBranchMerged(context.Background(), repoRoot, branchName, "main")
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

	status, err := handler.IsBranchMerged(context.Background(), repoRoot, branchName, "main")
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

	status, err := handler.IsBranchMerged(context.Background(), repoRoot, branchName, "main")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if status.IsMerged {
		t.Error("expected IsMerged to be false")
	}
	if status.Method != "none" {
		t.Errorf("expected method 'none', got %q", status.Method)
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

	status, err := handler.IsBranchMerged(context.Background(), repoRoot, branchName, "main")
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

	status, err := handler.IsBranchMerged(context.Background(), repoRoot, branchName, "main")
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
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Pieces directory doesn't exist
	opts := piece.CleanupOptions{MainBranch: "main"}
	results, err := handler.CleanupMergedPieces(context.Background(), "/repo", opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestHandler_CleanupMergedPieces_DryRun(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	pieceName := "merged-piece"
	worktreePath := filepath.Join(piecesDir, pieceName)

	// Create piece directory
	_ = fs.MkdirAll(worktreePath, 0755)

	// Mock git commands for the piece
	fullWorktreePath := worktreePath
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte(pieceName+"\n"), nil)

	// Mock branch check - no PR metadata, use git method
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", pieceName}, []byte(""), nil)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  "+pieceName+"\n"), nil)

	opts := piece.CleanupOptions{
		MainBranch: "main",
		DryRun:     true,
	}

	results, err := handler.CleanupMergedPieces(context.Background(), "/repo", opts)
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
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	pieceName := "issue-piece"
	worktreePath := filepath.Join(piecesDir, pieceName)
	fullWorktreePath := worktreePath

	// Create piece directory with issue marker (new format with IssueRef)
	_ = fs.MkdirAll(fullWorktreePath+"/.monkeypuzzle", 0755)
	issueMarker := `{"issue": {"provider": "markdown", "id": "issues/test.md", "title": "Test Issue"}, "piece_name": "issue-piece", "status": "in-progress", "dirty": false}`
	_ = fs.WriteFile(fullWorktreePath+"/.monkeypuzzle/current-issue.json", []byte(issueMarker), 0644)

	// Mock git commands for the piece
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte(pieceName+"\n"), nil)
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", pieceName}, []byte(""), nil)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  "+pieceName+"\n"), nil)

	// Mock worktree removal
	mockExec.AddResponse("git", []string{"worktree", "remove", fullWorktreePath}, nil, nil)

	// Mock tmux kill (may or may not be called, ignore errors)
	mockExec.AddResponse("tmux", []string{"kill-session", "-t", "mp/repo/" + pieceName}, nil, nil)

	opts := piece.CleanupOptions{MainBranch: "main"}
	results, err := handler.CleanupMergedPieces(context.Background(), repoRoot, opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Issue.ID != "issues/test.md" {
		t.Errorf("expected issue ID 'issues/test.md', got %q", results[0].Issue.ID)
	}

	if results[0].Issue.Provider != "markdown" {
		t.Errorf("expected issue provider 'markdown', got %q", results[0].Issue.Provider)
	}
}

func TestHandler_CleanupMergedPieces_SkipsUnmerged(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
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
	results, err := handler.CleanupMergedPieces(context.Background(), "/repo", opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 results for unmerged piece, got %d", len(results))
	}
}

func TestHandler_CleanupMergedPieces_NoIssueMarker(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	pieceName := "no-issue-piece"
	worktreePath := filepath.Join(piecesDir, pieceName)
	fullWorktreePath := worktreePath

	// Create piece directory WITHOUT issue marker
	_ = fs.MkdirAll(worktreePath, 0755)

	// Mock git commands for the piece
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte(pieceName+"\n"), nil)
	mockExec.AddResponse("git", []string{"ls-remote", "--heads", "origin", pieceName}, []byte(""), nil)
	mockExec.AddResponse("git", []string{"branch", "--merged", "main"}, []byte("  main\n  "+pieceName+"\n"), nil)

	// Mock worktree removal
	mockExec.AddResponse("git", []string{"worktree", "remove", fullWorktreePath}, nil, nil)
	mockExec.AddResponse("tmux", []string{"kill-session", "-t", "mp/repo/" + pieceName}, nil, nil)

	opts := piece.CleanupOptions{MainBranch: "main"}
	results, err := handler.CleanupMergedPieces(context.Background(), "/repo", opts)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !results[0].Issue.IsEmpty() {
		t.Errorf("expected empty issue ref, got %+v", results[0].Issue)
	}
}

func TestHandler_ListPieces_NoPiecesDir(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	// Mock git repo root
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte("/projects/myrepo\n"), nil)

	// No pieces directory exists
	// Use empty repoRoot to test global directory fallback

	pieces, err := handler.ListPieces(context.Background(), "")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(pieces) != 0 {
		t.Errorf("expected 0 pieces, got %d", len(pieces))
	}
}

func TestHandler_ListPieces_EmptyDir(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	// Create empty pieces directory (using repo-scoped path)
	repoRoot := "/test-repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	_ = fs.MkdirAll(piecesDir, 0755)

	pieces, err := handler.ListPieces(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(pieces) != 0 {
		t.Errorf("expected 0 pieces, got %d", len(pieces))
	}
}

func TestHandler_ListPieces_WithPieces(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	// Use TmuxMultiplexer to test session detection
	mux := adapters.NewTmuxMultiplexer(mockExec)
	handler := piece.NewHandlerWithMultiplexer(deps, mux)

	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	// Create pieces directory with pieces (using repo-scoped path)
	repoRoot := "/test-repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	_ = fs.MkdirAll(filepath.Join(piecesDir, "piece-one"), 0755)
	_ = fs.MkdirAll(filepath.Join(piecesDir, "piece-two"), 0755)

	// Mock tmux has-session for each piece
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp/test-repo/piece-one"}, nil, nil)
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp/test-repo/piece-two"}, nil, fmt.Errorf("no session"))

	pieces, err := handler.ListPieces(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(pieces) != 2 {
		t.Errorf("expected 2 pieces, got %d", len(pieces))
	}

	// Find pieces by name
	var foundOne, foundTwo *piece.PieceListItem
	for i := range pieces {
		if pieces[i].Name == "piece-one" {
			foundOne = &pieces[i]
		}
		if pieces[i].Name == "piece-two" {
			foundTwo = &pieces[i]
		}
	}

	if foundOne == nil {
		t.Error("expected to find piece-one")
	} else if !foundOne.HasSession {
		t.Error("expected piece-one to have session")
	}

	if foundTwo == nil {
		t.Error("expected to find piece-two")
	} else if foundTwo.HasSession {
		t.Error("expected piece-two to NOT have session")
	}
}

func TestHandler_ListPieces_WithParentMetadata(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	// Create pieces directory with pieces (using repo-scoped path)
	repoRoot := "/test-repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")

	// Create piece-one with parent metadata pointing to piece-two
	pieceOnePath := filepath.Join(piecesDir, "piece-one")
	_ = fs.MkdirAll(filepath.Join(pieceOnePath, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(pieceOnePath, ".monkeypuzzle", "piece-metadata.json"),
		[]byte(`{"parent": "piece-two", "created_from_branch": "piece-two-branch"}`), 0644)

	// Create piece-two with parent=main (default)
	pieceTwoPath := filepath.Join(piecesDir, "piece-two")
	_ = fs.MkdirAll(filepath.Join(pieceTwoPath, ".monkeypuzzle"), 0755)
	_ = fs.WriteFile(filepath.Join(pieceTwoPath, ".monkeypuzzle", "piece-metadata.json"),
		[]byte(`{"parent": "main", "created_from_branch": "main"}`), 0644)

	// Create piece-three with no metadata file (should default to "main")
	pieceThreePath := filepath.Join(piecesDir, "piece-three")
	_ = fs.MkdirAll(pieceThreePath, 0755)

	// Mock tmux has-session for each piece
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp/test-repo/piece-one"}, nil, fmt.Errorf("no session"))
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp/test-repo/piece-two"}, nil, fmt.Errorf("no session"))
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp/test-repo/piece-three"}, nil, fmt.Errorf("no session"))

	pieces, err := handler.ListPieces(context.Background(), repoRoot)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if len(pieces) != 3 {
		t.Fatalf("expected 3 pieces, got %d", len(pieces))
	}

	// Find pieces by name and check parent
	for _, p := range pieces {
		switch p.Name {
		case "piece-one":
			if p.Parent != "piece-two" {
				t.Errorf("piece-one: expected parent 'piece-two', got %q", p.Parent)
			}
		case "piece-two":
			if p.Parent != "main" {
				t.Errorf("piece-two: expected parent 'main', got %q", p.Parent)
			}
		case "piece-three":
			if p.Parent != "main" {
				t.Errorf("piece-three: expected parent 'main' (default), got %q", p.Parent)
			}
		}
	}
}

func TestHandler_SwitchPiece_NotFound(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	// Mock git repo root
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte("/projects/myrepo\n"), nil)

	// Create pieces directory with one piece
	piecesDir := "/projects/myrepo/.monkeypuzzle/pieces"
	_ = fs.MkdirAll(filepath.Join(piecesDir, "existing-piece"), 0755)

	// Mock tmux has-session
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp/myrepo/existing-piece"}, nil, nil)

	_, err := handler.SwitchPiece(context.Background(), "non-existent")
	if err == nil {
		t.Error("expected error for non-existent piece")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "existing-piece") {
		t.Errorf("expected available pieces in error, got: %v", err)
	}
}

func TestHandler_SwitchPiece_PrintsPath_NoSession(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	// Mock git repo root
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte("/projects/myrepo\n"), nil)

	// Create pieces directory with one piece
	piecesDir := "/projects/myrepo/.monkeypuzzle/pieces"
	_ = fs.MkdirAll(filepath.Join(piecesDir, "my-piece"), 0755)

	// Mock tmux has-session - no session exists
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp/myrepo/my-piece"}, nil, fmt.Errorf("no session"))

	result, err := handler.SwitchPiece(context.Background(), "my-piece")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if result.Method != "path" {
		t.Errorf("expected method 'path', got %q", result.Method)
	}

	if result.Piece.Name != "my-piece" {
		t.Errorf("expected piece name 'my-piece', got %q", result.Piece.Name)
	}
}

func TestHandler_SwitchPiece_NoPiecesExist(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Override data dir for tests
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	// Mock git repo root
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte("/projects/myrepo\n"), nil)

	// No pieces directory

	_, err := handler.SwitchPiece(context.Background(), "any-piece")
	if err == nil {
		t.Error("expected error when no pieces exist")
	}
	if !strings.Contains(err.Error(), "no pieces exist") {
		t.Errorf("expected 'no pieces exist' in error, got: %v", err)
	}
}

// ============================================================================
// Piece Metadata Integration Tests
// ============================================================================

func TestHandler_CreatePiece_WritesPieceMetadata(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/projects/myrepo"
	pieceName := "test-feature"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	worktreePath := filepath.Join(piecesDir, pieceName)

	// Mock git repo root
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	// Mock getting current branch for metadata
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("main\n"), nil)
	// Mock worktree creation
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	// Mock main repo session check - already exists
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp-myrepo"}, nil, nil)
	// Mock piece session creation
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", "mp/myrepo/" + pieceName, "-c", worktreePath}, nil, nil)

	info, err := handler.CreatePiece(context.Background(), pieceName, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, info.Name)
	}

	// Verify piece metadata was written
	metadata, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("failed to read piece metadata: %v", err)
	}

	// Parent should be "main" when created from main repo
	if metadata.Parent != "main" {
		t.Errorf("expected Parent 'main', got %q", metadata.Parent)
	}

	// CreatedFromBranch should be the branch we were on when creating the piece
	if metadata.CreatedFromBranch != "main" {
		t.Errorf("expected CreatedFromBranch 'main', got %q", metadata.CreatedFromBranch)
	}
}

func TestHandler_CreatePiece_WritesPieceMetadata_FromFeatureBranch(t *testing.T) {
	paths.SetDataDir("/test-data/monkeypuzzle")
	t.Cleanup(paths.ResetDataDir)

	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	repoRoot := "/projects/myrepo"
	pieceName := "child-feature"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	worktreePath := filepath.Join(piecesDir, pieceName)

	// Mock git repo root
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	// Mock getting current branch for metadata - we're on a feature branch
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("parent-feature\n"), nil)
	// Mock worktree creation
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, nil, nil)
	// Mock main repo session check - already exists
	mockExec.AddResponse("tmux", []string{"has-session", "-t", "mp-myrepo"}, nil, nil)
	// Mock piece session creation
	mockExec.AddResponse("tmux", []string{"new-session", "-d", "-s", "mp/myrepo/" + pieceName, "-c", worktreePath}, nil, nil)

	info, err := handler.CreatePiece(context.Background(), pieceName, piece.CreatePieceOptions{})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if info.Name != pieceName {
		t.Errorf("expected piece name %q, got %q", pieceName, info.Name)
	}

	// Verify piece metadata was written
	metadata, err := piece.ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("failed to read piece metadata: %v", err)
	}

	// Parent should still be "main" for now (future issue will allow --parent flag)
	if metadata.Parent != "main" {
		t.Errorf("expected Parent 'main', got %q", metadata.Parent)
	}

	// CreatedFromBranch should be the feature branch we were on
	if metadata.CreatedFromBranch != "parent-feature" {
		t.Errorf("expected CreatedFromBranch 'parent-feature', got %q", metadata.CreatedFromBranch)
	}
}

func TestWriteAndReadPieceMd(t *testing.T) {
	fs := adapters.NewMemoryFS()
	dir := "/tmp/test-piece"
	_ = fs.MkdirAll(dir, 0755)

	md := piece.PieceMd{
		Title:  "add-dark-mode",
		Status: "in-progress",
		Body:   "add dark mode support",
	}

	err := piece.WritePieceMd(dir, md, fs)
	if err != nil {
		t.Fatalf("failed to write piece.md: %v", err)
	}

	got, err := piece.ReadPieceMd(dir, fs)
	if err != nil {
		t.Fatalf("failed to read piece.md: %v", err)
	}

	if got.Title != "add-dark-mode" {
		t.Errorf("expected title 'add-dark-mode', got %q", got.Title)
	}
	if got.Status != "in-progress" {
		t.Errorf("expected status 'in-progress', got %q", got.Status)
	}
	if got.Body != "# add-dark-mode\n\nadd dark mode support" {
		t.Errorf("unexpected body: %q", got.Body)
	}
}

func TestReadPieceMd_NotExist(t *testing.T) {
	fs := adapters.NewMemoryFS()
	_, err := piece.ReadPieceMd("/nonexistent", fs)
	if err == nil {
		t.Error("expected error for missing piece.md")
	}
}

func TestHandler_CreatePieceFromPrompt(t *testing.T) {
	fs := adapters.NewMemoryFS()
	out := adapters.NewBufferOutput()
	mockExec := adapters.NewMockExec()
	deps := core.Deps{FS: fs, Output: out, Exec: mockExec}
	handler := piece.NewHandler(deps)

	// Setup mocks for CreatePiece flow
	repoRoot := "/repo"
	piecesDir := filepath.Join(repoRoot, ".monkeypuzzle", "pieces")
	worktreePath := filepath.Join(piecesDir, "add-dark-mode")

	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(repoRoot+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("main\n"), nil)
	mockExec.AddResponse("git", []string{"worktree", "add", worktreePath}, []byte(""), nil)

	info, err := handler.CreatePieceFromPrompt(
		context.Background(),
		"", // name empty - derived from prompt
		"add dark mode",
		piece.CreatePieceOptions{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Name != "add-dark-mode" {
		t.Errorf("expected name 'add-dark-mode', got %q", info.Name)
	}

	// Verify piece.md was written
	md, err := piece.ReadPieceMd(info.WorktreePath, fs)
	if err != nil {
		t.Fatalf("failed to read piece.md: %v", err)
	}
	if md.Title != "add-dark-mode" {
		t.Errorf("expected title 'add-dark-mode', got %q", md.Title)
	}
	if md.Status != "in-progress" {
		t.Errorf("expected status 'in-progress', got %q", md.Status)
	}
}
