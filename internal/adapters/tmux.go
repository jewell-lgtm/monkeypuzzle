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

// exactTarget formats a session name as a tmux target that matches only by
// exact name. By default tmux resolves a -t target by trying an exact match,
// then an fnmatch pattern, then a prefix — so "mp/dearest" would silently
// resolve to an existing "mp/dearest-mobileapp" session. The leading "=" forces
// an exact match, which is what we always want when targeting a session we
// named ourselves. See tmux(1) "COMMANDS / target-session".
func exactTarget(sessionName string) string {
	return "=" + sessionName
}

// SwitchTo switches to (or creates) a tmux session.
func (t *TmuxMultiplexer) SwitchTo(ctx context.Context, sessionName, workDir string) error {
	if !t.Exists(ctx, sessionName) {
		// -s names the new session literally (no "=" prefix); only -t targets
		// are subject to tmux's prefix/fnmatch resolution.
		_, err := t.exec.Run(ctx, "tmux", "new-session", "-d", "-s", sessionName, "-c", workDir)
		if err != nil {
			return fmt.Errorf("failed to create tmux session: %w", err)
		}
	}

	if t.InSession() {
		_, err := t.exec.Run(ctx, "tmux", "switch-client", "-t", exactTarget(sessionName))
		if err != nil {
			return fmt.Errorf("failed to switch tmux client: %w", err)
		}
		return nil
	}

	_, err := t.exec.Run(ctx, "tmux", "attach-session", "-t", exactTarget(sessionName))
	if err != nil {
		return fmt.Errorf("failed to attach to tmux session: %w", err)
	}
	return nil
}

// Kill terminates a tmux session.
func (t *TmuxMultiplexer) Kill(ctx context.Context, sessionName string) error {
	_, err := t.exec.Run(ctx, "tmux", "kill-session", "-t", exactTarget(sessionName))
	if err != nil {
		return fmt.Errorf("failed to kill tmux session: %w", err)
	}
	return nil
}

// Exists checks if a tmux session exists.
func (t *TmuxMultiplexer) Exists(ctx context.Context, sessionName string) bool {
	_, err := t.exec.Run(ctx, "tmux", "has-session", "-t", exactTarget(sessionName))
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

// Name returns "tmux".
func (t *TmuxMultiplexer) Name() string {
	return "tmux"
}
