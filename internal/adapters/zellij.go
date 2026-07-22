package adapters

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// ZellijMultiplexer implements core.Multiplexer for zellij.
//
// Unlike tmux, zellij cannot move a client between sessions from inside one
// (nested attach is forbidden and there is no switch-client equivalent), so
// pieces map to TABS within the user's current session rather than separate
// sessions: the "session name" mp passes in becomes the tab name.
type ZellijMultiplexer struct {
	exec core.Exec
}

// NewZellijMultiplexer creates a ZellijMultiplexer.
func NewZellijMultiplexer(exec core.Exec) *ZellijMultiplexer {
	return &ZellijMultiplexer{exec: exec}
}

// SwitchTo focuses (or creates) the tab named sessionName.
func (z *ZellijMultiplexer) SwitchTo(ctx context.Context, sessionName, workDir string) error {
	if z.Exists(ctx, sessionName) {
		_, err := z.exec.Run(ctx, "zellij", "action", "go-to-tab-name", sessionName)
		if err != nil {
			return fmt.Errorf("failed to switch zellij tab: %w", err)
		}
		return nil
	}
	// new-tab focuses the created tab. go-to-tab-name --create exists but
	// cannot set the cwd, so create explicitly.
	_, err := z.exec.Run(ctx, "zellij", "action", "new-tab", "--name", sessionName, "--cwd", workDir)
	if err != nil {
		return fmt.Errorf("failed to create zellij tab: %w", err)
	}
	return nil
}

// Kill closes the tab named sessionName. zellij's close-tab only acts on the
// focused tab, so this jumps focus to the target first — after a kill, focus
// stays wherever zellij lands post-close. A missing tab is not an error
// (idempotent kill).
func (z *ZellijMultiplexer) Kill(ctx context.Context, sessionName string) error {
	if !z.Exists(ctx, sessionName) {
		return nil
	}
	if _, err := z.exec.Run(ctx, "zellij", "action", "go-to-tab-name", sessionName); err != nil {
		return fmt.Errorf("failed to focus zellij tab for close: %w", err)
	}
	if _, err := z.exec.Run(ctx, "zellij", "action", "close-tab"); err != nil {
		return fmt.Errorf("failed to close zellij tab: %w", err)
	}
	return nil
}

// Exists checks if a tab named sessionName exists in the current session.
// Exact line match — "mp/dearest" must not match "mp/dearest-mobileapp".
func (z *ZellijMultiplexer) Exists(ctx context.Context, sessionName string) bool {
	out, err := z.exec.Run(ctx, "zellij", "action", "query-tab-names")
	if err != nil {
		return false
	}
	for _, name := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(name) == sessionName {
			return true
		}
	}
	return false
}

// InSession returns true if inside a zellij session.
func (z *ZellijMultiplexer) InSession() bool {
	return os.Getenv("ZELLIJ") != ""
}

// IsInstalled returns true if zellij is available.
func (z *ZellijMultiplexer) IsInstalled(ctx context.Context) bool {
	_, err := z.exec.Run(ctx, "which", "zellij")
	return err == nil
}

// Name returns "zellij".
func (z *ZellijMultiplexer) Name() string {
	return "zellij"
}
