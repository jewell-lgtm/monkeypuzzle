//go:build integration

package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestArgs_StrayPositionalsError pins the Args validators: commands that take
// no positionals must reject them loudly instead of silently ignoring them
// (the old behavior made `mp abandon my-piece` abandon the *current* piece).
func TestArgs_StrayPositionalsError(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	for _, tc := range [][]string{
		{"create", "stray-arg"},
		{"update", "stray-arg"},
		{"merge", "stray-arg"},
		{"cleanup", "stray-arg"},
		{"list", "stray-arg"},
		{"flatten", "stray-arg"},
		{"sync", "stray-arg"},
		{"go", "stray-arg"},
		{"pr", "create", "stray-arg"},
		{"pr", "ready", "stray-arg"},
		{"stack", "status", "stray-arg"},
	} {
		cmd := exec.Command(e.binPath, tc...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("mp %v should reject the stray positional, got success\n%s", tc, out)
			continue
		}
		if !strings.Contains(string(out), "unknown command") && !strings.Contains(string(out), "accepts") {
			t.Errorf("mp %v should fail with an args error, got: %s", tc, out)
		}
	}
}

// TestSelectors_StatusDoneAbandonByName pins the positional/--piece selector on
// status, done, and abandon: each targets a named piece from the repo root
// (not just from inside the piece).
func TestSelectors_StatusDoneAbandonByName(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")

	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")

	// status <piece> from the repo root reports the piece, not "main repo".
	statusOut := mpRun(t, e, repo, dataDir, "status", "fix-x")
	if !strings.Contains(statusOut, `"piece_name": "fix-x"`) {
		t.Errorf("mp status fix-x should report the piece, got: %s", statusOut)
	}
	// --piece works too, and conflicting selectors error.
	statusOut = mpRun(t, e, repo, dataDir, "status", "--piece", "fix-x")
	if !strings.Contains(statusOut, `"piece_name": "fix-x"`) {
		t.Errorf("mp status --piece fix-x should report the piece, got: %s", statusOut)
	}
	cmd := exec.Command(e.binPath, "status", "fix-x", "--piece", "other")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("conflicting selectors should error, got success\n%s", out)
	}

	// A named missing piece is a clear error.
	cmd = exec.Command(e.binPath, "status", "no-such-piece")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Errorf("status on a missing piece should error, got success\n%s", out)
	} else if !strings.Contains(string(out), "not found") {
		t.Errorf("missing-piece error should say not found, got: %s", out)
	}

	// abandon <piece> from the repo root removes it.
	mpRun(t, e, repo, dataDir, "abandon", "fix-x", "--force")
	if _, err := os.Stat(filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")); !os.IsNotExist(err) {
		t.Errorf("abandon fix-x should remove the worktree")
	}

	// done <piece>: create → merge → done by name from the repo root.
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-y", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-y")
	if err := os.WriteFile(filepath.Join(wt, "y.txt"), []byte("y"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	gitCmd(t, wt, "add", "y.txt")
	gitCmd(t, wt, "commit", "-m", "feat: y")
	mpRunIn(t, e, wt, dataDir, "merge")
	doneOut := mpRun(t, e, repo, dataDir, "done", "fix-y")
	if !strings.Contains(doneOut, "fix-y") {
		t.Errorf("done fix-y output should mention the piece, got: %s", doneOut)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("done fix-y should remove the worktree")
	}
}

// mpRunIn is mpRun but tolerant of the command needing to run in a worktree
// dir (identical; named for readability at call sites).
func mpRunIn(t *testing.T, e *testEnv, dir, dataDir string, args ...string) string {
	t.Helper()
	return mpRun(t, e, dir, dataDir, args...)
}

// TestSelectors_ProjectRemovePositionalOnly pins `mp project remove <target>`:
// the positional is the only flagless form (the old --target flag is gone).
func TestSelectors_ProjectRemovePositionalOnly(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	_ = projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	cmd := exec.Command(e.binPath, "project", "remove", "--target", "alpha")
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("--target should be gone, got success\n%s", out)
	}

	out := mpRun(t, e, e.tmpDir, dataDir, "project", "remove", "alpha")
	if !strings.Contains(out, "alpha") {
		t.Errorf("project remove alpha should report the removal, got: %s", out)
	}
}

// TestSelectors_BlankPositionalErrors pins the fix for a bug Sol's review
// caught: an explicitly-given-but-blank positional (whitespace only) must
// error, not silently fall back to "the piece you're standing in" — that
// silent fallback would make `mp abandon "   " --force` abandon whatever
// piece the caller happens to be in in, discarding the malformed input
// instead of rejecting it.
func TestSelectors_BlankPositionalErrors(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	// From inside fix-x, a blank positional must error rather than silently
	// abandoning fix-x (the piece the caller happens to be standing in).
	cmd := exec.Command(e.binPath, "abandon", "   ", "--force")
	cmd.Dir = wt
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("blank positional should error, got success\n%s", out)
	}
	if _, statErr := os.Stat(wt); os.IsNotExist(statErr) {
		t.Errorf("fix-x should NOT have been abandoned by a blank positional, but it's gone")
	}
}

// TestSelectors_NameFlagConflictsWithPositional pins the fix for another Sol
// finding: the deprecated --name alias must participate in the same conflict
// check as --piece, not silently lose to a different positional/--piece value.
func TestSelectors_NameFlagConflictsWithPositional(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-y", "--skip-switch")

	cmd := exec.Command(e.binPath, "abandon", "fix-x", "--name", "fix-y", "--force")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("conflicting positional vs --name should error, got success\n%s", out)
	}
	if !strings.Contains(string(out), "conflicting piece selectors") {
		t.Errorf("expected a conflicting-selectors error, got: %s", out)
	}
	// Neither piece should have been touched.
	for _, name := range []string{"fix-x", "fix-y"} {
		if _, statErr := os.Stat(filepath.Join(repo, ".monkeypuzzle", "pieces", name)); statErr != nil {
			t.Errorf("piece %s should still exist after the rejected conflicting call", name)
		}
	}
}

// TestResolvePieceWorkDir_RejectsPathTraversal pins the hardening fix: a
// selector containing a path separator or "." / ".." must be rejected before
// it's joined into piecesDir, so `mp done ../../etc` (or any traversal
// attempt) can never resolve outside the pieces directory.
func TestResolvePieceWorkDir_RejectsPathTraversal(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	for _, selector := range []string{"..", "../escape", "a/b", "."} {
		cmd := exec.Command(e.binPath, "status", selector)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Errorf("selector %q should be rejected, got success\n%s", selector, out)
		}
		if !strings.Contains(string(out), "invalid piece name") {
			t.Errorf("selector %q: expected an 'invalid piece name' error, got: %s", selector, out)
		}
	}
}

// TestProjectList_RejectsStrayArgs pins the fix for the one command Sol found
// missing an Args validator in the "Args validators everywhere" pass.
func TestProjectList_RejectsStrayArgs(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	_ = projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	cmd := exec.Command(e.binPath, "project", "list", "stray-arg")
	cmd.Dir = e.tmpDir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("project list should reject a stray positional, got success\n%s", out)
	}
}
