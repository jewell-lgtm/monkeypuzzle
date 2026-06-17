package adapters

import (
	"context"
	"testing"
)

func TestTmuxMultiplexer_SwitchTo_CreatesAndAttaches(t *testing.T) {
	exec := NewMockExec()
	// Session doesn't exist (queried with an exact "=" target).
	exec.AddResponse("tmux", []string{"has-session", "-t", "=test-session"}, nil, MockError("no session"))
	// Create session (named literally with -s, no "=").
	exec.AddResponse("tmux", []string{"new-session", "-d", "-s", "test-session", "-c", "/work/dir"}, nil, nil)
	// Either attach (outside tmux) or switch-client (inside tmux) depending on $TMUX
	exec.AddResponse("tmux", []string{"attach-session", "-t", "=test-session"}, nil, nil)
	exec.AddResponse("tmux", []string{"switch-client", "-t", "=test-session"}, nil, nil)

	mux := NewTmuxMultiplexer(exec)
	err := mux.SwitchTo(context.Background(), "test-session", "/work/dir")

	if err != nil {
		t.Errorf("SwitchTo() error = %v", err)
	}
}

func TestTmuxMultiplexer_SwitchTo_ExistingSession(t *testing.T) {
	exec := NewMockExec()
	// Session exists
	exec.AddResponse("tmux", []string{"has-session", "-t", "=test-session"}, nil, nil)
	// Either attach (outside tmux) or switch-client (inside tmux) depending on $TMUX
	exec.AddResponse("tmux", []string{"attach-session", "-t", "=test-session"}, nil, nil)
	exec.AddResponse("tmux", []string{"switch-client", "-t", "=test-session"}, nil, nil)

	mux := NewTmuxMultiplexer(exec)
	err := mux.SwitchTo(context.Background(), "test-session", "/work/dir")

	if err != nil {
		t.Errorf("SwitchTo() error = %v", err)
	}
}

// TestTmuxMultiplexer_SwitchTo_PrefixCollision is the regression test for the
// bug where selecting "mp/dearest" while sitting in "mp/dearest-mobileapp" did
// nothing: tmux prefix-matched the non-existent "mp/dearest" target onto the
// already-open "mp/dearest-mobileapp" session. With exact "=" targeting,
// has-session for "mp/dearest" must NOT match, so a fresh session is created
// and switched to.
func TestTmuxMultiplexer_SwitchTo_PrefixCollision(t *testing.T) {
	exec := NewMockExec()
	// Only the longer session exists. An exact "=mp/dearest" lookup must miss
	// it; the mock returns "no response configured" (an error) for the exact
	// target, which is exactly the has-session miss we want.
	exec.AddResponse("tmux", []string{"has-session", "-t", "mp/dearest-mobileapp"}, nil, nil)
	exec.AddResponse("tmux", []string{"new-session", "-d", "-s", "mp/dearest", "-c", "/work/dir"}, nil, nil)
	exec.AddResponse("tmux", []string{"attach-session", "-t", "=mp/dearest"}, nil, nil)
	exec.AddResponse("tmux", []string{"switch-client", "-t", "=mp/dearest"}, nil, nil)

	mux := NewTmuxMultiplexer(exec)
	if err := mux.SwitchTo(context.Background(), "mp/dearest", "/work/dir"); err != nil {
		t.Fatalf("SwitchTo() error = %v", err)
	}

	// The real, distinct session must have been created — proving we did not
	// silently resolve onto the existing prefix-matching session.
	if !exec.WasCalled("tmux", "new-session", "-d", "-s", "mp/dearest", "-c", "/work/dir") {
		t.Errorf("expected a new mp/dearest session to be created, calls: %+v", exec.GetCalls())
	}
}

func TestTmuxMultiplexer_Kill(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		mockErr     error
		wantErr     bool
	}{
		{
			name:        "success",
			sessionName: "test-session",
			mockErr:     nil,
			wantErr:     false,
		},
		{
			name:        "session not found",
			sessionName: "test-session",
			mockErr:     MockError("session not found"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("tmux", []string{"kill-session", "-t", exactTarget(tt.sessionName)}, nil, tt.mockErr)

			mux := NewTmuxMultiplexer(exec)
			err := mux.Kill(context.Background(), tt.sessionName)

			if (err != nil) != tt.wantErr {
				t.Errorf("Kill() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTmuxMultiplexer_Exists(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		mockErr     error
		want        bool
	}{
		{
			name:        "session exists",
			sessionName: "test-session",
			mockErr:     nil,
			want:        true,
		},
		{
			name:        "session not found",
			sessionName: "test-session",
			mockErr:     MockError("session not found"),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("tmux", []string{"has-session", "-t", exactTarget(tt.sessionName)}, nil, tt.mockErr)

			mux := NewTmuxMultiplexer(exec)
			got := mux.Exists(context.Background(), tt.sessionName)

			if got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTmuxMultiplexer_InSession(t *testing.T) {
	// Can't easily test environment variable without modifying global state
	// Just verify it doesn't panic
	mux := NewTmuxMultiplexer(NewMockExec())
	_ = mux.InSession()
}

func TestTmuxMultiplexer_IsInstalled(t *testing.T) {
	tests := []struct {
		name    string
		mockErr error
		want    bool
	}{
		{
			name:    "tmux installed",
			mockErr: nil,
			want:    true,
		},
		{
			name:    "tmux not installed",
			mockErr: MockError("not found"),
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("which", []string{"tmux"}, nil, tt.mockErr)

			mux := NewTmuxMultiplexer(exec)
			got := mux.IsInstalled(context.Background())

			if got != tt.want {
				t.Errorf("IsInstalled() = %v, want %v", got, tt.want)
			}
		})
	}
}
