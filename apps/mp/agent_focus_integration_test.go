//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// reportAgent seeds a live agent record on the piece at wt via `mp agent
// report`, run from inside the piece worktree (report is cwd-scoped). The
// liveness filter reaps records whose PID isn't a real running process, so
// this uses the test binary's own PID (guaranteed alive for the test's
// duration) rather than an arbitrary number.
func reportAgent(t *testing.T, e *testEnv, wt, dataDir, id, status string) {
	t.Helper()
	mpRun(t, e, wt, dataDir, "agent", "report", "--id", id, "--kind", "claude", "--status", status, "--pid", strconv.Itoa(os.Getpid()), "--pane", "%1")
}

// TestAgentFocus_ByPieceName_FallsBackToSwitch pins the no-live-session
// fallback: with the test harness's multiplexer set to "none" (setupTestEnv's
// default), the agent's session never exists, so `mp agent focus <piece>`
// falls back to a plain piece switch. With no multiplexer to attach to, that
// switch prints only the bare worktree path (the `cd $(mp switch ...)`
// contract) — and, critically, nothing else: printing JSON on top of that
// path would be a double-print bug (regression-tested here).
func TestAgentFocus_ByPieceName_FallsBackToSwitch(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	reportAgent(t, e, wt, dataDir, "agent-1", "working")

	out := strings.TrimSpace(mpRun(t, e, repo, dataDir, "agent", "focus", "fix-x"))
	if out != wt {
		t.Errorf("expected the bare worktree path %q and nothing else, got: %q", wt, out)
	}
}

// TestAgentFocus_ByAgentID resolves by agent id instead of piece name.
func TestAgentFocus_ByAgentID(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	reportAgent(t, e, wt, dataDir, "agent-1", "working")

	out := strings.TrimSpace(mpRun(t, e, repo, dataDir, "agent", "focus", "agent-1"))
	if out != wt {
		t.Errorf("expected to resolve fix-x's worktree by agent id, got: %q", out)
	}
}

// TestAgentFocus_Blocked_PicksMostUrgent pins --blocked: it must pick the
// blocked agent over a working one, regardless of positional order.
func TestAgentFocus_Blocked_PicksMostUrgent(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-y", "--skip-switch")
	wtX := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	wtY := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-y")
	reportAgent(t, e, wtX, dataDir, "agent-x", "working")
	reportAgent(t, e, wtY, dataDir, "agent-y", "blocked")

	out := strings.TrimSpace(mpRun(t, e, repo, dataDir, "agent", "focus", "--blocked"))
	if out != wtY {
		t.Errorf("--blocked should pick the blocked agent's worktree (fix-y), got: %q", out)
	}
}

// TestAgentFocus_Blocked_NoneLive pins the no-blocked-agents case: exit 0,
// a warning on stderr, and no stdout JSON (nothing to report).
func TestAgentFocus_Blocked_NoneLive(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	reportAgent(t, e, wt, dataDir, "agent-1", "working")

	cmd := exec.Command(e.binPath, "agent", "focus", "--blocked")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	stdout, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("agent focus --blocked with none blocked should exit 0: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(string(stdout)) != "" {
		t.Errorf("no blocked agents: stdout should be empty, got: %q", stdout)
	}
}

// TestAgentFocus_NoSelectorErrors pins the "pass one or use --blocked"
// requirement: no positional and no --blocked must error, not silently pick
// something.
func TestAgentFocus_NoSelectorErrors(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")

	cmd := exec.Command(e.binPath, "agent", "focus")
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), "MP_DATA_DIR="+dataDir, "MP_CONFIG_DIR="+e.configDir)
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Errorf("agent focus with no selector should error, got success\n%s", out)
	}
}

// TestAgentList_IconField pins the single-sourced icon field: `mp agent list
// --json` items carry an icon matching their status (blocked -> the same
// glyph as mp agent summary's 🔴).
func TestAgentList_IconField(t *testing.T) {
	e := setupTestEnv(t)
	defer e.cleanup()

	dataDir := filepath.Join(e.tmpDir, "data")
	repo := projectTestRepo(t, e, dataDir, filepath.Join(e.tmpDir, "repos"), "alpha")
	gitCmd(t, repo, "add", ".claude")
	gitCmd(t, repo, "commit", "-m", "chore: claude")
	mpRun(t, e, repo, dataDir, "create", "--name", "fix-x", "--skip-switch")
	wt := filepath.Join(repo, ".monkeypuzzle", "pieces", "fix-x")
	reportAgent(t, e, wt, dataDir, "agent-1", "blocked")

	out, _ := mpJSON(t, e, repo, dataDir, "agent", "list", "--json")
	var result struct {
		Agents []struct {
			Status string `json:"status"`
			Icon   string `json:"icon"`
		} `json:"agents"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out)
	}
	if len(result.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d: %+v", len(result.Agents), result.Agents)
	}
	if result.Agents[0].Icon != "🔴" {
		t.Errorf("expected blocked icon 🔴, got %q", result.Agents[0].Icon)
	}
}
