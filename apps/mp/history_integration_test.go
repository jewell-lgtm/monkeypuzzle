//go:build integration

package main_test

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
)

// TestCLI_History_RecordsCreateAndDone drives a piece through create → done
// and reads the log back through `mp history`. The env var reaches the child
// binary via os.Environ() (see testEnv.env).
func TestCLI_History_RecordsCreateAndDone(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	t.Setenv(history.EnvFile, filepath.Join(env.tmpDir, "history.jsonl"))

	env.initGitRepo()
	env.initProject("histproj")

	stdout, stderr, err := env.run("create", "--name", "hist-piece", "--skip-switch")
	if err != nil {
		t.Fatalf("create failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	var created map[string]any
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("invalid JSON from create: %v", err)
	}
	worktreePath := created["worktree_path"].(string)

	cmd := exec.Command("git", "merge", "hist-piece", "--no-edit")
	cmd.Dir = env.tmpDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git merge failed: %v\n%s", err, output)
	}
	if stdout, stderr, err = env.runInDirWithStdin(worktreePath, "{}", "done"); err != nil {
		t.Fatalf("done failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Non-TTY → JSON lines on stdout, oldest first.
	stdout, stderr, err = env.run("history", "--project", "histproj")
	if err != nil {
		t.Fatalf("history failed: %v\nstderr: %s", err, stderr)
	}
	events := parseHistoryLines(t, stdout)
	if len(events) != 2 || events[0].Event != "piece.created" || events[1].Event != "piece.done" {
		t.Fatalf("want [piece.created piece.done], got %+v\nstderr: %s", events, stderr)
	}
	for _, ev := range events {
		if ev.Piece != "hist-piece" || ev.Project != "histproj" || ev.Actor.Kind == "" {
			t.Errorf("unexpected event: %+v", ev)
		}
	}

	// Filters: --event glob and --limit.
	stdout, _, err = env.run("history", "--event", "piece.d*", "-n", "1")
	if err != nil {
		t.Fatal(err)
	}
	events = parseHistoryLines(t, stdout)
	if len(events) != 1 || events[0].Event != "piece.done" {
		t.Fatalf("filtered: got %+v", events)
	}

	// A wrong project yields nothing on stdout and exits 0.
	stdout, _, err = env.run("history", "--project", "nope")
	if err != nil || strings.TrimSpace(stdout) != "" {
		t.Fatalf("empty result: stdout=%q err=%v", stdout, err)
	}
}

func parseHistoryLines(t *testing.T, stdout string) []history.Event {
	t.Helper()
	var events []history.Event
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		var ev history.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("stdout line is not JSON: %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}
