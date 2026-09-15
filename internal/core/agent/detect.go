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

// agentCommandKinds maps pane_current_command values to an agent kind. The
// allowlist is deliberately tight: "node" would make every dev server and
// REPL an "agent" (phantom rows, and worse — a send/read target).
var agentCommandKinds = map[string]string{
	"claude": "claude",
	"codex":  "codex",
}

// classification markers, checked case-insensitively against a narrow window
// at the bottom of the pane. The windows are what keep conversation text
// honest: an agent *quoting* "esc to interrupt" mid-transcript scrolls that
// text up, while the real spinner and permission dialog always hug the
// bottom. Blocked detection is deliberately strict (herdr-style): a false 🔴
// trains people to ignore it. "❯ 1." is the option selector of claude's
// permission dialog; free-text like "do you want" alone never triggers.
var (
	blockedMarkers = []string{"❯ 1.", "allow command", "waiting for your input", "needs your permission"}
	workingMarkers = []string{"esc to interrupt"}
)

const (
	blockedWindow = 15 // dialog box + options + hint line
	workingWindow = 3  // the spinner line sits at the very bottom
)

// classifyPane maps visible pane content to an agent status. Order matters:
// an open permission dialog means blocked even if a spinner line lingers.
func classifyPane(content string) string {
	blockedTail := strings.ToLower(paneTail(content, blockedWindow))
	for _, marker := range blockedMarkers {
		if strings.Contains(blockedTail, marker) {
			return piece.AgentBlocked
		}
	}
	workingTail := strings.ToLower(paneTail(content, workingWindow))
	for _, marker := range workingMarkers {
		if strings.Contains(workingTail, marker) {
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
// classifies each from its screen content. selfPane (the pane mp itself was
// invoked from, when inside tmux) is excluded: an agent running `mp wait`
// would otherwise detect its own spinner and never settle.
func detectSessionAgents(ctx context.Context, pane core.PaneOps, pieceName, sessionName, selfPane string) []ListItem {
	panes, err := pane.ListPanes(ctx, sessionName)
	if err != nil {
		return nil
	}

	var items []ListItem
	for _, p := range panes {
		kind, isAgent := agentCommandKinds[p.Command]
		if !isAgent || p.ID == selfPane {
			continue
		}

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
	return items
}

// mergeAgents combines hook-reported records with detected panes for one
// piece. Detection is the current truth for working/blocked/idle; a hook's
// "done" survives an idle screen (the agent finished, then sat at its
// prompt). Hook records without a matching detected pane pass through
// untouched — the PID-liveness filter already reaped dead agents, and a live
// record may legitimately live outside the piece session (another tmux
// session, a wrapper binary detection doesn't recognize, headless).
func mergeAgents(reported, detected []ListItem) []ListItem {
	byPane := make(map[string]int)
	merged := make([]ListItem, 0, len(reported)+len(detected))
	for _, item := range reported {
		if item.Pane != "" {
			byPane[item.Pane] = len(merged)
		}
		merged = append(merged, item)
	}
	for _, d := range detected {
		if i, ok := byPane[d.Pane]; ok {
			if d.Status != piece.AgentIdle || merged[i].Status != piece.AgentDone {
				merged[i].Status = d.Status
			}
			continue
		}
		merged = append(merged, d)
	}
	return merged
}
