package adapters

import (
	"context"
	"testing"
)

// listJSON builds a `cmux workspace list --json` payload. Workspaces mp
// creates carry the name in custom_title; auto-titled ones have it null.
func listJSON(entries ...string) []byte {
	out := `{"window_ref":"window:1","workspaces":[`
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e
	}
	return []byte(out + `]}`)
}

func TestCmuxMultiplexer_SwitchTo_CreatesWorkspace(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("cmux", []string{"workspace", "list", "--json"},
		listJSON(`{"ref":"workspace:1","custom_title":null,"title":"⠂ other"}`), nil)
	exec.AddResponse("cmux", []string{"workspace", "create", "--name", "mp/proj/piece", "--cwd", "/work/dir", "--focus", "true"}, []byte("OK workspace:2\n"), nil)

	mux := NewCmuxMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/proj/piece", "/work/dir"); err != nil {
		t.Errorf("SwitchTo() error = %v", err)
	}
	if !exec.WasCalled("cmux", "workspace", "create", "--name", "mp/proj/piece", "--cwd", "/work/dir", "--focus", "true") {
		t.Errorf("expected workspace create, calls: %+v", exec.GetCalls())
	}
}

func TestCmuxMultiplexer_SwitchTo_SelectsExisting(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("cmux", []string{"workspace", "list", "--json"},
		listJSON(`{"ref":"workspace:2","custom_title":"mp/proj/piece"}`), nil)
	exec.AddResponse("cmux", []string{"select-workspace", "--workspace", "workspace:2"}, []byte("OK workspace:2\n"), nil)

	mux := NewCmuxMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/proj/piece", "/work/dir"); err != nil {
		t.Errorf("SwitchTo() error = %v", err)
	}
	if !exec.WasCalled("cmux", "select-workspace", "--workspace", "workspace:2") {
		t.Errorf("expected select-workspace by ref, calls: %+v", exec.GetCalls())
	}
}

// Name matching is against custom_title, never the decorated display title —
// and must be exact: "mp/dearest" must not match "mp/dearest-mobileapp".
func TestCmuxMultiplexer_SwitchTo_ExactTitleMatch(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("cmux", []string{"workspace", "list", "--json"},
		listJSON(
			`{"ref":"workspace:1","custom_title":"mp/dearest-mobileapp"}`,
			`{"ref":"workspace:2","custom_title":null,"title":"⠂ mp/dearest"}`,
		), nil)
	exec.AddResponse("cmux", []string{"workspace", "create", "--name", "mp/dearest", "--cwd", "/work/dir", "--focus", "true"}, []byte("OK workspace:3\n"), nil)

	mux := NewCmuxMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/dearest", "/work/dir"); err != nil {
		t.Fatalf("SwitchTo() error = %v", err)
	}
	if !exec.WasCalled("cmux", "workspace", "create", "--name", "mp/dearest", "--cwd", "/work/dir", "--focus", "true") {
		t.Errorf("expected a new mp/dearest workspace, calls: %+v", exec.GetCalls())
	}
}

func TestCmuxMultiplexer_Kill(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("cmux", []string{"workspace", "list", "--json"},
		listJSON(`{"ref":"workspace:2","custom_title":"mp/proj/piece"}`), nil)
	exec.AddResponse("cmux", []string{"close-workspace", "--workspace", "workspace:2"}, []byte("OK workspace:2\n"), nil)

	mux := NewCmuxMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Errorf("Kill() error = %v", err)
	}
	if !exec.WasCalled("cmux", "close-workspace", "--workspace", "workspace:2") {
		t.Errorf("expected close-workspace by ref, calls: %+v", exec.GetCalls())
	}
}

// Killing a workspace that doesn't exist is a no-op, not an error.
func TestCmuxMultiplexer_Kill_MissingWorkspaceIsNoop(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("cmux", []string{"workspace", "list", "--json"}, listJSON(), nil)

	mux := NewCmuxMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Errorf("Kill() of missing workspace should be nil, got %v", err)
	}
	if exec.WasCalled("cmux", "close-workspace", "--workspace", "workspace:2") {
		t.Error("must not close-workspace when the target doesn't exist")
	}
}

func TestCmuxMultiplexer_Exists(t *testing.T) {
	tests := []struct {
		name string
		json []byte
		err  error
		want bool
	}{
		{"exists", listJSON(`{"ref":"workspace:1","custom_title":"mp/proj/piece"}`), nil, true},
		{"missing", listJSON(`{"ref":"workspace:1","custom_title":"other"}`), nil, false},
		{"list fails", nil, MockError("no socket"), false},
		{"garbage json", []byte("not json"), nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("cmux", []string{"workspace", "list", "--json"}, tt.json, tt.err)

			mux := NewCmuxMultiplexer(exec)
			if got := mux.Exists(context.Background(), "mp/proj/piece"); got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCmuxMultiplexer_InSession(t *testing.T) {
	// Env-dependent; just verify it doesn't panic (mirrors the tmux test).
	mux := NewCmuxMultiplexer(NewMockExec())
	_ = mux.InSession()
}

func TestCmuxMultiplexer_IsInstalled(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		want    bool
	}{
		{"cmux installed", nil, true},
		{"cmux not installed", MockError("not found"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("which", []string{"cmux"}, nil, tt.mockErr)

			mux := NewCmuxMultiplexer(exec)
			if got := mux.IsInstalled(context.Background()); got != tt.want {
				t.Errorf("IsInstalled() = %v, want %v", got, tt.want)
			}
		})
	}
}
