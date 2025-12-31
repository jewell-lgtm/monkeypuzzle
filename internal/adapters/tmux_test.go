package adapters

import (
	"context"
	"testing"
)

func TestTmux_NewSession(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		workDir     string
		mockErr     error
		wantErr     bool
	}{
		{
			name:        "success",
			sessionName: "mp-piece-test",
			workDir:     "/path/to/worktree",
			mockErr:     nil,
			wantErr:     false,
		},
		{
			name:        "failure",
			sessionName: "mp-piece-test",
			workDir:     "/path/to/worktree",
			mockErr:     MockError("duplicate session"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("tmux", []string{"new-session", "-d", "-s", tt.sessionName, "-c", tt.workDir}, nil, tt.mockErr)

			tmux := NewTmux(exec)
			err := tmux.NewSession(context.Background(), tt.sessionName, tt.workDir)

			if (err != nil) != tt.wantErr {
				t.Errorf("NewSession() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTmux_HasSession(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		mockErr     error
		want        bool
	}{
		{
			name:        "session exists",
			sessionName: "mp-piece-test",
			mockErr:     nil,
			want:        true,
		},
		{
			name:        "session not found",
			sessionName: "mp-piece-test",
			mockErr:     MockError("session not found"),
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("tmux", []string{"has-session", "-t", tt.sessionName}, nil, tt.mockErr)

			tmux := NewTmux(exec)
			got := tmux.HasSession(context.Background(), tt.sessionName)

			if got != tt.want {
				t.Errorf("HasSession() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTmux_KillSession(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		mockErr     error
		wantErr     bool
	}{
		{
			name:        "success",
			sessionName: "mp-piece-test",
			mockErr:     nil,
			wantErr:     false,
		},
		{
			name:        "session not found",
			sessionName: "mp-piece-test",
			mockErr:     MockError("session not found"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("tmux", []string{"kill-session", "-t", tt.sessionName}, nil, tt.mockErr)

			tmux := NewTmux(exec)
			err := tmux.KillSession(context.Background(), tt.sessionName)

			if (err != nil) != tt.wantErr {
				t.Errorf("KillSession() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTmux_SwitchClient(t *testing.T) {
	tests := []struct {
		name        string
		sessionName string
		mockErr     error
		wantErr     bool
	}{
		{
			name:        "success",
			sessionName: "mp-piece-test",
			mockErr:     nil,
			wantErr:     false,
		},
		{
			name:        "failure",
			sessionName: "mp-piece-test",
			mockErr:     MockError("no clients attached"),
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := NewMockExec()
			exec.AddResponse("tmux", []string{"switch-client", "-t", tt.sessionName}, nil, tt.mockErr)

			tmux := NewTmux(exec)
			err := tmux.SwitchClient(context.Background(), tt.sessionName)

			if (err != nil) != tt.wantErr {
				t.Errorf("SwitchClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTmux_InTmux(t *testing.T) {
	// Can't easily test environment variable without modifying global state
	// Just verify it doesn't panic
	tmux := NewTmux(NewMockExec())
	_ = tmux.InTmux()
}
