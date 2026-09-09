//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/inbox"
)

// TestCLI_Inbox_RanksPiecesAcrossProjects creates two pieces and reads them
// back through `mp inbox --json`, first with derived ranks, then with a
// hand-written inbox.json order. MP_CONFIG_DIR (testEnv.env) isolates the
// state file alongside the user config.
func TestCLI_Inbox_RanksPiecesAcrossProjects(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	env.initGitRepo()
	env.initProject("inboxproj")
	// Pin the worktree mod times so the derived (newest-first) order is
	// deterministic even when both creates land in the same instant.
	for i, name := range []string{"first", "second"} {
		if stdout, stderr, err := env.run("create", "--name", name, "--skip-switch"); err != nil {
			t.Fatalf("create %s failed: %v\nstdout: %s\nstderr: %s", name, err, stdout, stderr)
		}
		when := time.Now().Add(time.Duration(i-2) * time.Minute)
		if err := os.Chtimes(filepath.Join(env.tmpDir, ".monkeypuzzle", "pieces", name), when, when); err != nil {
			t.Fatal(err)
		}
	}

	rows := runInbox(t, env, "--json")
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %+v", rows)
	}
	// No manual order: newest piece first, ranks 1..n.
	if rows[0].Key != "inboxproj/second" || rows[0].Rank != 1 || rows[1].Key != "inboxproj/first" || rows[1].Rank != 2 {
		t.Fatalf("derived order: %+v", rows)
	}
	for _, r := range rows {
		if r.Urgency != inbox.UrgencyIdle || r.Project != "inboxproj" || r.Branch != r.Piece || r.WorktreePath == "" {
			t.Errorf("row: %+v", r)
		}
	}

	// A hand-written order wins, and annotations ride along.
	state := `{"version":1,"order":["inboxproj/first","inboxproj/second","inboxproj/gone"],"notes":{"inboxproj/first":"ship it","inboxproj/gone":"stale"}}`
	statePath := filepath.Join(env.configDir, "inbox.json")
	if err := os.WriteFile(statePath, []byte(state), 0o644); err != nil {
		t.Fatal(err)
	}
	rows = runInbox(t, env)
	if rows[0].Key != "inboxproj/first" || rows[0].Rank != 1 || rows[0].Note != "ship it" || rows[1].Key != "inboxproj/second" || rows[1].Rank != 2 {
		t.Fatalf("manual order: %+v", rows)
	}

	// The stale key was dropped from the saved state.
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	var saved inbox.State
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatalf("state file: %v\n%s", err, data)
	}
	if len(saved.Order) != 2 || saved.Order[1] != "inboxproj/second" || saved.Notes["inboxproj/gone"] != "" {
		t.Fatalf("stale key kept: %+v", saved)
	}

	// Works from outside any repo too.
	outside := t.TempDir()
	stdout, stderr, err := env.runInDir(outside, "inbox", "--sort", "urgency")
	if err != nil {
		t.Fatalf("inbox outside repo: %v\nstderr: %s", err, stderr)
	}
	if rows = parseInbox(t, stdout); len(rows) != 2 {
		t.Fatalf("outside repo: %+v", rows)
	}
	if _, _, err := env.run("inbox", "--sort", "bogus"); err == nil {
		t.Fatal("bogus --sort must fail")
	}
}

func runInbox(t *testing.T, env *testEnv, args ...string) []inbox.Row {
	t.Helper()
	stdout, stderr, err := env.run(append([]string{"inbox"}, args...)...)
	if err != nil {
		t.Fatalf("inbox failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}
	return parseInbox(t, stdout)
}

func parseInbox(t *testing.T, stdout string) []inbox.Row {
	t.Helper()
	var out struct {
		Rows []inbox.Row `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not {\"rows\":[…]}: %v\n%s", err, stdout)
	}
	return out.Rows
}
