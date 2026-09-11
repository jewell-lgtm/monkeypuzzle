//go:build integration

package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// mpRunAllowFail runs mp and returns stdout, stderr and whether it succeeded.
func mpRunAllowFail(t *testing.T, e *testEnv, dir, dataDir string, args ...string) (string, string, bool) {
	t.Helper()
	cmd := exec.Command(e.binPath, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err == nil
}

// TestOpen_RunsTheOpenerWithPlaceholders pins the opener contract: the
// template runs with {path} and {piece} filled in, from the worktree.
func TestOpen_RunsTheOpenerWithPlaceholders(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	marker := filepath.Join(t.TempDir(), "opened")
	with := fmt.Sprintf("printf '%%s %%s' {piece} {path} > %s", marker)
	mpRun(t, e, repo, dataDir, "open", "fix-x", "--with", with)

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("opener did not run: %v", err)
	}
	if want := "fix-x " + wt; string(got) != want {
		t.Errorf("opener got %q, want %q", got, want)
	}
}

// TestOpen_BareCommandGetsThePath pins the convenience: a template with no
// placeholder is handed the path as its final argument.
func TestOpen_BareCommandGetsThePath(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	marker := filepath.Join(t.TempDir(), "args")
	stdout, _, ok := mpRunAllowFail(t, e, repo, dataDir, "open", "fix-x", "--with", "printf '%s' ", "--json")
	_ = stdout
	if !ok {
		t.Fatal("open with a bare command should succeed")
	}
	// Prove the same thing through the recorded command in --json output.
	out, _, _ := mpRunAllowFail(t, e, repo, dataDir, "open", "fix-x", "--with", "true", "--json")
	var res struct {
		Command string `json:"command"`
		Opened  bool   `json:"opened"`
		Path    string `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("parse open json: %v\n%s", err, out)
	}
	if !strings.HasSuffix(res.Command, "'"+wt+"'") {
		t.Errorf("bare command should get the quoted path appended, got %q", res.Command)
	}
	if !res.Opened || res.Path != wt {
		t.Errorf("unexpected result: %+v", res)
	}
	_ = marker
}

// TestOpen_NoOpenerPrintsPath pins the assume-nothing default: with nothing
// configured, mp reports the path and how to set an opener, and still exits 0.
func TestOpen_NoOpenerPrintsPath(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")

	stdout, stderr, ok := mpRunAllowFail(t, e, repo, dataDir, "open", "fix-x")
	if !ok {
		t.Fatalf("open without an opener should still succeed; stderr: %s", stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), wt) {
		t.Errorf("expected the worktree path on stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "open_command") {
		t.Errorf("expected guidance on setting an opener, got %q", stderr)
	}
}

// TestOpen_NeverCreates pins the difference from switch: open resolves, but
// does not mint a piece for an unknown name.
func TestOpen_NeverCreates(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	_, stderr, ok := mpRunAllowFail(t, e, repo, dataDir, "open", "no-such-thing", "--with", "true")
	if ok {
		t.Error("open should fail on a name that matches nothing")
	}
	if !strings.Contains(stderr, "mp create") {
		t.Errorf("expected a pointer to mp create, got %q", stderr)
	}
}

// TestOpen_ConfigKeyIsUsed pins open_command as the configured default.
func TestOpen_ConfigKeyIsUsed(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")

	marker := filepath.Join(t.TempDir(), "opened")
	mpRun(t, e, repo, dataDir, "config", "set", "open_command", fmt.Sprintf("printf '%%s' {piece} > %s", marker))
	mpRun(t, e, repo, dataDir, "open", "fix-x")

	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("configured opener did not run: %v", err)
	}
	if string(got) != "fix-x" {
		t.Errorf("configured opener got %q", got)
	}
}

// TestDoctor_ReportsSetup pins the local doctor: it reports the multiplexer,
// the shell wrapper and the opener without changing anything.
func TestDoctor_ReportsSetup(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	out, _, ok := mpRunAllowFail(t, e, repo, dataDir, "doctor", "--json")
	if !ok {
		t.Fatal("doctor should exit 0")
	}
	var report struct {
		Checks []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("parse doctor json: %v\n%s", err, out)
	}
	byName := map[string]string{}
	for _, c := range report.Checks {
		byName[c.Name] = c.Status + ": " + c.Detail
	}
	for _, want := range []string{"multiplexer", "shell-init", "open_command", "project"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("doctor should report %q; got %v", want, byName)
		}
	}
	if !strings.Contains(byName["multiplexer"], "none") {
		t.Errorf("test env configures multiplexer none, doctor said %q", byName["multiplexer"])
	}
}
