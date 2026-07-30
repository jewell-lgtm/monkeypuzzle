package agent

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// AggregateCounts collapses per-status counts to one status by severity —
// the counts-shaped sibling of piece.AggregateAgents.
func AggregateCounts(counts map[string]int) string {
	for _, status := range []string{piece.AgentBlocked, piece.AgentWorking, piece.AgentDone, piece.AgentIdle} {
		if counts[status] > 0 {
			return status
		}
	}
	return ""
}

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

// WaitOptions tunes WaitSettled's polling.
type WaitOptions struct {
	Interval time.Duration
	// Timeout gives up after this long (0 = wait forever).
	Timeout time.Duration
	// Grace covers the launch race: `mp create --agent` returns before the
	// agent's first status report lands, so an immediate wait would see zero
	// agents and settle vacuously. Until Grace elapses, an all-empty snapshot
	// keeps polling; once any agent has been seen, normal settling applies.
	Grace time.Duration
}

// WaitSettled polls Snapshot until no agent in the target pieces is working,
// the context is cancelled, or the timeout elapses.
func (h *Handler) WaitSettled(ctx context.Context, repoRoot string, pieces []string, opts WaitOptions) (WaitResult, error) {
	var deadline <-chan time.Time
	if opts.Timeout > 0 {
		timer := time.NewTimer(opts.Timeout)
		defer timer.Stop()
		deadline = timer.C
	}

	start := time.Now()
	seenAgents := false
	for {
		aggregates, settled, err := h.Snapshot(ctx, repoRoot, pieces)
		if err != nil {
			return WaitResult{}, err
		}
		for _, a := range aggregates {
			if a.Aggregate != "" {
				seenAgents = true
			}
		}
		inGrace := !seenAgents && time.Since(start) < opts.Grace
		if settled && !inGrace {
			return WaitResult{Pieces: aggregates, Settled: true}, nil
		}

		select {
		case <-ctx.Done():
			return WaitResult{Pieces: aggregates}, ctx.Err()
		case <-deadline:
			return WaitResult{Pieces: aggregates, TimedOut: true}, fmt.Errorf("timed out after %s waiting for agents to settle", opts.Timeout)
		case <-time.After(opts.Interval):
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
