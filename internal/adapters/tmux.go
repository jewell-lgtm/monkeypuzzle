package adapters

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

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

// paneTarget formats a -t target for pane-level commands: pane ids ("%12")
// pass through; session names get the exact-match prefix plus a trailing
// colon. The colon matters: tmux's target-PANE parser rejects a bare
// "=session" (unlike target-session), but accepts "=session:" as
// session-qualified, resolving to the active pane.
func paneTarget(target string) string {
	if len(target) > 0 && target[0] == '%' {
		return target
	}
	return exactTarget(target) + ":"
}

// SendText types text into the target pane followed by Enter. -l sends the
// text literally (no key-name lookup) and "--" keeps leading-dash text from
// parsing as flags; Enter goes in a second call so it is interpreted as the
// key, not the word.
func (t *TmuxMultiplexer) SendText(ctx context.Context, target, text string) error {
	if _, err := t.exec.Run(ctx, "tmux", "send-keys", "-t", paneTarget(target), "-l", "--", text); err != nil {
		return fmt.Errorf("failed to send text to pane: %w", err)
	}
	if _, err := t.exec.Run(ctx, "tmux", "send-keys", "-t", paneTarget(target), "Enter"); err != nil {
		return fmt.Errorf("failed to send Enter to pane: %w", err)
	}
	return nil
}

// CapturePane returns the visible contents of the target pane.
func (t *TmuxMultiplexer) CapturePane(ctx context.Context, target string) ([]byte, error) {
	out, err := t.exec.Run(ctx, "tmux", "capture-pane", "-p", "-t", paneTarget(target))
	if err != nil {
		return nil, fmt.Errorf("failed to capture pane: %w", err)
	}
	return out, nil
}

// ListPanes enumerates every pane in a session (-s = all windows).
func (t *TmuxMultiplexer) ListPanes(ctx context.Context, sessionName string) ([]core.PaneInfo, error) {
	out, err := t.exec.Run(ctx, "tmux", "list-panes", "-s", "-t", exactTarget(sessionName),
		"-F", "#{pane_id}\t#{pane_current_command}\t#{pane_pid}")
	if err != nil {
		return nil, fmt.Errorf("failed to list panes: %w", err)
	}
	var panes []core.PaneInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		pid, _ := strconv.Atoi(fields[2])
		panes = append(panes, core.PaneInfo{ID: fields[0], Command: fields[1], PID: pid})
	}
	return panes, nil
}

// FocusPane switches the client to sessionName, then (if pane is given)
// selects that pane's window and the pane itself. Mirrors the tmux plugin's
// shell-level focus_agent helper. Best-effort past the switch-client call:
// select-window/select-pane failures (e.g. a pane that closed) are not fatal
// — the client still lands in the right session.
func (t *TmuxMultiplexer) FocusPane(ctx context.Context, sessionName, pane string) error {
	if _, err := t.exec.Run(ctx, "tmux", "switch-client", "-t", exactTarget(sessionName)); err != nil {
		return fmt.Errorf("failed to switch tmux client: %w", err)
	}
	if pane == "" {
		return nil
	}
	_, _ = t.exec.Run(ctx, "tmux", "select-window", "-t", paneTarget(pane))
	_, _ = t.exec.Run(ctx, "tmux", "select-pane", "-t", paneTarget(pane))
	return nil
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
