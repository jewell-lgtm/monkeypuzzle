package adapters

import (
	"context"
	"fmt"
	"os"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// Tmux provides tmux operations using an Exec interface
type Tmux struct {
	exec core.Exec
}

// NewTmux creates a Tmux adapter with the provided Exec interface
func NewTmux(exec core.Exec) *Tmux {
	return &Tmux{exec: exec}
}

// NewSession creates a new detached tmux session in the specified directory.
// The session is created in detached mode (-d) so it can be attached to later.
func (t *Tmux) NewSession(ctx context.Context, sessionName, workDir string) error {
	_, err := t.exec.Run(ctx, "tmux", "new-session", "-d", "-s", sessionName, "-c", workDir)
	if err != nil {
		return fmt.Errorf("failed to create tmux session: %w", err)
	}
	return nil
}

// AttachSession attaches to an existing tmux session.
// This will block until the session is detached or terminated.
func (t *Tmux) AttachSession(ctx context.Context, sessionName string) error {
	_, err := t.exec.Run(ctx, "tmux", "attach-session", "-t", sessionName)
	if err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

// KillSession terminates a tmux session.
func (t *Tmux) KillSession(ctx context.Context, sessionName string) error {
	_, err := t.exec.Run(ctx, "tmux", "kill-session", "-t", sessionName)
	if err != nil {
		return fmt.Errorf("failed to kill tmux session: %w", err)
	}
	return nil
}

// HasSession checks if a tmux session with the given name exists.
func (t *Tmux) HasSession(ctx context.Context, sessionName string) bool {
	_, err := t.exec.Run(ctx, "tmux", "has-session", "-t", sessionName)
	return err == nil
}

// SwitchClient switches the current tmux client to another session.
// This should be used when already inside a tmux session.
func (t *Tmux) SwitchClient(ctx context.Context, sessionName string) error {
	_, err := t.exec.Run(ctx, "tmux", "switch-client", "-t", sessionName)
	if err != nil {
		return fmt.Errorf("failed to switch tmux client: %w", err)
	}
	return nil
}

// InTmux returns true if the current process is running inside a tmux session.
func (t *Tmux) InTmux() bool {
	return os.Getenv("TMUX") != ""
}

// IsInstalled returns true if tmux is available on the system.
func (t *Tmux) IsInstalled(ctx context.Context) bool {
	_, err := t.exec.Run(ctx, "which", "tmux")
	return err == nil
}

// HasAnySessions returns true if any tmux sessions exist.
func (t *Tmux) HasAnySessions(ctx context.Context) bool {
	_, err := t.exec.Run(ctx, "tmux", "list-sessions")
	return err == nil
}
