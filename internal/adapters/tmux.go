package adapters

import (
	"context"
	"fmt"
	"os"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// TmuxMultiplexer implements core.Multiplexer for tmux.
type TmuxMultiplexer struct {
	exec core.Exec
}

// NewTmuxMultiplexer creates a TmuxMultiplexer.
func NewTmuxMultiplexer(exec core.Exec) *TmuxMultiplexer {
	return &TmuxMultiplexer{exec: exec}
}

// SwitchTo switches to (or creates) a tmux session.
func (t *TmuxMultiplexer) SwitchTo(ctx context.Context, sessionName, workDir string) error {
	if !t.Exists(ctx, sessionName) {
		_, err := t.exec.Run(ctx, "tmux", "new-session", "-d", "-s", sessionName, "-c", workDir)
		if err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	if t.InSession() {
		_, err := t.exec.Run(ctx, "tmux", "switch-client", "-t", sessionName)
		if err != nil {
			return fmt.Errorf("failed to switch tmux client: %w", err)
		}
		return nil
	}

	_, err := t.exec.Run(ctx, "tmux", "attach-session", "-t", sessionName)
	if err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

// Kill terminates a tmux session.
func (t *TmuxMultiplexer) Kill(ctx context.Context, sessionName string) error {
	_, err := t.exec.Run(ctx, "tmux", "kill-session", "-t", sessionName)
	if err != nil {
		return fmt.Errorf("failed to kill tmux session: %w", err)
	}
	return nil
}

// Exists checks if a tmux session exists.
func (t *TmuxMultiplexer) Exists(ctx context.Context, sessionName string) bool {
	_, err := t.exec.Run(ctx, "tmux", "has-session", "-t", sessionName)
	return err == nil
}

// InSession returns true if inside a tmux session.
func (t *TmuxMultiplexer) InSession() bool {
	return os.Getenv("TMUX") != ""
}

// IsInstalled returns true if tmux is available.
func (t *TmuxMultiplexer) IsInstalled(ctx context.Context) bool {
	_, err := t.exec.Run(ctx, "which", "tmux")
	return err == nil
}
