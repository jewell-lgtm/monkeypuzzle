package adapters

import (
	"context"
	"testing"
)

func TestTmuxMultiplexer_SendText_PaneTarget(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("tmux", []string{"send-keys", "-t", "%7", "-l", "--", "hello"}, nil, nil)
	exec.AddResponse("tmux", []string{"send-keys", "-t", "%7", "Enter"}, nil, nil)

	mux := NewTmuxMultiplexer(exec)
	if err := mux.SendText(context.Background(), "%7", "hello"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if !exec.WasCalled("tmux", "send-keys", "-t", "%7", "-l", "--", "hello") {
		t.Error("expected literal text send-keys call")
	}
	if !exec.WasCalled("tmux", "send-keys", "-t", "%7", "Enter") {
		t.Error("expected Enter send-keys call")
	}
}

// Session names must render as "=name:" — tmux's target-pane parser rejects
// the bare "=name" that target-session commands accept.
func TestTmuxMultiplexer_SendText_SessionTargetIsExactPaneForm(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("tmux", []string{"send-keys", "-t", "=mp/proj/piece:", "-l", "--", "claude"}, nil, nil)
	exec.AddResponse("tmux", []string{"send-keys", "-t", "=mp/proj/piece:", "Enter"}, nil, nil)

	mux := NewTmuxMultiplexer(exec)
	if err := mux.SendText(context.Background(), "mp/proj/piece", "claude"); err != nil {
		t.Fatalf("SendText: %v", err)
	}
	if !exec.WasCalled("tmux", "send-keys", "-t", "=mp/proj/piece:", "-l", "--", "claude") {
		t.Error("session name target should get exact-match prefix + colon")
	}
}

func TestTmuxMultiplexer_SendText_LeadingDashText(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("tmux", []string{"send-keys", "-t", "%7", "-l", "--", "-p run tests"}, nil, nil)
	exec.AddResponse("tmux", []string{"send-keys", "-t", "%7", "Enter"}, nil, nil)

	mux := NewTmuxMultiplexer(exec)
	if err := mux.SendText(context.Background(), "%7", "-p run tests"); err != nil {
		t.Fatalf("SendText with leading dash: %v", err)
	}
}

func TestTmuxMultiplexer_CapturePane(t *testing.T) {
	exec := NewMockExec()
	exec.AddResponse("tmux", []string{"capture-pane", "-p", "-t", "%3"}, []byte("pane contents\n"), nil)

	mux := NewTmuxMultiplexer(exec)
	out, err := mux.CapturePane(context.Background(), "%3")
	if err != nil {
		t.Fatalf("CapturePane: %v", err)
	}
	if string(out) != "pane contents\n" {
		t.Errorf("CapturePane = %q", out)
	}
}

func TestTmuxMultiplexer_SendText_Error(t *testing.T) {
	exec := NewMockExec()
	// No response configured: MockExec returns an error.
	mux := NewTmuxMultiplexer(exec)
	if err := mux.SendText(context.Background(), "%9", "x"); err == nil {
		t.Error("expected error when send-keys fails")
	}
}
