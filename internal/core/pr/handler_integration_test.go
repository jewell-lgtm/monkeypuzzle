//go:build integration

package pr_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
)

// Integration tests for PR command using real git and filesystem.
// gh commands are mocked since we can't create real PRs.
// Run with: go test -tags=integration ./internal/core/pr/...

// setupTestRepo creates a real git repo with a worktree for testing
func setupTestRepo(t *testing.T) (repoDir, worktreeDir string, cleanup func()) {
	t.Helper()

	// Create temp directory for repo
	repoDir, err := os.MkdirTemp("", "mp-pr-test-repo-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// Create temp directory for worktrees
	worktreeBase, err := os.MkdirTemp("", "mp-pr-test-wt-*")
	if err != nil {
		os.RemoveAll(repoDir)
		t.Fatalf("failed to create worktree dir: %v", err)
	}

	cleanup = func() {
		os.RemoveAll(repoDir)
		os.RemoveAll(worktreeBase)
	}

	// Initialize git repo
	cmd := exec.Command("git", "init")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure git
	for _, args := range [][]string{
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test User"},
	} {
		cmd = exec.Command("git", args...)
		cmd.Dir = repoDir
		if out, err := cmd.CombinedOutput(); err != nil {
			cleanup()
			t.Fatalf("git config failed: %v\n%s", err, out)
		}
	}

	// Create initial commit
	testFile := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0644); err != nil {
		cleanup()
		t.Fatalf("failed to write file: %v", err)
	}

	cmd = exec.Command("git", "add", ".")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		t.Fatalf("git add failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", "initial")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "branch", "-M", "main")
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		t.Fatalf("git branch failed: %v\n%s", err, out)
	}

	// Create .monkeypuzzle directory in repo + minimal config so pr.Handler
	// can resolve a provider via the registry.
	if err := os.MkdirAll(filepath.Join(repoDir, ".monkeypuzzle"), 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create .monkeypuzzle: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".monkeypuzzle/monkeypuzzle.json"), []byte(`{
  "version": "1",
  "project": {"name": "test"},
  "issues": {"provider": "markdown", "config": {"directory": "issues"}},
  "pr": {"provider": "github", "config": {}}
}`), 0644); err != nil {
		cleanup()
		t.Fatalf("failed to write monkeypuzzle.json: %v", err)
	}

	// Create worktree
	worktreeDir = filepath.Join(worktreeBase, "test-piece")
	cmd = exec.Command("git", "worktree", "add", "-b", "test-piece", worktreeDir)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		t.Fatalf("git worktree add failed: %v\n%s", err, out)
	}

	// Create .monkeypuzzle directory in worktree
	if err := os.MkdirAll(filepath.Join(worktreeDir, ".monkeypuzzle"), 0755); err != nil {
		cleanup()
		t.Fatalf("failed to create worktree .monkeypuzzle: %v", err)
	}

	return repoDir, worktreeDir, cleanup
}

func TestIntegration_CreatePR_HappyPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	_, worktreeDir, cleanup := setupTestRepo(t)
	defer cleanup()

	// Create mock exec that handles git commands for real, but mocks gh
	mockExec := adapters.NewMockExec()

	// Mock push and gh pr create
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "Test PR", "--body", "Description", "--base", "main"},
		[]byte("https://github.com/test/repo/pull/42\n"), nil)

	// Use hybrid exec: real git for queries, mock for push/gh
	hybridExec := &hybridExec{
		real: adapters.NewOSExec(),
		mock: mockExec,
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   hybridExec,
	}

	handler := pr.NewHandler(deps)

	input := pr.Input{
		Title: "Test PR",
		Body:  "Description",
		Base:  "main",
	}

	result, err := handler.CreatePR(context.Background(), worktreeDir, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 42 {
		t.Errorf("PR number = %d, want 42", result.PRNumber)
	}
	if result.Branch != "test-piece" {
		t.Errorf("branch = %q, want 'test-piece'", result.Branch)
	}

	// Verify metadata was written to real filesystem
	metadata, err := piece.ReadPRMetadata(worktreeDir, adapters.NewOSFS(""))
	if err != nil {
		t.Fatalf("failed to read PR metadata: %v", err)
	}
	if metadata.PRNumber != 42 {
		t.Errorf("metadata PRNumber = %d, want 42", metadata.PRNumber)
	}
	if metadata.Branch != "test-piece" {
		t.Errorf("metadata branch = %q, want 'test-piece'", metadata.Branch)
	}
}

func TestIntegration_CreatePR_WithIssueMarker(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	_, worktreeDir, cleanup := setupTestRepo(t)
	defer cleanup()

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
	metaPath := filepath.Join(worktreeDir, ".monkeypuzzle", "piece-metadata.json")
	if err := os.WriteFile(metaPath, metaData, 0644); err != nil {
		t.Fatalf("failed to write piece metadata: %v", err)
	}

	mockExec := adapters.NewMockExec()
	mockExec.AddResponse("git", []string{"push", "-u", "origin", "HEAD"}, []byte(""), nil)
	// Note: title should come from the recorded issue ref since input.Title is empty
	mockExec.AddResponse("gh", []string{"pr", "create", "--title", "My Awesome Feature", "--body", "", "--base", "main"},
		[]byte("https://github.com/test/repo/pull/99\n"), nil)

	hybridExec := &hybridExec{
		real: adapters.NewOSExec(),
		mock: mockExec,
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   hybridExec,
	}

	handler := pr.NewHandler(deps)

	// No title - should use issue marker
	input := pr.Input{
		Title: "",
		Body:  "",
		Base:  "main",
	}

	result, err := handler.CreatePR(context.Background(), worktreeDir, input)
	if err != nil {
		t.Fatalf("CreatePR failed: %v", err)
	}

	if result.PRNumber != 99 {
		t.Errorf("PR number = %d, want 99", result.PRNumber)
	}

	// Verify issue path stored in metadata
	metadata, err := piece.ReadPRMetadata(worktreeDir, adapters.NewOSFS(""))
	if err != nil {
		t.Fatalf("failed to read PR metadata: %v", err)
	}
	if metadata.Issue.ID != "issues/my-feature.md" {
		t.Errorf("metadata Issue.ID = %q, want 'issues/my-feature.md'", metadata.Issue.ID)
	}
	if metadata.Issue.Title != "My Awesome Feature" {
		t.Errorf("metadata Issue.Title = %q, want 'My Awesome Feature'", metadata.Issue.Title)
	}
}

func TestIntegration_CreatePR_NotInWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a regular git repo (not a worktree)
	tmpDir, err := os.MkdirTemp("", "mp-pr-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cmd := exec.Command("git", "init")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v\n%s", err, out)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}

	handler := pr.NewHandler(deps)

	input := pr.Input{
		Title: "Test",
		Base:  "main",
	}

	_, err = handler.CreatePR(context.Background(), tmpDir, input)
	if err == nil {
		t.Error("expected error when not in worktree")
	}
}

// hybridExec uses real exec for git queries but mocks push/gh commands
type hybridExec struct {
	real *adapters.OSExec
	mock *adapters.MockExec
}

func (h *hybridExec) shouldMock(name string, args []string) bool {
	if name == "gh" {
		return true
	}
	if name == "git" && len(args) > 0 && args[0] == "push" {
		return true
	}
	return false
}

func (h *hybridExec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if h.shouldMock(name, args) {
		return h.mock.Run(ctx, name, args...)
	}
	return h.real.Run(ctx, name, args...)
}

func (h *hybridExec) RunWithDir(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if h.shouldMock(name, args) {
		return h.mock.RunWithDir(ctx, dir, name, args...)
	}
	return h.real.RunWithDir(ctx, dir, name, args...)
}

func (h *hybridExec) RunWithEnv(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, error) {
	if h.shouldMock(name, args) {
		return h.mock.RunWithEnv(ctx, dir, env, name, args...)
	}
	return h.real.RunWithEnv(ctx, dir, env, name, args...)
}

func (h *hybridExec) StartDetached(dir string, env []string, logPath, name string, args ...string) error {
	if h.shouldMock(name, args) {
		return h.mock.StartDetached(dir, env, logPath, name, args...)
	}
	return h.real.StartDetached(dir, env, logPath, name, args...)
}
