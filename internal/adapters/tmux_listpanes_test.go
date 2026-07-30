package adapters

import (
	"context"
	"testing"
)

func TestTmuxMultiplexer_ListPanes(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("tmux",
		[]string{"list-panes", "-s", "-t", "=mp/proj/piece", "-F", "#{pane_id}\t#{pane_current_command}\t#{pane_pid}"},
		[]byte("%1\tzsh\t100\n%2\tclaude\t200\n"), nil)

	mux := NewTmuxMultiplexer(exec)
	panes, err := mux.ListPanes(context.Background(), "mp/proj/piece")
	if err != nil {
		t.Fatalf("ListPanes: %v", err)
	}
	if len(panes) != 2 {
		t.Fatalf("expected 2 panes, got %d", len(panes))
	}
	if panes[1].ID != "%2" || panes[1].Command != "claude" || panes[1].PID != 200 {
		t.Errorf("unexpected pane: %+v", panes[1])
	}
}
