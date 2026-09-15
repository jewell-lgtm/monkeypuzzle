//go:build integration

package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mpRunCwdFile runs mp with MP_CWD_FILE pointed at a fresh file and returns
// what mp wrote there — the directory the `mp shell-init` wrapper would cd
// into. Empty means mp asked for no move.
func mpRunCwdFile(t *testing.T, e *testEnv, dir, dataDir string, args ...string) (noted, out string) {
	t.Helper()
	cwdFile := filepath.Join(t.TempDir(), "cwd")
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir, "MP_CWD_FILE="+cwdFile)
	stdout, err := cmd.Output()
	if err != nil {
		t.Fatalf("mp %v in %s: %v", args, dir, err)
	}
	b, readErr := os.ReadFile(cwdFile)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read cwd file: %v", readErr)
	}
	return strings.TrimSpace(string(b)), string(stdout)
}

// TestCwdHandoff_Create pins the create hand-off: with no multiplexer there is
// no session to attach, so the new worktree is where the caller should end up.
// A shell wrapper reads it from MP_CWD_FILE; a human reads it off stdout.
func TestCwdHandoff_Create(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	noted, _ := mpRunCwdFile(t, e, repo, dataDir, "create", "--name", "fix-x")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	if noted != wt {
		t.Errorf("create should hand off the new worktree %q, got %q", wt, noted)
	}
}

// TestCwdHandoff_CreateSkipSwitch pins the other half: --skip-switch means
// "leave me where I am", so nothing is handed off.
func TestCwdHandoff_CreateSkipSwitch(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	noted, _ := mpRunCwdFile(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	if noted != "" {
		t.Errorf("--skip-switch should hand off nothing, got %q", noted)
	}
}

// TestCwdHandoff_Switch pins `mp switch <piece>`: the worktree path, both on
// stdout (the `cd "$(mp switch x)"` contract) and in the cwd file.
func TestCwdHandoff_Switch(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	noted, out := mpRunCwdFile(t, e, repo, dataDir, "switch", "fix-x")
	if noted != wt {
		t.Errorf("switch should hand off %q, got %q", wt, noted)
	}
	if strings.TrimSpace(out) != wt {
		t.Errorf("switch stdout should be the bare path %q, got %q", wt, out)
	}
}

// TestCwdHandoff_DoneFromInsidePiece pins the strand fix: finishing the piece
// you are standing in deletes that directory, so mp hands back the main repo
// root instead of leaving the shell in a removed worktree.
func TestCwdHandoff_DoneFromInsidePiece(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	noted, _ := mpRunCwdFile(t, e, wt, dataDir, "done", "--force")
	if noted != repo {
		t.Errorf("done from inside the piece should hand back the repo root %q, got %q", repo, noted)
	}
}

// TestCwdHandoff_DoneFromMainRepo pins the converse: finishing a piece from
// somewhere else leaves the caller's directory alone.
func TestCwdHandoff_DoneFromMainRepo(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")

	noted, _ := mpRunCwdFile(t, e, repo, dataDir, "done", "--piece", "fix-x", "--force")
	if noted != "" {
		t.Errorf("done from the main repo should hand off nothing, got %q", noted)
	}
}

// TestCwdHandoff_AbandonFromInsidePiece pins the same for abandon.
func TestCwdHandoff_AbandonFromInsidePiece(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	noted, _ := mpRunCwdFile(t, e, wt, dataDir, "abandon", "--piece", "fix-x", "--force")
	if noted != repo {
		t.Errorf("abandon from inside the piece should hand back %q, got %q", repo, noted)
	}
}

// TestCwdHandoff_Unset pins the default: with no MP_CWD_FILE mp writes nothing
// anywhere and assumes nothing about the caller's shell.
func TestCwdHandoff_Unset(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	out := mpRun(t, e, repo, dataDir, "create", "--name", "fix-x")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	if !strings.Contains(out, wt) {
		t.Errorf("create should still report the worktree path, got: %q", out)
	}
}
