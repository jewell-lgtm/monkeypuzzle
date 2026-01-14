package adapters

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

const zellijSessionName = "mp"

// ZellijMultiplexer implements core.Multiplexer for zellij using a tab-based workflow.
// All pieces live as tabs in a single "mp" session.
type ZellijMultiplexer struct {
	exec core.Exec
}

func NewZellijMultiplexer(exec core.Exec) *ZellijMultiplexer {
	return &ZellijMultiplexer{exec: exec}
}

// SwitchTo switches to a tab (creating it if needed).
// If not in zellij, attaches to the mp session first.
func (z *ZellijMultiplexer) SwitchTo(ctx context.Context, tabName, workDir string) error {
	if !z.InSession() {
		// Not in zellij - need to attach first
		return z.attachAndSwitchTab(ctx, tabName, workDir)
	}
	// Already in zellij - just switch/create tab
	return z.switchTab(ctx, tabName, workDir)
}

// Kill closes a tab by switching to it first, then closing.
func (z *ZellijMultiplexer) Kill(ctx context.Context, tabName string) error {
	if !z.InSession() {
		// Can't close tabs when not in zellij - no-op
		return nil
	}
	// Switch to tab first
	if _, err := z.exec.Run(ctx, "zellij", "action", "go-to-tab-name", tabName); err != nil {
		// Tab doesn't exist - nothing to close
		return nil
	}
	// Close current tab
	_, err := z.exec.Run(ctx, "zellij", "action", "close-tab")
	if err != nil {
		return fmt.Errorf("failed to close tab: %w", err)
	}
	return nil
}

// Exists checks if a tab exists.
// Note: zellij doesn't have great tab introspection, so this checks session existence.
func (z *ZellijMultiplexer) Exists(ctx context.Context, tabName string) bool {
	// For now, check if session exists (tabs harder to query)
	output, err := z.exec.Run(ctx, "zellij", "list-sessions")
	if err != nil {
		return false
	}
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Fields(line)
		if len(parts) > 0 && parts[0] == zellijSessionName {
			return true
		}
	}
	return false
}

func (z *ZellijMultiplexer) InSession() bool {
	return os.Getenv("ZELLIJ") != ""
}

func (z *ZellijMultiplexer) IsInstalled(ctx context.Context) bool {
	_, err := exec.LookPath("zellij")
	return err == nil
}

// switchTab switches to or creates a tab when already in zellij.
func (z *ZellijMultiplexer) switchTab(ctx context.Context, tabName, workDir string) error {
	// Try to switch to existing tab first
	_, err := z.exec.Run(ctx, "zellij", "action", "go-to-tab-name", tabName)
	if err == nil {
		return nil
	}

	// Tab doesn't exist - create it
	_, err = z.exec.Run(ctx, "zellij", "action", "new-tab", "-n", tabName, "-c", workDir)
	if err != nil {
		return fmt.Errorf("failed to create tab: %w", err)
	}
	return nil
}

// attachAndSwitchTab attaches to session and switches to tab.
func (z *ZellijMultiplexer) attachAndSwitchTab(ctx context.Context, tabName, workDir string) error {
	// Create session with initial tab if it doesn't exist
	// zellij attach --create will create or attach
	// We use a run command to create the tab after attaching

	// For now, simple approach: attach and let the tab be created on first switch
	// The --create flag creates session if needed
	// After attach, we're in zellij, so use action to create/switch tab

	// Problem: attach blocks. We need to either:
	// 1. Use a layout that creates the right tab
	// 2. Run zellij in a way that executes commands after attach

	// Approach: Create a layout with the tab, then attach with that layout
	layout := fmt.Sprintf(`layout {
    tab name="%s" cwd="%s" {
        pane
    }
}`, tabName, workDir)

	// Write temp layout
	tmpFile, err := os.CreateTemp("", "mp-zellij-*.kdl")
	if err != nil {
		return fmt.Errorf("failed to create temp layout: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(layout); err != nil {
		return fmt.Errorf("failed to write layout: %w", err)
	}
	tmpFile.Close()

	// Attach with layout - creates session if needed
	_, err = z.exec.Run(ctx, "zellij", "attach", zellijSessionName, "--create", "-l", tmpFile.Name())
	if err != nil {
		return fmt.Errorf("failed to attach to zellij: %w", err)
	}
	return nil
}
