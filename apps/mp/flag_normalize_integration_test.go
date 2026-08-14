//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFlatten_DryRunByDefault pins the sweep model: a non-interactive
// `mp flatten` previews and removes nothing; `--apply` (flag or stdin) removes.
func TestFlatten_DryRunByDefault(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	// Bare flatten (non-TTY): preview only, worktree survives.
	out := mpRun(t, e, repo, dataDir, "flatten")
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("bare flatten must not remove worktrees (dry-run default): stat: %v\n%s", err, out)
	}

	// --apply removes it.
	out = mpRun(t, e, repo, dataDir, "flatten", "--apply")
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Errorf("flatten --apply should remove the worktree\n%s", out)
	}

	// stdin {"apply":true} removes a fresh one.
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-y", "--skip-switch")
	wtY := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-y")
	cmd := exec.Command(e.binPath, "flatten")
	cmd.Dir = repo
	cmd.Stdin = strings.NewReader(`{"apply":true}`)
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if raw, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("flatten apply-over-stdin: %v\n%s", err, raw)
	}
	if _, err := os.Stat(wtY); !os.IsNotExist(err) {
		t.Errorf("flatten with stdin apply:true should remove the worktree")
	}
}

// TestMainFlag_CanonicalAndAlias pins the --main rename: --main is canonical,
// --main-branch still works (deprecated), and stdin accepts both "main" and
// the legacy "main_branch" key.
func TestMainFlag_CanonicalAndAlias(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	// update --main works; update --main-branch still works (deprecation warning
	// on stderr is fine).
	for _, flag := range []string{"--main", "--main-branch"} {
		cmd := exec.Command(e.binPath, "update", flag, "main")
		cmd.Dir = wt
		cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("update %s main failed: %v\n%s", flag, err, out)
		}
	}

	// stdin with the canonical "main" key.
	cmd := exec.Command(e.binPath, "update")
	cmd.Dir = wt
	cmd.Stdin = strings.NewReader(`{"main":"main"}`)
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("update with stdin main key failed: %v\n%s", out, err)
	}

	// The schema advertises the canonical key.
	schemaOut := mpRun(t, e, repo, dataDir, "update", "--schema")
	var schema map[string]any
	if err := json.Unmarshal([]byte(schemaOut), &schema); err != nil {
		t.Fatalf("update --schema not JSON: %v\n%s", err, schemaOut)
	}
	if _, ok := schema["main"]; !ok {
		t.Errorf("update --schema should use the canonical 'main' key, got: %s", schemaOut)
	}
	if _, ok := schema["main_branch"]; ok {
		t.Errorf("update --schema should not advertise the deprecated key, got: %s", schemaOut)
	}
}

// TestCleanup_ForceRemoved pins the --force removal on cleanup.
func TestCleanup_ForceRemoved(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	cmd := exec.Command(e.binPath, "cleanup", "--force")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("cleanup --force should be gone, got success\n%s", out)
	}
}

// TestPRReady_Quad pins the introspection quad on pr ready: --schema prints an
// (empty) shape and stdin JSON is accepted, so agents can treat every command
// uniformly.
func TestPRReady_Quad(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	out := mpRun(t, e, repo, dataDir, "pr", "ready", "--schema")
	var schema map[string]any
	if err := json.Unmarshal([]byte(out), &schema); err != nil {
		t.Errorf("pr ready --schema should print JSON, got: %s", out)
	}

	// stdin {} is accepted; the command then fails on the missing PR metadata,
	// not on the input handling.
	cmd := exec.Command(e.binPath, "pr", "ready")
	cmd.Dir = repo
	cmd.Stdin = strings.NewReader(`{}`)
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	raw, err := cmd.CombinedOutput()
	if err == nil {
		t.Skip("unexpected pr-ready success (forge configured?)")
	}
	if strings.Contains(string(raw), "invalid JSON") {
		t.Errorf("pr ready should accept stdin {}, got: %s", raw)
	}
}
