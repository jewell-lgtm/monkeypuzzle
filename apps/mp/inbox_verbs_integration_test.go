//go:build integration

package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/inbox"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// TestCLI_Inbox_MoveNoteSnoozeStep drives the editing verbs and next/prev
// over two pieces (derived order: second, first) and reads the history back.
func TestCLI_Inbox_MoveNoteSnoozeStep(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()
	t.Setenv(history.EnvFile, filepath.Join(env.tmpDir, "history.jsonl"))

	env.initGitRepo()
	env.initProject("inboxproj")
	worktrees := map[string]string{}
	for i, name := range []string{"first", "second"} {
		if stdout, stderr, err := env.run("create", "--name", name, "--skip-switch"); err != nil {
			t.Fatalf("create %s failed: %v\nstdout: %s\nstderr: %s", name, err, stdout, stderr)
		}
		worktrees[name] = filepath.Join(env.tmpDir, ".monkeypuzzle", "pieces", name)
		when := time.Now().Add(time.Duration(i-2) * time.Minute)
		if err := os.Chtimes(worktrees[name], when, when); err != nil {
			t.Fatal(err)
		}
	}

	// move: bare name inside the repo, then stdin JSON, then relative placement.
	var moved inbox.MoveResult
	runInboxJSON(t, env, &moved, "inbox", "move", "first", "--top")
	if moved.Key != "inboxproj/first" || moved.Rank != 1 || moved.FromRank != 2 {
		t.Fatalf("move --top: %+v", moved)
	}
	if rows := runInbox(t, env, "--json"); rows[0].Key != "inboxproj/first" || rows[0].Rank != 1 {
		t.Fatalf("after move --top: %+v", rows)
	}
	stdout, stderr, err := env.runWithStdin(`{"piece":"inboxproj/first","down":1}`, "inbox", "move")
	if err != nil {
		t.Fatalf("move via stdin: %v\nstderr: %s", err, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &moved); err != nil || moved.Rank != 2 {
		t.Fatalf("move --down: %+v %v\n%s", moved, err, stdout)
	}
	runInboxJSON(t, env, &moved, "inbox", "move", "first", "--before", "second")
	if moved.Rank != 1 || moved.FromRank != 2 {
		t.Fatalf("move --before: %+v", moved)
	}
	if _, _, err := env.run("inbox", "move", "first"); err == nil {
		t.Fatal("move without a placement must fail")
	}
	if _, _, err := env.run("inbox", "move", "first", "--top", "--bottom"); err == nil {
		t.Fatal("move with two placements must fail")
	}
	if _, stderr, err := env.run("inbox", "move", "gone", "--top"); err == nil || !strings.Contains(stderr, "no such piece") {
		t.Fatalf("move of a missing piece: err=%v stderr=%s", err, stderr)
	}

	// note round-trip.
	var noted inbox.NoteResult
	runInboxJSON(t, env, &noted, "inbox", "note", "second", "ship it")
	if noted.Key != "inboxproj/second" || noted.Note != "ship it" {
		t.Fatalf("note: %+v", noted)
	}
	if rows := runInbox(t, env, "--json"); rows[1].Key != "inboxproj/second" || rows[1].Note != "ship it" {
		t.Fatalf("note not listed: %+v", rows)
	}
	runInboxJSON(t, env, &noted, "inbox", "note", "second", "--clear")
	if noted.Note != "" {
		t.Fatalf("note --clear: %+v", noted)
	}
	if rows := runInbox(t, env, "--json"); rows[1].Note != "" {
		t.Fatalf("note still listed: %+v", rows)
	}

	// snooze: the row drops to the bottom; next skips it.
	var snoozed inbox.SnoozeResult
	runInboxJSON(t, env, &snoozed, "inbox", "snooze", "first", "--for", "2h")
	if snoozed.Key != "inboxproj/first" || snoozed.SnoozedUntil == nil || time.Until(*snoozed.SnoozedUntil) < time.Hour {
		t.Fatalf("snooze: %+v", snoozed)
	}
	rows := runInbox(t, env, "--json")
	if rows[1].Key != "inboxproj/first" || rows[1].SnoozedUntil == nil || rows[1].Rank != 1 {
		t.Fatalf("snoozed row must be last and keep its rank: %+v", rows)
	}
	stdout, stderr, err = env.runInDir(worktrees["second"], "inbox", "next")
	if err != nil || strings.TrimSpace(stdout) != "" || !strings.Contains(stderr, "only piece") {
		t.Fatalf("next with one live row: err=%v stdout=%q stderr=%q", err, stdout, stderr)
	}
	if _, _, err := env.run("inbox", "snooze", "first", "--for", "yesterday"); err == nil {
		t.Fatal("bad --for must fail")
	}
	runInboxJSON(t, env, &snoozed, "inbox", "snooze", "first", "--clear")
	if snoozed.SnoozedUntil != nil {
		t.Fatalf("snooze --clear: %+v", snoozed)
	}

	// next/prev from inside piece "first" (rank 1) land on "second", wrapping;
	// off a multiplexer the method is "path" and the path is on stdout.
	var res piececmd.SwitchResult
	stdout, stderr, err = env.runInDir(worktrees["first"], "inbox", "next", "--json")
	if err != nil {
		t.Fatalf("next --json: %v\nstderr: %s", err, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &res); err != nil || res.Piece.Name != "second" || res.Method != "path" {
		t.Fatalf("next --json: %+v %v\n%s", res, err, stdout)
	}
	stdout, stderr, err = env.runInDir(worktrees["first"], "inbox", "prev")
	if err != nil {
		t.Fatalf("prev: %v\nstderr: %s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); filepath.Base(got) != "second" {
		t.Fatalf("prev must print the worktree path, got %q", got)
	}
	stdout, _, err = env.runInDir(worktrees["second"], "inbox", "next")
	if err != nil || filepath.Base(strings.TrimSpace(stdout)) != "first" {
		t.Fatalf("next wraps to rank 1: %q %v", stdout, err)
	}
	// Outside any piece: next is rank 1, prev is the last row.
	if stdout, _, err = env.run("inbox", "next"); err != nil || filepath.Base(strings.TrimSpace(stdout)) != "first" {
		t.Fatalf("next outside a piece: %q %v", stdout, err)
	}
	if stdout, _, err = env.run("inbox", "prev"); err != nil || filepath.Base(strings.TrimSpace(stdout)) != "second" {
		t.Fatalf("prev outside a piece: %q %v", stdout, err)
	}
	if _, _, err := env.run("inbox", "next", "--sort", "bogus"); err == nil {
		t.Fatal("bogus --sort must fail")
	}

	// refresh is the list.
	if rows := parseInbox(t, mustRun(t, env, "inbox", "refresh")); len(rows) != 2 {
		t.Fatalf("refresh: %+v", rows)
	}

	// --schema prints an input document.
	for _, verb := range []string{"move", "note", "snooze"} {
		var doc map[string]any
		if err := json.Unmarshal([]byte(mustRun(t, env, "inbox", verb, "--schema")), &doc); err != nil || doc["piece"] == "" {
			t.Fatalf("%s --schema: %v", verb, err)
		}
	}

	// History: every mutation recorded; next/prev ride on piece.switched.
	events := parseHistoryLines(t, mustRun(t, env, "history", "--event", "inbox.*"))
	var names []string
	for _, ev := range events {
		names = append(names, ev.Event)
		if ev.Project != "inboxproj" || ev.Piece == "" {
			t.Errorf("event without project/piece: %+v", ev)
		}
	}
	want := "inbox.moved inbox.moved inbox.moved inbox.noted inbox.noted inbox.snoozed inbox.snoozed"
	if strings.Join(names, " ") != want {
		t.Fatalf("want %q, got %q", want, strings.Join(names, " "))
	}
	if d := events[0].Data; d["from_rank"] != float64(2) || d["to_rank"] != float64(1) {
		t.Errorf("moved data: %+v", d)
	}
	if d := events[6].Data; d["until"] != nil {
		t.Errorf("snooze --clear must record until: null, got %+v", d)
	}
	switched := parseHistoryLines(t, mustRun(t, env, "history", "--event", "piece.switched"))
	if len(switched) != 5 {
		t.Fatalf("want 5 piece.switched (one per next/prev), got %d", len(switched))
	}
}

func mustRun(t *testing.T, env *testEnv, args ...string) string {
	t.Helper()
	stdout, stderr, err := env.run(args...)
	if err != nil {
		t.Fatalf("%v failed: %v\nstdout: %s\nstderr: %s", args, err, stdout, stderr)
	}
	return stdout
}

func runInboxJSON(t *testing.T, env *testEnv, out any, args ...string) {
	t.Helper()
	stdout := mustRun(t, env, args...)
	if err := json.Unmarshal([]byte(stdout), out); err != nil {
		t.Fatalf("%v: stdout is not JSON: %v\n%s", args, err, stdout)
	}
}
