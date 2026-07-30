package agent

import (
	"context"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// Zero-install agent detection: recognize agent TUIs by looking at the panes
// of a piece's session — the process name says whether a pane runs an agent,
// the visible screen content says what state it is in. Nothing is installed
// into the agent; `mp integration install` remains an optional upgrade that
// adds precise session ids, the "done" state, and hook events.

// agentCommandKinds maps pane_current_command values to an agent kind. node
// covers agents distributed as node binaries whose process title isn't set;
// kind stays generic for those.
var agentCommandKinds = map[string]string{
	"claude": "claude",
	"codex":  "codex",
	"node":   "agent",
}

// classification markers, checked case-insensitively against the pane tail.
// Blocked detection is deliberately strict (herdr-style): a false 🔴 trains
// people to ignore it. "❯ 1." is the option selector of claude's permission
// dialog; free-text like "do you want" alone never triggers.
var (
	blockedMarkers = []string{"❯ 1.", "allow command", "waiting for your input", "needs your permission"}
	workingMarkers = []string{"esc to interrupt"}
)

// classifyPane maps visible pane content to an agent status. Order matters:
// an open permission dialog means blocked even if a spinner line lingers.
func classifyPane(content string) string {
	tail := strings.ToLower(paneTail(content, 40))
	for _, marker := range blockedMarkers {
		if strings.Contains(tail, marker) {
			return piece.AgentBlocked
		}
	}
	for _, marker := range workingMarkers {
		if strings.Contains(tail, marker) {
			return piece.AgentWorking
		}
	}
	return piece.AgentIdle
}

// paneTail returns the last n non-empty lines: agent TUIs render state at the
// bottom of the screen.
func paneTail(content string, n int) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, n)
	for i := len(lines) - 1; i >= 0 && len(kept) < n; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			kept = append(kept, lines[i])
		}
	}
	return strings.Join(kept, "\n")
}

// detectSessionAgents finds agent processes in a session's panes and
// classifies each from its screen content.
func detectSessionAgents(ctx context.Context, pane core.PaneOps, pieceName, sessionName string) ([]ListItem, map[string]bool) {
	panes, err := pane.ListPanes(ctx, sessionName)
	if err != nil {
		return nil, nil
	}

	agentPanes := make(map[string]bool)
	var items []ListItem
	for _, p := range panes {
		kind, isAgent := agentCommandKinds[p.Command]
		if !isAgent {
			continue
		}
		agentPanes[p.ID] = true

		status := piece.AgentIdle
		if content, err := pane.CapturePane(ctx, p.ID); err == nil {
			status = classifyPane(string(content))
		}
		items = append(items, ListItem{
			Piece:       pieceName,
			SessionName: sessionName,
			ID:          "pane-" + p.ID,
			Kind:        kind,
			Status:      status,
			PID:         p.PID,
			Pane:        p.ID,
			UpdatedAt:   time.Now(),
		})
	}
	return items, agentPanes
}

// mergeAgents combines hook-reported records with detected panes for one
// piece. Detection is the current truth for working/blocked/idle; a hook's
// "done" survives an idle screen (the agent finished, then sat at its
// prompt). Pane-bearing hook records not confirmed by detection are stale
// (the agent exited without a SessionEnd hook) and are dropped; pane-less
// records (headless agents) pass through untouched.
func mergeAgents(reported, detected []ListItem, agentPanes map[string]bool, detectionRan bool) []ListItem {
	byPane := make(map[string]int)
	merged := make([]ListItem, 0, len(reported)+len(detected))
	for _, item := range reported {
		if detectionRan && item.Pane != "" && !agentPanes[item.Pane] {
			continue
		}
		if item.Pane != "" {
			byPane[item.Pane] = len(merged)
		}
		merged = append(merged, item)
	}
	for _, d := range detected {
		if i, ok := byPane[d.Pane]; ok {
			if !(d.Status == piece.AgentIdle && merged[i].Status == piece.AgentDone) {
				merged[i].Status = d.Status
			}
			continue
		}
		merged = append(merged, d)
	}
	return merged
}
