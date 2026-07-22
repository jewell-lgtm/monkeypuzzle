package adapters

import (
	"context"
	"testing"
)

func TestZellijMultiplexer_SwitchTo_CreatesTab(t *testing.T) {
	exec := NewMockExec()
	// Tab doesn't exist.
	exec.AddResponse("zellij", []string{"action", "query-tab-names"}, []byte("other-tab\n"), nil)
	exec.AddResponse("zellij", []string{"action", "new-tab", "--name", "mp/proj/piece", "--cwd", "/work/dir"}, nil, nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/proj/piece", "/work/dir"); err != nil {
		t.Errorf("SwitchTo() error = %v", err)
	}
	if !exec.WasCalled("zellij", "action", "new-tab", "--name", "mp/proj/piece", "--cwd", "/work/dir") {
		t.Errorf("expected new-tab call, calls: %+v", exec.GetCalls())
	}
}

func TestZellijMultiplexer_SwitchTo_ExistingTab(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("zellij", []string{"action", "query-tab-names"}, []byte("mp/proj/piece\nother-tab\n"), nil)
	exec.AddResponse("zellij", []string{"action", "go-to-tab-name", "mp/proj/piece"}, nil, nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/proj/piece", "/work/dir"); err != nil {
		t.Errorf("SwitchTo() error = %v", err)
	}
	if exec.WasCalled("zellij", "action", "new-tab", "--name", "mp/proj/piece", "--cwd", "/work/dir") {
		t.Error("must not create a tab that already exists")
	}
}

// Exact-match regression, mirroring the tmux "=" prefix-collision test: a tab
// list containing only "mp/dearest-mobileapp" must NOT satisfy an Exists check
// for "mp/dearest" — a fresh tab gets created instead.
func TestZellijMultiplexer_SwitchTo_PrefixCollision(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("zellij", []string{"action", "query-tab-names"}, []byte("mp/dearest-mobileapp\n"), nil)
	exec.AddResponse("zellij", []string{"action", "new-tab", "--name", "mp/dearest", "--cwd", "/work/dir"}, nil, nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/dearest", "/work/dir"); err != nil {
		t.Fatalf("SwitchTo() error = %v", err)
	}
	if !exec.WasCalled("zellij", "action", "new-tab", "--name", "mp/dearest", "--cwd", "/work/dir") {
		t.Errorf("expected a new mp/dearest tab to be created, calls: %+v", exec.GetCalls())
	}
}

// Kill must close by stable tab ID so the user's focus never moves — plain
// close-tab acts on the focused tab and would drag focus into the dying tab.
func TestZellijMultiplexer_Kill_ClosesByIDWithoutFocusing(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("zellij", []string{"action", "list-tabs", "--json"},
		[]byte(`[{"name":"main","tab_id":0},{"name":"mp/proj/piece","tab_id":3}]`), nil)
	exec.AddResponse("zellij", []string{"action", "close-tab-by-id", "3"}, nil, nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}
	if !exec.WasCalled("zellij", "action", "close-tab-by-id", "3") {
		t.Errorf("expected close-tab-by-id 3, calls: %+v", exec.GetCalls())
	}
	for _, c := range exec.GetCalls() {
		if len(c.Args) > 1 && (c.Args[1] == "go-to-tab-name" || c.Args[1] == "close-tab") {
			t.Errorf("Kill must not move focus or use focused close, calls: %+v", exec.GetCalls())
		}
	}
}

// Kill matches the tab name exactly — a prefix-sharing tab must not be closed.
func TestZellijMultiplexer_Kill_ExactNameMatch(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("zellij", []string{"action", "list-tabs", "--json"},
		[]byte(`[{"name":"mp/dearest-mobileapp","tab_id":2}]`), nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/dearest"); err != nil {
		t.Errorf("Kill() of missing tab should be nil, got %v", err)
	}
	if exec.WasCalled("zellij", "action", "close-tab-by-id", "2") {
		t.Error("must not close a tab whose name merely shares a prefix")
	}
}

// Killing a tab that doesn't exist is a no-op, not an error.
func TestZellijMultiplexer_Kill_MissingTabIsNoop(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("zellij", []string{"action", "list-tabs", "--json"},
		[]byte(`[{"name":"other-tab","tab_id":1}]`), nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Errorf("Kill() of missing tab should be nil, got %v", err)
	}
	if exec.WasCalled("zellij", "action", "close-tab-by-id", "1") {
		t.Error("must not close-tab when the target tab doesn't exist")
	}
}

func TestZellijMultiplexer_Exists(t *testing.T) {
	tests := []struct {
		name   string
		output string
		mockOK bool
		want   bool
	}{
		{"tab exists", "one\nmp/proj/piece\ntwo\n", true, true},
		{"tab missing", "one\ntwo\n", true, false},
		{"prefix must not match", "mp/proj/piece-longer\n", true, false},
		{"query fails", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			var mockErr error
			if !tt.mockOK {
				mockErr = MockError("not in a session")
			}
			exec.AddResponse("zellij", []string{"action", "query-tab-names"}, []byte(tt.output), mockErr)

			mux := NewZellijMultiplexer(exec)
			if got := mux.Exists(context.Background(), "mp/proj/piece"); got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestZellijMultiplexer_InSession(t *testing.T) {
	// Env-dependent; just verify it doesn't panic (mirrors the tmux test).
	mux := NewZellijMultiplexer(NewMockExec())
	_ = mux.InSession()
}

func TestZellijMultiplexer_IsInstalled(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		want    bool
	}{
		{"zellij installed", nil, true},
		{"zellij not installed", MockError("not found"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("which", []string{"zellij"}, nil, tt.mockErr)

			mux := NewZellijMultiplexer(exec)
			if got := mux.IsInstalled(context.Background()); got != tt.want {
				t.Errorf("IsInstalled() = %v, want %v", got, tt.want)
			}
		})
	}
}
