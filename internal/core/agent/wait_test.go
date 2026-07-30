package agent_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/agent"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// seedPiece writes a piece dir + metadata with the given agent statuses.
func seedPiece(t *testing.T, fs *adapters.MemoryFS, repoRoot, name string, statuses map[string]string) {
	t.Helper()
	worktree := filepath.Join(repoRoot, ".monkeypuzzle", "pieces", name)
	if err := fs.MkdirAll(worktree, 0755); err != nil {
		t.Fatal(err)
	}
	agents := make(map[string]piece.AgentRecord)
	for id, status := range statuses {
		agents[id] = piece.AgentRecord{Status: status, UpdatedAt: time.Now()}
	}
	metadata := piece.PieceMetadata{Parent: "main", Agents: agents}
	if err := piece.WritePieceMetadata(worktree, metadata, fs); err != nil {
		t.Fatal(err)
	}
}

func newWaitHandler(fs *adapters.MemoryFS) *agent.Handler {
	h := agent.NewHandler(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()})
	h.Alive = func(pid int) bool { return true }
	return h
}

func TestSnapshot_SettledAndAggregates(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", map[string]string{"a1": piece.AgentBlocked, "a2": piece.AgentDone})
	seedPiece(t, fs, "/repo", "p2", map[string]string{"b1": piece.AgentWorking})
	h := newWaitHandler(fs)

	aggregates, settled, err := h.Snapshot(context.Background(), "/repo", nil)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if settled {
		t.Error("expected not settled while an agent is working")
	}
	got := map[string]string{}
	for _, a := range aggregates {
		got[a.Piece] = a.Aggregate
	}
	if got["p1"] != piece.AgentBlocked || got["p2"] != piece.AgentWorking {
		t.Errorf("unexpected aggregates: %v", got)
	}

	// Scoped to the settled piece only.
	_, settled, err = h.Snapshot(context.Background(), "/repo", []string{"p1"})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if !settled {
		t.Error("expected p1 alone to be settled")
	}
}

func TestWaitSettled_ImmediateWhenSettled(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", map[string]string{"a1": piece.AgentDone})
	h := newWaitHandler(fs)

	result, err := h.WaitSettled(context.Background(), "/repo", nil, agent.WaitOptions{Interval: time.Hour, Grace: time.Hour})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !result.Settled {
		t.Error("expected settled: a seen agent ends the grace window")
	}
}

func TestWaitSettled_TimesOut(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", map[string]string{"a1": piece.AgentWorking})
	h := newWaitHandler(fs)

	result, err := h.WaitSettled(context.Background(), "/repo", nil, agent.WaitOptions{Interval: 10 * time.Millisecond, Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !result.TimedOut || result.Settled {
		t.Errorf("expected timed-out unsettled result, got %+v", result)
	}
}

// The launch race: no agents have reported yet. Grace keeps the wait polling
// instead of settling vacuously.
func TestWaitSettled_GraceCoversEmptyStart(t *testing.T) {
	fs := adapters.NewMemoryFS()
	h := newWaitHandler(fs)

	start := time.Now()
	result, err := h.WaitSettled(context.Background(), "/repo", []string{"p1"}, agent.WaitOptions{Interval: 10 * time.Millisecond, Grace: 60 * time.Millisecond})
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if !result.Settled {
		t.Error("expected settled after grace expires")
	}
	if time.Since(start) < 60*time.Millisecond {
		t.Error("expected wait to hold for the grace window")
	}
}

func TestFind(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", map[string]string{"a1": piece.AgentWorking, "a2": piece.AgentBlocked})
	h := newWaitHandler(fs)
	ctx := context.Background()

	byID, err := h.Find(ctx, "/repo", "a1")
	if err != nil || byID.ID != "a1" {
		t.Errorf("Find by id = %+v, %v", byID, err)
	}

	// By piece name: the blocked agent outranks the working one.
	byPiece, err := h.Find(ctx, "/repo", "p1")
	if err != nil || byPiece.ID != "a2" {
		t.Errorf("Find by piece = %+v, %v; want blocked agent a2", byPiece, err)
	}

	if _, err := h.Find(ctx, "/repo", "nope"); err == nil {
		t.Error("expected error for unknown query")
	}
}

func TestListItemTarget(t *testing.T) {
	withPane := agent.ListItem{Pane: "%7", SessionName: "mp/x/y"}
	if withPane.Target() != "%7" {
		t.Errorf("Target = %q, want pane", withPane.Target())
	}
	noPane := agent.ListItem{SessionName: "mp/x/y"}
	if noPane.Target() != "mp/x/y" {
		t.Errorf("Target = %q, want session", noPane.Target())
	}
}
