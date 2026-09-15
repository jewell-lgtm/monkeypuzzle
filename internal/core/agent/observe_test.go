package agent_test

import (
	"context"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/agent"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// fakeObserverMux implements Multiplexer + PaneOps + AgentObserver. The pane
// data contradicts the observations so tests can prove which path ran.
type fakeObserverMux struct {
	fakePaneMux
	observations []core.AgentObservation
}

func (f *fakeObserverMux) ObserveAgents(ctx context.Context, sessionName string) ([]core.AgentObservation, error) {
	return f.observations, nil
}

// An AgentObserver mux must preempt screen-scraping entirely: the fake's
// panes would scrape claude-idle, but the native observation says blocked —
// and includes an agent kind (opencode) the scraping allowlist would drop.
func TestList_ObserverPreemptsScraping(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", nil)
	mux := &fakeObserverMux{
		fakePaneMux: fakePaneMux{
			panes:    []core.PaneInfo{{ID: "w1:p1", Command: "claude", PID: 20}},
			captures: map[string]string{"w1:p1": claudeIdlePane},
		},
		observations: []core.AgentObservation{
			{Pane: "w1:p1", Kind: "claude", Status: piece.AgentBlocked, PID: 20},
			{Pane: "w1:p2", Kind: "opencode", Status: piece.AgentWorking, PID: 30},
		},
	}
	h := agent.NewHandlerWithMux(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()}, mux)
	h.Alive = func(pid int) bool { return true }

	items, err := h.List(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 observed agents, got %d: %+v", len(items), items)
	}
	if items[0].Status != piece.AgentBlocked || items[0].Pane != "w1:p1" {
		t.Errorf("observation must win over the idle screen: %+v", items[0])
	}
	if items[1].Kind != "opencode" {
		t.Errorf("observer kinds pass through unfiltered: %+v", items[1])
	}
}

// Native observations merge with hook-reported records exactly like scraped
// detection: observation is the current truth for working/blocked/idle, a
// hook's "done" survives an idle observation.
func TestList_ObserverMergesWithHookRecords(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", nil)
	worktree := "/repo/.monkeypuzzle/pieces/p1"
	metadata := piece.PieceMetadata{Parent: "main", Agents: map[string]piece.AgentRecord{
		"sess-done":    {Kind: "claude", Status: piece.AgentDone, Pane: "w1:p1", PID: 20, UpdatedAt: time.Now()},
		"sess-working": {Kind: "claude", Status: piece.AgentWorking, Pane: "w1:p2", PID: 30, UpdatedAt: time.Now()},
	}}
	if err := piece.WritePieceMetadata(worktree, metadata, fs); err != nil {
		t.Fatal(err)
	}

	mux := &fakeObserverMux{
		observations: []core.AgentObservation{
			{Pane: "w1:p1", Kind: "claude", Status: piece.AgentIdle, PID: 20},
			{Pane: "w1:p2", Kind: "claude", Status: piece.AgentBlocked, PID: 30},
		},
	}
	h := agent.NewHandlerWithMux(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()}, mux)
	h.Alive = func(pid int) bool { return true }

	items, err := h.List(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	got := map[string]string{}
	for _, item := range items {
		got[item.Pane] = item.Status
	}
	if got["w1:p1"] != piece.AgentDone {
		t.Errorf("hook done must survive an idle observation, got %q", got["w1:p1"])
	}
	if got["w1:p2"] != piece.AgentBlocked {
		t.Errorf("observation must override a stale hook status, got %q", got["w1:p2"])
	}
}
