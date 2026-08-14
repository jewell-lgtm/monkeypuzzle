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
