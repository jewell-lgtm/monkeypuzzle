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

// Kill must focus the target tab before close-tab (which only acts on the
// focused tab), in that order.
func TestZellijMultiplexer_Kill_FocusThenClose(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("zellij", []string{"action", "query-tab-names"}, []byte("mp/proj/piece\n"), nil)
	exec.AddResponse("zellij", []string{"action", "go-to-tab-name", "mp/proj/piece"}, nil, nil)
	exec.AddResponse("zellij", []string{"action", "close-tab"}, nil, nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Fatalf("Kill() error = %v", err)
	}

	calls := exec.GetCalls()
	gotoIdx, closeIdx := -1, -1
	for i, c := range calls {
		if len(c.Args) > 1 && c.Args[0] == "action" {
			switch c.Args[1] {
			case "go-to-tab-name":
				gotoIdx = i
			case "close-tab":
				closeIdx = i
			}
		}
	}
	if gotoIdx == -1 || closeIdx == -1 || gotoIdx > closeIdx {
		t.Errorf("expected go-to-tab-name before close-tab, calls: %+v", calls)
	}
}

// Killing a tab that doesn't exist is a no-op, not an error.
func TestZellijMultiplexer_Kill_MissingTabIsNoop(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("zellij", []string{"action", "query-tab-names"}, []byte("other-tab\n"), nil)

	mux := NewZellijMultiplexer(exec)
	if err := mux.Kill(context.Background(), "mp/proj/piece"); err != nil {
		t.Errorf("Kill() of missing tab should be nil, got %v", err)
	}
	if exec.WasCalled("zellij", "action", "close-tab") {
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
