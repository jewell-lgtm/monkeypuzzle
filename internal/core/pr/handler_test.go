package pr_test

import (
	"context"
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

	// Write minimal monkeypuzzle.json so pr.Handler.providerForRepo can resolve a provider.
	configData := `{
  "version": "1",
  "project": {"name": "test"},
  "issues": {"provider": "markdown", "config": {"directory": "issues"}},
  "pr": {"provider": "github", "config": {}}
}`
	_ = fs.WriteFile(filepath.Join(mainRepoPath, ".monkeypuzzle/monkeypuzzle.json"), []byte(configData), 0644)

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

	result, err := handler.CreatePR(context.Background(), worktreePath, input)
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

	// Record the issue ref in piece metadata (the PR handler reads it for the
	// default PR title).
	meta := piece.PieceMetadata{
		Parent: "main",
		Issue: piece.IssueRef{
			Provider: "markdown",
			ID:       "issues/my-feature.md",
			Title:    "My Awesome Feature",
		},
	}
	metaData, _ := json.Marshal(meta)
	_ = fs.WriteFile(filepath.Join(worktreePath, ".monkeypuzzle", "piece-metadata.json"), metaData, 0644)

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

	result, err := handler.CreatePR(context.Background(), worktreePath, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 99 {
		t.Errorf("expected PR number 99, got %d", result.PRNumber)
	}

	// Verify issue ref was stored in metadata
	metadata, err := piece.ReadPRMetadata(worktreePath, fs)
	if err != nil {
		t.Fatalf("failed to read PR metadata: %v", err)
	}
	if metadata.Issue.ID != "issues/my-feature.md" {
		t.Errorf("metadata Issue.ID = %q, want 'issues/my-feature.md'", metadata.Issue.ID)
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

	// Minimal monkeypuzzle.json so pr.Handler resolves a github provider
	_ = fs.WriteFile(filepath.Join(mainRepoPath, ".monkeypuzzle/monkeypuzzle.json"), []byte(`{
  "version": "1",
  "project": {"name": "test"},
  "issues": {"provider": "markdown", "config": {"directory": "issues"}},
  "pr": {"provider": "github", "config": {}}
}`), 0644)

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

	result, err := handler.CreatePR(context.Background(), worktreePath, input)
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

	_, err := handler.CreatePR(context.Background(), workDir, input)
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

	_, err := handler.CreatePR(context.Background(), worktreePath, input)
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

	_, err := handler.CreatePR(context.Background(), worktreePath, input)
	if err == nil {
		t.Error("expected error when gh fails")
	}
}

func TestCreatePR_UsesParentBranchAsBase(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/child-piece"
	mainRepoPath := "/repo"

	// Create .monkeypuzzle directories
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)
	_ = fs.MkdirAll(filepath.Join(mainRepoPath, ".monkeypuzzle"), 0755)

	// Minimal monkeypuzzle.json so pr.Handler resolves a github provider
	_ = fs.WriteFile(filepath.Join(mainRepoPath, ".monkeypuzzle/monkeypuzzle.json"), []byte(`{
  "version": "1",
  "project": {"name": "test"},
  "issues": {"provider": "markdown", "config": {"directory": "issues"}},
  "pr": {"provider": "github", "config": {}}
}`), 0644)

	// Create piece metadata with parent piece
	pieceMetadata := piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece",
	}
	metadataData, _ := json.Marshal(pieceMetadata)
	_ = fs.WriteFile(filepath.Join(worktreePath, ".monkeypuzzle", "piece-metadata.json"), metadataData, 0644)

	// Mock git commands
	gitDir := filepath.Join(mainRepoPath, ".git", "worktrees", "child-piece")
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("child-piece\n"), nil)

	// Mock git push
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)

	// Mock gh pr create - should use parent-piece as base since no explicit base provided
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "Child Feature", "--body", "", "--base", "parent-piece"},
		[]byte("https://github.com/owner/repo/pull/50\n"), nil)

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	// Empty base - should auto-detect from parent metadata
	input := pr.Input{
		Title: "Child Feature",
		Body:  "",
		Base:  "", // Empty means auto-detect
	}

	result, err := handler.CreatePR(context.Background(), worktreePath, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 50 {
		t.Errorf("expected PR number 50, got %d", result.PRNumber)
	}
}

func TestCreatePR_ExplicitBaseOverridesParent(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/child-piece"
	mainRepoPath := "/repo"

	// Create .monkeypuzzle directories
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)
	_ = fs.MkdirAll(filepath.Join(mainRepoPath, ".monkeypuzzle"), 0755)

	// Minimal monkeypuzzle.json so pr.Handler resolves a github provider
	_ = fs.WriteFile(filepath.Join(mainRepoPath, ".monkeypuzzle/monkeypuzzle.json"), []byte(`{
  "version": "1",
  "project": {"name": "test"},
  "issues": {"provider": "markdown", "config": {"directory": "issues"}},
  "pr": {"provider": "github", "config": {}}
}`), 0644)

	// Create piece metadata with parent piece
	pieceMetadata := piece.PieceMetadata{
		Parent:            "parent-piece",
		CreatedFromBranch: "parent-piece",
	}
	metadataData, _ := json.Marshal(pieceMetadata)
	_ = fs.WriteFile(filepath.Join(worktreePath, ".monkeypuzzle", "piece-metadata.json"), metadataData, 0644)

	// Mock git commands
	gitDir := filepath.Join(mainRepoPath, ".git", "worktrees", "child-piece")
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("child-piece\n"), nil)

	// Mock git push
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)

	// Mock gh pr create - should use explicit base "main" despite parent metadata
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "Child Feature", "--body", "", "--base", "main"},
		[]byte("https://github.com/owner/repo/pull/51\n"), nil)

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	// Explicit base overrides parent
	input := pr.Input{
		Title: "Child Feature",
		Body:  "",
		Base:  "main", // Explicit override
	}

	result, err := handler.CreatePR(context.Background(), worktreePath, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 51 {
		t.Errorf("expected PR number 51, got %d", result.PRNumber)
	}
}

func TestCreatePR_MainParentUsesMain(t *testing.T) {
	fs := adapters.NewMemoryFS()
	mockExec := adapters.NewMockExec()
	output := adapters.NewBufferOutput()

	worktreePath := "/pieces/root-piece"
	mainRepoPath := "/repo"

	// Create .monkeypuzzle directories
	_ = fs.MkdirAll(filepath.Join(worktreePath, ".monkeypuzzle"), 0755)
	_ = fs.MkdirAll(filepath.Join(mainRepoPath, ".monkeypuzzle"), 0755)

	// Minimal monkeypuzzle.json so pr.Handler resolves a github provider
	_ = fs.WriteFile(filepath.Join(mainRepoPath, ".monkeypuzzle/monkeypuzzle.json"), []byte(`{
  "version": "1",
  "project": {"name": "test"},
  "issues": {"provider": "markdown", "config": {"directory": "issues"}},
  "pr": {"provider": "github", "config": {}}
}`), 0644)

	// Create piece metadata with main as parent (root piece)
	pieceMetadata := piece.PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "main",
	}
	metadataData, _ := json.Marshal(pieceMetadata)
	_ = fs.WriteFile(filepath.Join(worktreePath, ".monkeypuzzle", "piece-metadata.json"), metadataData, 0644)

	// Mock git commands
	gitDir := filepath.Join(mainRepoPath, ".git", "worktrees", "root-piece")
	mockExec.AddResponse("git", []string{"rev-parse", "--git-dir"}, []byte(gitDir+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--show-toplevel"}, []byte(worktreePath+"\n"), nil)
	mockExec.AddResponse("git", []string{"rev-parse", "--abbrev-ref", "HEAD"}, []byte("root-piece\n"), nil)

	// Mock git push
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)

	// Mock gh pr create - should use main as base
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "Root Feature", "--body", "", "--base", "main"},
		[]byte("https://github.com/owner/repo/pull/52\n"), nil)

	deps := core.Deps{
		FS:     fs,
		Output: output,
		Exec:   mockExec,
	}

	handler := pr.NewHandler(deps)

	// Empty base with main parent - should use main
	input := pr.Input{
		Title: "Root Feature",
		Body:  "",
		Base:  "", // Empty, will use parent which is "main"
	}

	result, err := handler.CreatePR(context.Background(), worktreePath, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 52 {
		t.Errorf("expected PR number 52, got %d", result.PRNumber)
	}
}

func TestWithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		input    pr.Input
		expected pr.Input
	}{
		{
			name:     "empty base stays empty for auto-detection",
			input:    pr.Input{Title: "Test", Body: "Body", Base: ""},
			expected: pr.Input{Title: "Test", Body: "Body", Base: ""}, // Handler auto-detects from piece parent
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
