package adapters

import (
	"context"
	"strings"
	"testing"
)

// herdrListJSON builds a `herdr workspace list --json` payload.
func herdrListJSON(entries ...string) []byte {
	out := `{"workspaces":[`
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e
	}
	return []byte(out + `]}`)
}

// herdrPanesJSON builds a `herdr pane list <ws> --json` payload.
func herdrPanesJSON(entries ...string) []byte {
	out := `{"panes":[`
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e
	}
	return []byte(out + `]}`)
}

func TestHerdrMultiplexer_SwitchTo_FocusesExisting(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"},
		herdrListJSON(`{"id":"w2","label":"mp/proj/piece"}`), nil)
	exec.AddResponse("herdr", []string{"workspace", "focus", "w2"}, nil, nil)

	mux := NewHerdrMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/proj/piece", "/work/dir"); err != nil {
		t.Errorf("SwitchTo() error = %v", err)
	}
	if !exec.WasCalled("herdr", "workspace", "focus", "w2") {
		t.Errorf("expected workspace focus, calls: %+v", exec.GetCalls())
	}
	if exec.WasCalled("herdr", "workspace", "create", "--cwd", "/work/dir", "--label", "mp/proj/piece") {
		t.Error("must not create when the workspace already exists")
	}
}

func TestHerdrMultiplexer_SwitchTo_CreatePath(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"}, herdrListJSON(), nil)
	exec.AddResponse("herdr", []string{"workspace", "create", "--cwd", "/work/dir", "--label", "mp/proj/piece"}, nil, nil)

	mux := NewHerdrMultiplexer(exec)
	// The post-create lookup still returns an empty list, which must surface
	// as an error rather than a silent no-focus success.
	err := mux.SwitchTo(context.Background(), "mp/proj/piece", "/work/dir")
	if err == nil || !strings.Contains(err.Error(), "not listed after create") {
		t.Errorf("SwitchTo() error = %v, want not-listed-after-create", err)
	}
	if !exec.WasCalled("herdr", "workspace", "create", "--cwd", "/work/dir", "--label", "mp/proj/piece") {
		t.Errorf("expected workspace create, calls: %+v", exec.GetCalls())
	}
}

// Label matching must be exact: "mp/dearest" must not match
// "mp/dearest-mobileapp".
func TestHerdrMultiplexer_SwitchTo_ExactLabelMatch(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"},
		herdrListJSON(`{"id":"w1","label":"mp/dearest-mobileapp"}`), nil)
	exec.AddResponse("herdr", []string{"workspace", "create", "--cwd", "/work/dir", "--label", "mp/dearest"}, nil, nil)

	mux := NewHerdrMultiplexer(exec)
	_ = mux.SwitchTo(context.Background(), "mp/dearest", "/work/dir")
	if !exec.WasCalled("herdr", "workspace", "create", "--cwd", "/work/dir", "--label", "mp/dearest") {
		t.Errorf("expected a new mp/dearest workspace, calls: %+v", exec.GetCalls())
	}
	if exec.WasCalled("herdr", "workspace", "focus", "w1") {
		t.Error("must not focus the prefix-matching workspace")
	}
}

func TestHerdrMultiplexer_Kill(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"},
		herdrListJSON(`{"id":"w2","label":"mp/proj/piece"}`), nil)
	exec.AddResponse("herdr", []string{"workspace", "close", "w2"}, nil, nil)

	mux := NewHerdrMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Errorf("Kill() error = %v", err)
	}
	if !exec.WasCalled("herdr", "workspace", "close", "w2") {
		t.Errorf("expected workspace close, calls: %+v", exec.GetCalls())
	}
}

// Killing a workspace that doesn't exist is a no-op, not an error.
func TestHerdrMultiplexer_Kill_MissingWorkspaceIsNoop(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"}, herdrListJSON(), nil)

	mux := NewHerdrMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Errorf("Kill() of missing workspace should be nil, got %v", err)
	}
	if exec.WasCalled("herdr", "workspace", "close", "w2") {
		t.Error("must not close when the target doesn't exist")
	}
}

func TestHerdrMultiplexer_Exists(t *testing.T) {
	tests := []struct {
		name string
		json []byte
		err  error
		want bool
	}{
		{"exists", herdrListJSON(`{"id":"w1","label":"mp/proj/piece"}`), nil, true},
		{"missing", herdrListJSON(`{"id":"w1","label":"other"}`), nil, false},
		{"list fails", nil, MockError("no socket"), false},
		{"garbage json", []byte("not json"), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("herdr", []string{"workspace", "list", "--json"}, tt.json, tt.err)

			mux := NewHerdrMultiplexer(exec)
			if got := mux.Exists(context.Background(), "mp/proj/piece"); got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHerdrMultiplexer_InSession(t *testing.T) {
	// Env-dependent; just verify it doesn't panic (mirrors the tmux test).
	mux := NewHerdrMultiplexer(NewMockExec())
	_ = mux.InSession()
}

func TestHerdrMultiplexer_IsInstalled(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		want    bool
	}{
		{"herdr installed", nil, true},
		{"herdr not installed", MockError("not found"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("which", []string{"herdr"}, nil, tt.mockErr)

			mux := NewHerdrMultiplexer(exec)
			if got := mux.IsInstalled(context.Background()); got != tt.want {
				t.Errorf("IsInstalled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHerdrMultiplexer_SendText_ResolvesFocusedPane(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"},
		herdrListJSON(`{"id":"w2","label":"mp/proj/piece"}`), nil)
	exec.AddResponse("herdr", []string{"pane", "list", "w2", "--json"},
		herdrPanesJSON(
			`{"id":"w2:p1","command":"zsh","pid":100,"focused":false}`,
			`{"id":"w2:p2","command":"claude","pid":200,"focused":true}`,
		), nil)
	exec.AddResponse("herdr", []string{"pane", "send-text", "w2:p2", "--", "hello world"}, nil, nil)
	exec.AddResponse("herdr", []string{"pane", "send-keys", "w2:p2", "enter"}, nil, nil)

	mux := NewHerdrMultiplexer(exec)
	if err := mux.SendText(context.Background(), "mp/proj/piece", "hello world"); err != nil {
		t.Errorf("SendText() error = %v", err)
	}
	if !exec.WasCalled("herdr", "pane", "send-text", "w2:p2", "--", "hello world") {
		t.Errorf("expected send-text to the focused pane, calls: %+v", exec.GetCalls())
	}
	if !exec.WasCalled("herdr", "pane", "send-keys", "w2:p2", "enter") {
		t.Errorf("expected a separate Enter, calls: %+v", exec.GetCalls())
	}
}

// A pane-id target ("w1:p1") bypasses workspace resolution entirely.
func TestHerdrMultiplexer_SendText_PaneIDPassesThrough(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"pane", "send-text", "w1:p3", "--", "hi"}, nil, nil)
	exec.AddResponse("herdr", []string{"pane", "send-keys", "w1:p3", "enter"}, nil, nil)

	mux := NewHerdrMultiplexer(exec)
	if err := mux.SendText(context.Background(), "w1:p3", "hi"); err != nil {
		t.Errorf("SendText() error = %v", err)
	}
	if exec.WasCalled("herdr", "workspace", "list", "--json") {
		t.Error("pane-id target must not trigger a workspace lookup")
	}
}

func TestHerdrMultiplexer_CapturePane(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"pane", "read", "w2:p2", "--source", "visible"}, []byte("screen\n"), nil)

	mux := NewHerdrMultiplexer(exec)
	out, err := mux.CapturePane(context.Background(), "w2:p2")
	if err != nil {
		t.Fatalf("CapturePane() error = %v", err)
	}
	if string(out) != "screen\n" {
		t.Errorf("CapturePane() = %q", out)
	}
}

func TestHerdrMultiplexer_ListPanes(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"},
		herdrListJSON(`{"id":"w2","label":"mp/proj/piece"}`), nil)
	exec.AddResponse("herdr", []string{"pane", "list", "w2", "--json"},
		herdrPanesJSON(`{"id":"w2:p1","command":"claude","pid":4242,"focused":true}`), nil)

	mux := NewHerdrMultiplexer(exec)
	panes, err := mux.ListPanes(context.Background(), "mp/proj/piece")
	if err != nil {
		t.Fatalf("ListPanes() error = %v", err)
	}
	if len(panes) != 1 || panes[0].ID != "w2:p1" || panes[0].Command != "claude" || panes[0].PID != 4242 {
		t.Errorf("ListPanes() = %+v", panes)
	}
}

func TestHerdrMultiplexer_FocusPane(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("herdr", []string{"workspace", "list", "--json"},
		herdrListJSON(`{"id":"w2","label":"mp/proj/piece"}`), nil)
	exec.AddResponse("herdr", []string{"workspace", "focus", "w2"}, nil, nil)
	// Pane focus failing (pane closed) must not fail the call.
	exec.AddResponse("herdr", []string{"pane", "focus", "w2:p9"}, nil, MockError("pane not found"))

	mux := NewHerdrMultiplexer(exec)
	if err := mux.FocusPane(context.Background(), "mp/proj/piece", "w2:p9"); err != nil {
		t.Errorf("FocusPane() error = %v", err)
	}
	if !exec.WasCalled("herdr", "workspace", "focus", "w2") {
		t.Errorf("expected workspace focus, calls: %+v", exec.GetCalls())
	}
}
