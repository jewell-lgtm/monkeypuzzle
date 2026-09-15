package history_test

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
)

func useTempLog(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "history.jsonl")
	t.Setenv(history.EnvFile, p)
	return p
}

func TestPath_EnvOverride(t *testing.T) {
	t.Setenv(history.EnvFile, "/tmp/custom.jsonl")
	p, err := history.Path()
	if err != nil || p != "/tmp/custom.jsonl" {
		t.Fatalf("Path() = %q, %v; want /tmp/custom.jsonl", p, err)
	}
}

func TestPath_XDGStateHome(t *testing.T) {
	t.Setenv(history.EnvFile, "")
	t.Setenv("XDG_STATE_HOME", "/xdg/state")
	p, err := history.Path()
	if err != nil || p != filepath.Join("/xdg/state", "monkeypuzzle", "history.jsonl") {
		t.Fatalf("Path() = %q, %v", p, err)
	}
}

func TestPath_HomeFallback(t *testing.T) {
	t.Setenv(history.EnvFile, "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "/home/me")
	p, err := history.Path()
	if err != nil || p != filepath.Join("/home/me", ".local", "state", "monkeypuzzle", "history.jsonl") {
		t.Fatalf("Path() = %q, %v", p, err)
	}
}

func TestPath_NoHome(t *testing.T) {
	t.Setenv(history.EnvFile, "")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")
	if _, err := history.Path(); !errors.Is(err, history.ErrNoHome) {
		t.Fatalf("expected ErrNoHome, got %v", err)
	}
}

func TestAppend_WritesOneLinePerEvent_AndCreatesParentDirs(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "deeper", "history.jsonl")
	t.Setenv(history.EnvFile, p)

	if err := history.Append(history.Event{Event: "piece.created", Project: "proj", Piece: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := history.Append(history.Event{Event: "piece.done", Project: "proj", Piece: "a"}); err != nil {
		t.Fatal(err)
	}

	lines := readLines(t, p)
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), lines)
	}
	var ev history.Event
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("line 0 not JSON: %v", err)
	}
	if ev.Event != "piece.created" || ev.Actor.Kind == "" || ev.TS == "" {
		t.Errorf("defaults not filled: %+v", ev)
	}
	if _, err := time.Parse(time.RFC3339, ev.TS); err != nil {
		t.Errorf("ts not RFC3339: %q", ev.TS)
	}
}

func TestAppend_ConcurrentWritersProduceIntactLines(t *testing.T) {
	p := useTempLog(t)
	const writers, each = 8, 25
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < each; i++ {
				_ = history.Append(history.Event{Event: "piece.switched", Project: "proj", Piece: strings.Repeat("x", 200)})
			}
		}()
	}
	wg.Wait()

	lines := readLines(t, p)
	if len(lines) != writers*each {
		t.Fatalf("want %d lines, got %d", writers*each, len(lines))
	}
	for i, l := range lines {
		var ev history.Event
		if err := json.Unmarshal([]byte(l), &ev); err != nil {
			t.Fatalf("line %d corrupt: %v", i, err)
		}
	}
}

func TestCurrentActor(t *testing.T) {
	t.Setenv("CLAUDECODE", "")
	t.Setenv("MP_AGENT_ID", "")
	if a := history.CurrentActor(); a.Kind != "user" || a.ID != "" {
		t.Errorf("plain env: %+v", a)
	}
	t.Setenv("CLAUDECODE", "1")
	if a := history.CurrentActor(); a.Kind != "agent" || a.ID != "" {
		t.Errorf("CLAUDECODE: %+v", a)
	}
	t.Setenv("MP_AGENT_ID", "claude-42")
	if a := history.CurrentActor(); a.Kind != "agent" || a.ID != "claude-42" {
		t.Errorf("MP_AGENT_ID: %+v", a)
	}
}

func TestRecord_NeverFails_WarnsOnOutput(t *testing.T) {
	// A directory where the file should be makes the open fail.
	dir := t.TempDir()
	t.Setenv(history.EnvFile, dir)
	out := adapters.NewBufferOutput()
	history.Record(out, history.Event{Event: "piece.created", Project: "p", Piece: "x"})
	msgs := out.Messages
	if len(msgs) != 1 || msgs[0].Type != core.MsgWarning {
		t.Fatalf("expected one warning, got %+v", msgs)
	}
}

func TestRead_Filters(t *testing.T) {
	p := useTempLog(t)
	base := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	seed := []history.Event{
		{TS: base.Format(time.RFC3339), Event: "piece.created", Project: "alpha", Piece: "a"},
		{TS: base.Add(1 * time.Hour).Format(time.RFC3339), Event: "pr.created", Project: "alpha", Piece: "a", Data: map[string]any{"pr_number": 7}},
		{TS: base.Add(2 * time.Hour).Format(time.RFC3339), Event: "pr.ready", Project: "alpha", Piece: "a"},
		{TS: base.Add(3 * time.Hour).Format(time.RFC3339), Event: "piece.created", Project: "beta", Piece: "b"},
		{TS: base.Add(4 * time.Hour).Format(time.RFC3339), Event: "piece.done", Project: "alpha", Piece: "a"},
	}
	for _, ev := range seed {
		if err := history.Append(ev); err != nil {
			t.Fatal(err)
		}
	}
	// Malformed and blank lines must be skipped, not fail the read.
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("{not json\n\n{\"ts\":\"x\"}\n")
	_ = f.Close()

	cases := []struct {
		name string
		opts history.ReadOptions
		want []string
	}{
		{"all", history.ReadOptions{}, []string{"piece.created", "pr.created", "pr.ready", "piece.created", "piece.done"}},
		{"project", history.ReadOptions{Project: "beta"}, []string{"piece.created"}},
		{"piece", history.ReadOptions{Piece: "a", Event: "piece.*"}, []string{"piece.created", "piece.done"}},
		{"event exact", history.ReadOptions{Event: "pr.ready"}, []string{"pr.ready"}},
		{"event glob", history.ReadOptions{Event: "pr.*"}, []string{"pr.created", "pr.ready"}},
		{"since", history.ReadOptions{Since: base.Add(3 * time.Hour)}, []string{"piece.created", "piece.done"}},
		{"limit keeps last n", history.ReadOptions{Limit: 2}, []string{"piece.created", "piece.done"}},
		{"no match", history.ReadOptions{Project: "nope"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := history.Read(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, ev := range got {
				names = append(names, ev.Event)
			}
			if strings.Join(names, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", names, tc.want)
			}
		})
	}

	// Data round-trips (numbers come back as float64 via encoding/json).
	got, _ := history.Read(history.ReadOptions{Event: "pr.created"})
	if n, _ := got[0].Data["pr_number"].(float64); n != 7 {
		t.Errorf("data lost: %+v", got[0].Data)
	}
}

func TestRead_MissingFileIsEmpty(t *testing.T) {
	t.Setenv(history.EnvFile, filepath.Join(t.TempDir(), "absent.jsonl"))
	got, err := history.Read(history.ReadOptions{})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestMatchEvent(t *testing.T) {
	cases := map[[2]string]bool{
		{"pr.*", "pr.created"}:        true,
		{"pr.*", "piece.created"}:     false,
		{"*", "anything"}:             true,
		{"piece.done", "piece.done"}:  true,
		{"piece.done", "piece.donee"}: false,
	}
	for in, want := range cases {
		if got := history.MatchEvent(in[0], in[1]); got != want {
			t.Errorf("MatchEvent(%q, %q) = %v, want %v", in[0], in[1], got, want)
		}
	}
}

func readLines(t *testing.T, p string) []string {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines
}
