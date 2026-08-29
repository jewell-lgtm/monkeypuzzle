package agent

import (
	"context"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
)

// observeSessionAgents converts a multiplexer's native agent tracking
// (core.AgentObserver — herdr) into list items for one piece. It plays the
// same "detected" role as detectSessionAgents and replaces it when the
// provider observes natively: identity and state come from the multiplexer
// instead of process-name matching plus screen-scraping, which also lifts
// the claude/codex allowlist — mp inherits whatever agents the provider
// recognizes. selfPane is excluded for the same reason as in detection: an
// agent running `mp wait` must not see itself and never settle.
func observeSessionAgents(ctx context.Context, observer core.AgentObserver, pieceName, sessionName, selfPane string) []ListItem {
	observations, err := observer.ObserveAgents(ctx, sessionName)
	if err != nil {
		return nil
	}

	var items []ListItem
	for _, o := range observations {
		if o.Pane == selfPane {
			continue
		}
		items = append(items, ListItem{
			Piece:       pieceName,
			SessionName: sessionName,
			ID:          "pane-" + o.Pane,
			Kind:        o.Kind,
			Status:      o.Status,
			PID:         o.PID,
			Pane:        o.Pane,
			UpdatedAt:   time.Now(),
		})
	}
	return items
}
