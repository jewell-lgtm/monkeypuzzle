package pr_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
)

// setupTestPieceWorktree creates a mock piece worktree environment
func setupTestPieceWorktree(t *testing.T, mockExec *adapters.MockExec, fs *adapters.MemoryFS, worktreePath, mainRepoPath string) {
	t.Helper()

	// Create .monkeypuzzle directories
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)
	_ = fs.MkdirAll(filepath.Join(mainRepoPath, ".monkeypuzzle"), 0755)

	// Mock git rev-parse --git-dir to indicate we're in a worktree
	// The gitdir for a worktree is under .git/worktrees/
	gitDir := filepath.Join(mainRepoPath, ".git", "worktrees", "test-piece")
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)

	// Mock git rev-parse --show-toplevel to return worktree path
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)

	// Mock git rev-parse --abbrev-ref HEAD to return branch name
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("test-piece\n"), nil)
}

func TestCreatePR_HappyPath(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/test-piece"
	mainRepoPath := "/repo"

	setupTestPieceWorktree(t, mockExec, fs, worktreePath, mainRepoPath)

	// Mock git push
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)

	// Mock gh pr create
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "Test PR", "--body", "PR body", "--base", "main"},
		[]byte("https://github.com/owner/repo/pull/42\n"), nil)

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	input := pr.Input{
		Title: "Test PR",
		Body:  "PR body",
		Base:  "main",
	}

	result, err := handler.CreatePR(worktreePath, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 42 {
		t.Errorf("expected PR number 42, got %d", result.PRNumber)
	}
	if result.PRURL != "https://github.com/owner/repo/pull/42" {
		t.Errorf("expected PR URL 'https://github.com/owner/repo/pull/42', got %q", result.PRURL)
	}
	if result.Branch != "test-piece" {
		t.Errorf("expected branch 'test-piece', got %q", result.Branch)
	}

	// Verify PR metadata was written
	metadata, err := piece.ReadPRMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("failed to read PR metadata: %v", err)
	}
	if metadata.PRNumber != 42 {
		t.Errorf("metadata PRNumber = %d, want 42", metadata.PRNumber)
	}
	if metadata.Branch != "test-piece" {
		t.Errorf("metadata Branch = %q, want 'test-piece'", metadata.Branch)
	}
}

func TestCreatePR_UsesIssueTitleWhenAvailable(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/test-piece"
	mainRepoPath := "/repo"

	setupTestPieceWorktree(t, mockExec, fs, worktreePath, mainRepoPath)

	// Create issue marker file
	markerPath := filepath.Join(worktreePath, ".monkeypuzzle", "current-issue.json")
	marker := piece.CurrentIssueMarker{
		IssuePath: "issues/my-feature.md",
		IssueName: "My Awesome Feature",
		PieceName: "test-piece",
	}
	markerData, _ := json.Marshal(marker)
	_ = fs.WriteFile(markerPath, markerData, 0644)

	// Mock git push
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)

	// Mock gh pr create - should use issue title since no title provided
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "My Awesome Feature", "--body", "", "--base", "main"},
		[]byte("https://github.com/owner/repo/pull/99\n"), nil)

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	// No title provided - should use issue title
	input := pr.Input{
		Title: "",
		Body:  "",
		Base:  "main",
	}

	result, err := handler.CreatePR(worktreePath, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 99 {
		t.Errorf("expected PR number 99, got %d", result.PRNumber)
	}

	// Verify issue path was stored in metadata
	metadata, err := piece.ReadPRMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("failed to read PR metadata: %v", err)
	}
	if metadata.IssuePath != "issues/my-feature.md" {
		t.Errorf("metadata IssuePath = %q, want 'issues/my-feature.md'", metadata.IssuePath)
	}
}

func TestCreatePR_UsesPieceNameAsFallback(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/my-feature-piece"
	mainRepoPath := "/repo"

	// Create .monkeypuzzle directories
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)
	_ = fs.MkdirAll(filepath.Join(mainRepoPath, ".monkeypuzzle"), 0755)

	// Mock git commands for a worktree named "my-feature-piece"
	gitDir := filepath.Join(mainRepoPath, ".git", "worktrees", "my-feature-piece")
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("my-feature-piece\n"), nil)

	// Mock git push
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)

	// Mock gh pr create - should use piece name since no title or issue
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "my-feature-piece", "--body", "", "--base", "main"},
		[]byte("https://github.com/owner/repo/pull/100\n"), nil)

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	input := pr.Input{
		Title: "", // No title
		Body:  "",
		Base:  "main",
	}

	result, err := handler.CreatePR(worktreePath, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 100 {
		t.Errorf("expected PR number 100, got %d", result.PRNumber)
	}
}

func TestCreatePR_NotInPieceWorktree(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	workDir := "/regular-repo"
	_ = fs.MkdirAll(workDir, 0755)

	// Mock git commands to indicate NOT in a worktree (regular .git directory)
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(filepath.Join(workDir, ".git")+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(workDir+"\n"), nil)

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	input := pr.Input{
		Title: "Test",
		Base:  "main",
	}

	_, err := handler.CreatePR(workDir, input)
	if err == nil {
		t.Error("expected error when not in piece worktree")
	}
}

func TestCreatePR_PushFails(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/test-piece"
	mainRepoPath := "/repo"

	setupTestPieceWorktree(t, mockExec, fs, worktreePath, mainRepoPath)

	// Mock git push - fails
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"},
		[]byte("error: failed to push\n"),
		adapters.MockError("push failed"))

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	input := pr.Input{
		Title: "Test PR",
		Base:  "main",
	}

	_, err := handler.CreatePR(worktreePath, input)
	if err == nil {
		t.Error("expected error when push fails")
	}
}

func TestCreatePR_GhFails(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/test-piece"
	mainRepoPath := "/repo"

	setupTestPieceWorktree(t, mockExec, fs, worktreePath, mainRepoPath)

	// Mock git push - succeeds
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)

	// Mock gh pr create - fails
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "Test PR", "--body", "", "--base", "main"},
		[]byte("Pull request creation failed\n"),
		adapters.MockError("gh failed"))

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	input := pr.Input{
		Title: "Test PR",
		Base:  "main",
	}

	_, err := handler.CreatePR(worktreePath, input)
	if err == nil {
		t.Error("expected error when gh fails")
	}
}

func TestWithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    pr.Input
		expected pr.Input
	}{
		{
			name:     "empty base defaults to main",
			input:    pr.Input{Title: "Test", Body: "Body", Base: ""},
			expected: pr.Input{Title: "Test", Body: "Body", Base: "main"},
		},
		{
			name:     "trims whitespace",
			input:    pr.Input{Title: "  Test  ", Body: "  Body  ", Base: "  develop  "},
			expected: pr.Input{Title: "Test", Body: "Body", Base: "develop"},
		},
		{
			name:     "preserves explicit base",
			input:    pr.Input{Title: "Test", Body: "", Base: "develop"},
			expected: pr.Input{Title: "Test", Body: "", Base: "develop"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pr.WithDefaults(tt.input)
			if result.Title != tt.expected.Title {
				t.Errorf("Title = %q, want %q", result.Title, tt.expected.Title)
			}
			if result.Body != tt.expected.Body {
				t.Errorf("Body = %q, want %q", result.Body, tt.expected.Body)
			}
			if result.Base != tt.expected.Base {
				t.Errorf("Base = %q, want %q", result.Base, tt.expected.Base)
			}
		})
	}
}
