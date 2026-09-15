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

// Pane fixtures approximating real agent TUIs.
const (
	claudeWorkingPane = `
● Reading handler.go...

✻ Cogitating… (esc to interrupt · ctrl+t for todos)
`
	claudePermissionPane = `
│ Bash command                                            │
│   rm -rf ./dist                                         │
│ Do you want to proceed?                                 │
│ ❯ 1. Yes                                                │
│   2. No, and tell Claude what to do differently         │
`
	claudeIdlePane = `
● Done. The fix is in handler.go.

╭──────────────────────────────────────────╮
│ >                                        │
╰──────────────────────────────────────────╯
  ? for shortcuts
`
	// Free-text mention of a permission-ish phrase must NOT read as blocked.
	claudeChattyPane = `
● Do you want me to also add tests? I went ahead and did.

╭──────────────────────────────────────────╮
│ >                                        │
╰──────────────────────────────────────────╯
`
	codexWorkingPane = `
▌ Working (32s • esc to interrupt)
`
	// A quoted marker scrolled above the bottom window must not read as
	// working: the real spinner always hugs the last lines.
	claudeQuotedMarkerPane = `
● The docs mention "esc to interrupt" for the spinner line.
● Anything else?

╭──────────────────────────────────────────╮
│ >                                        │
╰──────────────────────────────────────────╯
  ? for shortcuts
`
)

func TestClassifyPane(t *testing.T) {
	cases := []struct {
		name, content, want string
	}{
		{"claude working", claudeWorkingPane, piece.AgentWorking},
		{"claude permission dialog", claudePermissionPane, piece.AgentBlocked},
		{"claude idle prompt", claudeIdlePane, piece.AgentIdle},
		{"chatty do-you-want is not blocked", claudeChattyPane, piece.AgentIdle},
		{"quoted marker above bottom window is not working", claudeQuotedMarkerPane, piece.AgentIdle},
		{"codex working", codexWorkingPane, piece.AgentWorking},
		{"empty pane", "", piece.AgentIdle},
	}
	for _, tc := range cases {
		if got := agent.ClassifyPaneForTest(tc.content); got != tc.want {
			t.Errorf("%s: classify = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// fakePaneMux implements Multiplexer + PaneOps without a real tmux.
type fakePaneMux struct {
	adapters.NoopMultiplexer
	panes    []core.PaneInfo
	captures map[string]string
}

func (f *fakePaneMux) Exists(ctx context.Context, sessionName string) bool { return true }
func (f *fakePaneMux) ListPanes(ctx context.Context, sessionName string) ([]core.PaneInfo, error) {
	return f.panes, nil
}
func (f *fakePaneMux) CapturePane(ctx context.Context, target string) ([]byte, error) {
	return []byte(f.captures[target]), nil
}
func (f *fakePaneMux) SendText(ctx context.Context, target, text string) error       { return nil }
func (f *fakePaneMux) FocusPane(ctx context.Context, sessionName, pane string) error { return nil }
func (f *fakePaneMux) CurrentPane() string                                           { return "" }

func TestList_DetectsAgentsWithoutHooks(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", nil) // piece exists, no hook records
	mux := &fakePaneMux{
		panes: []core.PaneInfo{
			{ID: "%1", Command: "zsh", PID: 10},
			{ID: "%2", Command: "claude", PID: 20},
			{ID: "%3", Command: "codex", PID: 30},
		},
		captures: map[string]string{
			"%2": claudePermissionPane,
			"%3": codexWorkingPane,
		},
	}
	h := agent.NewHandlerWithMux(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()}, mux)
	h.Alive = func(pid int) bool { return true }

	items, err := h.List(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 detected agents (shell pane skipped), got %d: %+v", len(items), items)
	}
	// Blocked sorts first.
	if items[0].Kind != "claude" || items[0].Status != piece.AgentBlocked || items[0].Pane != "%2" {
		t.Errorf("unexpected first item: %+v", items[0])
	}
	if items[1].Kind != "codex" || items[1].Status != piece.AgentWorking {
		t.Errorf("unexpected second item: %+v", items[1])
	}
}

func TestList_MergesHookRecordsWithDetection(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", nil)
	// Hook-reported: one agent done in pane %2; one live agent whose pane is
	// in another tmux session (detection can't see it — must survive); one
	// with a dead PID (reaped); one headless (no pane).
	worktree := "/repo/.monkeypuzzle/pieces/p1"
	metadata := piece.PieceMetadata{Parent: "main", Agents: map[string]piece.AgentRecord{
		"sess-1":    {Kind: "claude", Status: piece.AgentDone, Pane: "%2", PID: 20, UpdatedAt: time.Now()},
		"elsewhere": {Kind: "claude", Status: piece.AgentBlocked, Pane: "%9", PID: 30, UpdatedAt: time.Now()},
		"dead":      {Kind: "claude", Status: piece.AgentWorking, Pane: "%5", PID: 666, UpdatedAt: time.Now()},
		"headless":  {Kind: "claude", Status: piece.AgentWorking, PID: 40, UpdatedAt: time.Now()},
	}}
	if err := piece.WritePieceMetadata(worktree, metadata, fs); err != nil {
		t.Fatal(err)
	}

	mux := &fakePaneMux{
		panes:    []core.PaneInfo{{ID: "%2", Command: "claude", PID: 20}},
		captures: map[string]string{"%2": claudeIdlePane},
	}
	h := agent.NewHandlerWithMux(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()}, mux)
	h.Alive = func(pid int) bool { return pid != 666 }

	items, err := h.List(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	got := map[string]string{}
	for _, item := range items {
		got[item.ID] = item.Status
	}
	// done + idle screen → done survives (identity from the hook record).
	if got["sess-1"] != piece.AgentDone {
		t.Errorf("sess-1 = %q, want done; items: %+v", got["sess-1"], items)
	}
	// A live agent outside the piece session is NOT dropped by detection.
	if got["elsewhere"] != piece.AgentBlocked {
		t.Errorf("elsewhere = %q, want blocked (must survive detection)", got["elsewhere"])
	}
	// Dead PID reaped by the liveness filter; headless untouched.
	if _, exists := got["dead"]; exists {
		t.Error("dead-PID record should be reaped")
	}
	if got["headless"] != piece.AgentWorking {
		t.Errorf("headless = %q, want working", got["headless"])
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d: %+v", len(items), items)
	}
}

// An agent invoking mp (its own pane = SelfPane) must not detect itself —
// otherwise `mp wait` sees its own spinner and never settles.
func TestList_ExcludesSelfPane(t *testing.T) {
	fs := adapters.NewMemoryFS()
	seedPiece(t, fs, "/repo", "p1", nil)
	mux := &fakePaneMux{
		panes:    []core.PaneInfo{{ID: "%1", Command: "claude", PID: 10}},
		captures: map[string]string{"%1": claudeWorkingPane},
	}
	h := agent.NewHandlerWithMux(core.Deps{FS: fs, Output: adapters.NewBufferOutput(), Exec: adapters.NewMockExec()}, mux)
	h.Alive = func(pid int) bool { return true }
	h.SelfPane = "%1"

	items, err := h.List(context.Background(), "/repo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected self pane excluded, got %+v", items)
	}
}
