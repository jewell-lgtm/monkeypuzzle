package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
)

// Location identifies the piece an agent report targets. Resolved from cwd at
// the CLI layer (piece.Handler.Status) so this package stays git-free.
type Location struct {
	PieceName    string
	WorktreePath string
	RepoRoot     string
}

// Handler implements agent report/list/summary over piece metadata.
type Handler struct {
	deps   core.Deps
	pieces *piece.Handler
	hooks  *piece.HookRunner
	// Alive reports process liveness; swapped out in tests.
	Alive func(pid int) bool
}

// NewHandler creates an agent handler.
func NewHandler(deps core.Deps) *Handler {
	return &Handler{
		deps:   deps,
		pieces: piece.NewHandler(deps),
		hooks:  piece.NewHookRunner(deps),
		Alive:  adapters.ProcessAlive,
	}
}

// Report upserts (or, for status "gone", removes) the calling agent's record
// in the piece's metadata, reaps records whose process has died, and fires the
// agent-blocked / agent-done hooks when the piece aggregate transitions.
func (h *Handler) Report(ctx context.Context, loc Location, input ReportInput) (ReportResult, error) {
	if err := ValidateReportInput(input); err != nil {
		return ReportResult{}, err
	}
	if input.ID == "" {
		input.ID = fmt.Sprintf("pid-%d", input.PID)
	}

	metadata, err := piece.ReadPieceMetadata(loc.WorktreePath, h.deps.FS)
	if err != nil {
		return ReportResult{}, err
	}

	oldAggregate := piece.AggregateAgents(metadata.Agents)

	if metadata.Agents == nil {
		metadata.Agents = make(map[string]piece.AgentRecord)
	}
	for id, rec := range metadata.Agents {
		if id != input.ID && rec.PID > 0 && !h.Alive(rec.PID) {
			delete(metadata.Agents, id)
		}
	}
	if input.Status == StatusGone {
		delete(metadata.Agents, input.ID)
	} else {
		metadata.Agents[input.ID] = piece.AgentRecord{
			Kind:      input.Kind,
			Status:    input.Status,
			PID:       input.PID,
			Pane:      input.Pane,
			UpdatedAt: time.Now(),
		}
	}

	if err := piece.WritePieceMetadata(loc.WorktreePath, *metadata, h.deps.FS); err != nil {
		return ReportResult{}, err
	}

	newAggregate := piece.AggregateAgents(metadata.Agents)
	if newAggregate != oldAggregate {
		hookName := ""
		switch newAggregate {
		case piece.AgentBlocked:
			hookName = piece.HookAgentBlocked
		case piece.AgentDone:
			hookName = piece.HookAgentDone
		}
		if hookName != "" && loc.RepoRoot != "" {
			// Detached: a notification hook must never block or fail the report.
			if err := h.hooks.RunHookDetached(loc.RepoRoot, hookName, piece.HookContext{
				PieceName:    loc.PieceName,
				WorktreePath: loc.WorktreePath,
				RepoRoot:     loc.RepoRoot,
				AgentID:      input.ID,
				AgentKind:    input.Kind,
				AgentStatus:  newAggregate,
				AgentPane:    input.Pane,
			}); err != nil {
				h.deps.Output.Write(core.Message{Type: core.MsgWarning, Content: err.Error()})
			}
		}
	}

	return ReportResult{
		Reported:  true,
		Piece:     loc.PieceName,
		AgentID:   input.ID,
		Status:    input.Status,
		Aggregate: newAggregate,
	}, nil
}

// List returns every live agent across the project's pieces, blocked first,
// then by recency. Records with dead PIDs are filtered (not persisted — only
// Report writes).
func (h *Handler) List(ctx context.Context, repoRoot string) ([]ListItem, error) {
	pieces, err := h.pieces.ListPieces(ctx, repoRoot)
	if err != nil {
		return nil, err
	}

	var items []ListItem
	for _, p := range pieces {
		metadata, err := piece.ReadPieceMetadata(p.WorktreePath, h.deps.FS)
		if err != nil {
			continue
		}
		for id, rec := range metadata.Agents {
			if rec.PID > 0 && !h.Alive(rec.PID) {
				continue
			}
			items = append(items, ListItem{
				Piece:       p.Name,
				SessionName: p.SessionName,
				ID:          id,
				Kind:        rec.Kind,
				Status:      rec.Status,
				PID:         rec.PID,
				Pane:        rec.Pane,
				UpdatedAt:   rec.UpdatedAt,
			})
		}
	}

	rank := map[string]int{piece.AgentBlocked: 0, piece.AgentWorking: 1, piece.AgentDone: 2, piece.AgentIdle: 3}
	sort.SliceStable(items, func(i, j int) bool {
		if rank[items[i].Status] != rank[items[j].Status] {
			return rank[items[i].Status] < rank[items[j].Status]
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	return items, nil
}

// statusIcons render the summary segment; blocked first — it's the one that
// needs the human.
var statusIcons = []struct{ status, icon string }{
	{piece.AgentBlocked, "🔴"},
	{piece.AgentWorking, "⚡"},
	{piece.AgentDone, "✅"},
	{piece.AgentIdle, "💤"},
}

// Summary renders a compact status-line segment like "🔴1 ⚡2". Empty string
// when no agents are live.
func Summary(items []ListItem) string {
	counts := make(map[string]int)
	for _, item := range items {
		counts[item.Status]++
	}
	var parts []string
	for _, si := range statusIcons {
		if counts[si.status] > 0 {
			parts = append(parts, fmt.Sprintf("%s%d", si.icon, counts[si.status]))
		}
	}
	return strings.Join(parts, " ")
}
