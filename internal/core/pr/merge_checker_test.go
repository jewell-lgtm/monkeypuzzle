package pr_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// TestNewMergeChecker_ReadsConfigFromMainRepoRoot_WhenCalledFromWorktree pins the
// fix for cleanup/done missing squash-merged PRs. cleanup resolves merge status
// from the *piece worktree* path, but monkeypuzzle.json lives in the main working
// tree's gitignored .monkeypuzzle/ dir — absent from a linked worktree's checkout.
// If the factory reads config from the worktree it finds nothing and (previously)
// returned nil, disabling the forge-based detection that is the only thing able to
// see a multi-commit squash-merge.
//
// The main-root config declares the gitlab provider, so the resolved checker is a
// *GitLabProvider only when config was read from the main root. Reading from the
// worktree (the bug) would miss it and fall back to the github default.
func TestNewMergeChecker_ReadsConfigFromMainRepoRoot_WhenCalledFromWorktree(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	mainRoot := t.TempDir()

	runGit(t, mainRoot, "init", "-b", "main")
	runGit(t, mainRoot, "config", "user.email", "test@example.com")
	runGit(t, mainRoot, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(mainRoot, "test.txt"), []byte("initial"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, mainRoot, "add", ".")
	runGit(t, mainRoot, "commit", "-m", "initial commit")

	// Config lives only at the main root, and declares gitlab so we can prove the
	// checker read it from here (and not from the worktree, where it is absent).
	mpDir := filepath.Join(mainRoot, ".monkeypuzzle")
	if err := os.MkdirAll(mpDir, 0o755); err != nil {
		t.Fatalf("mkdir .monkeypuzzle: %v", err)
	}
	config := `{"version":"1","project":{"name":"test"},"pr":{"provider":"gitlab","config":{}}}`
	if err := os.WriteFile(filepath.Join(mpDir, "monkeypuzzle.json"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// A real linked worktree under .monkeypuzzle/pieces/<name>, mirroring how mp
	// lays pieces out. Its checkout does not contain .monkeypuzzle/monkeypuzzle.json.
	worktreePath := filepath.Join(mpDir, "pieces", "feature")
	runGit(t, mainRoot, "worktree", "add", "-b", "feature", worktreePath)
	if _, err := os.Stat(filepath.Join(worktreePath, ".monkeypuzzle", "monkeypuzzle.json")); !os.IsNotExist(err) {
		t.Fatalf("precondition: worktree unexpectedly has its own config (err=%v)", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewBufferOutput(),
		Exec:   adapters.NewOSExec(),
	}

	mc := pr.NewMergeChecker(worktreePath, deps)
	if mc == nil {
		t.Fatal("NewMergeChecker returned nil for a piece worktree — PR-based merge detection would be skipped")
	}
	if _, ok := mc.(*pr.GitLabProvider); !ok {
		t.Fatalf("expected *pr.GitLabProvider (config resolved from main repo root), got %T — config was not read from the main root", mc)
	}
}
