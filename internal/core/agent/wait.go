package agent

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// PieceAggregate is one piece's agent state in wait/snapshot output.
type PieceAggregate struct {
	Piece     string `json:"piece"`
	Aggregate string `json:"aggregate"`
}

// WaitResult is the output of `mp wait`.
type WaitResult struct {
	Pieces []PieceAggregate `json:"pieces"`
	// Settled means no agent in the target pieces is working.
	Settled  bool `json:"settled"`
	TimedOut bool `json:"timed_out,omitempty"`
}

// Snapshot aggregates live agents per piece. With a filter it reports exactly
// those pieces (missing/agent-less ones aggregate to ""); without, every piece
// that has live agents. settled = no agent working.
func (h *Handler) Snapshot(ctx context.Context, repoRoot string, pieces []string) ([]PieceAggregate, bool, error) {
	items, err := h.List(ctx, repoRoot)
	if err != nil {
		return nil, false, err
	}

	byPiece := make(map[string][]piece.AgentRecord)
	for _, item := range items {
		byPiece[item.Piece] = append(byPiece[item.Piece], piece.AgentRecord{Status: item.Status})
	}

	names := pieces
	if len(names) == 0 {
		names = make([]string, 0, len(byPiece))
		for name := range byPiece {
			names = append(names, name)
		}
		sort.Strings(names)
	}

	settled := true
	result := make([]PieceAggregate, 0, len(names))
	for _, name := range names {
		agents := make(map[string]piece.AgentRecord, len(byPiece[name]))
		for i, rec := range byPiece[name] {
			agents[fmt.Sprintf("%d", i)] = rec
		}
		agg := piece.AggregateAgents(agents)
		if agg == piece.AgentWorking {
			settled = false
		}
		result = append(result, PieceAggregate{Piece: name, Aggregate: agg})
	}

	return result, settled, nil
}

// WaitSettled polls Snapshot until no agent in the target pieces is working,
// the context is cancelled, or timeout (0 = none) elapses.
func (h *Handler) WaitSettled(ctx context.Context, repoRoot string, pieces []string, interval, timeout time.Duration) (WaitResult, error) {
	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}

	for {
		aggregates, settled, err := h.Snapshot(ctx, repoRoot, pieces)
		if err != nil {
			return WaitResult{}, err
		}
		if settled {
			return WaitResult{Pieces: aggregates, Settled: true}, nil
		}

		select {
		case <-ctx.Done():
			return WaitResult{Pieces: aggregates}, ctx.Err()
		case <-deadline:
			return WaitResult{Pieces: aggregates, TimedOut: true}, fmt.Errorf("timed out after %s waiting for agents to settle", timeout)
		case <-time.After(interval):
		}
	}
}

// Find resolves an agent by exact id, falling back to piece name — in which
// case the piece's most attention-worthy agent wins (List sorts blocked
// first).
func (h *Handler) Find(ctx context.Context, repoRoot, query string) (ListItem, error) {
	items, err := h.List(ctx, repoRoot)
	if err != nil {
		return ListItem{}, err
	}
	for _, item := range items {
		if item.ID == query {
			return item, nil
		}
	}
	for _, item := range items {
		if item.Piece == query {
			return item, nil
		}
	}
	return ListItem{}, fmt.Errorf("no live agent matching %q (try 'mp agent list')", query)
}

// Target returns where pane operations should aim: the recorded pane if the
// agent reported one, else the piece session's active pane.
func (item ListItem) Target() string {
	if item.Pane != "" {
		return item.Pane
	}
	return item.SessionName
}
