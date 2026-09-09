package inbox

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
)

// Sort modes.
const (
	SortRank    = "rank"    // manual order, urgency breaks ties among unranked rows
	SortUrgency = "urgency" // urgency first, then rank
)

// Urgency values, most urgent first.
const (
	UrgencyBlocked = "blocked" // an agent is waiting on a human
	UrgencyReview  = "review"  // PR open and not draft, or an agent finished
	UrgencyWorking = "working" // an agent is running
	UrgencyIdle    = "idle"    // nothing derived (default)
	UrgencyMerged  = "merged"  // PR merged; ready to clean up
)

// cacheTTL is how long a project's forge lookup is reused before refetching.
const cacheTTL = 120 * time.Second

var urgencyOrder = map[string]int{
	UrgencyBlocked: 0, UrgencyReview: 1, UrgencyWorking: 2, UrgencyIdle: 3, UrgencyMerged: 4,
}

// Options tunes List.
type Options struct {
	Sort    string // SortRank (default) or SortUrgency
	Refresh bool   // bypass the forge cache
	// CwdRoot is the main repo root the caller stands in. When it is an mp
	// project that isn't registered, its pieces are listed too.
	CwdRoot string
}

// Row is one inbox entry: a piece plus its manual rank, annotations and the
// derived urgency every UI orders by.
type Row struct {
	Key          string         `json:"key"` // project/piece
	Project      string         `json:"project"`
	Piece        string         `json:"piece"`
	Rank         int            `json:"rank"`
	Host         string         `json:"host,omitempty"`
	Branch       string         `json:"branch"`
	Parent       string         `json:"parent"`
	WorktreePath string         `json:"worktree_path"`
	SessionName  string         `json:"session_name"`
	HasSession   bool           `json:"has_session"`
	AgentStatus  string         `json:"agent_status"`
	AgentCounts  map[string]int `json:"agent_counts"`
	PR           *PR            `json:"pr,omitempty"`
	Merged       bool           `json:"merged"`
	Urgency      string         `json:"urgency"`
	Note         string         `json:"note,omitempty"`
	SnoozedUntil *time.Time     `json:"snoozed_until,omitempty"`
	// IsSnoozed is SnoozedUntil evaluated at list time, so consumers of the
	// JSON never have to compare timestamps themselves.
	IsSnoozed bool      `json:"snoozed"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Snoozed reports whether the row is snoozed at now.
func (r Row) Snoozed(now time.Time) bool {
	return r.SnoozedUntil != nil && r.SnoozedUntil.After(now)
}

// Handler lists the inbox.
type Handler struct {
	deps        core.Deps
	pieces      *piece.Handler
	providerFor func(repoRoot string) (pr.Provider, error)
	now         func() time.Time
}

// NewHandler creates an inbox handler over the given piece handler (which
// carries the multiplexer used to detect live sessions and agents).
func NewHandler(deps core.Deps, pieces *piece.Handler) *Handler {
	h := &Handler{deps: deps, pieces: pieces, now: time.Now}
	h.providerFor = func(repoRoot string) (pr.Provider, error) {
		cfg, err := piece.ReadConfig(repoRoot, deps.FS)
		if err != nil {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
		providerType := cfg.PR.Provider
		if providerType == "" {
			providerType = "github"
		}
		return pr.NewProvider(pr.ProviderConfig{ProviderType: providerType, Config: cfg.PR.Config, Deps: pr.ProviderDeps{Exec: deps.Exec}})
	}
	return h
}

func (h *Handler) warn(msg string) {
	h.deps.Output.Write(core.Message{Type: core.MsgWarning, Content: "inbox: " + msg})
}

// List returns every piece across the local registered projects (plus the
// unregistered project at opts.CwdRoot), ranked and sorted. It drops state
// keys whose piece is gone and refreshes stale forge caches, saving the
// state file only when something changed.
func (h *Handler) List(ctx context.Context, opts Options) ([]Row, error) {
	return h.collect(ctx, opts, nil)
}

// mutation edits the state with the freshly listed rows (in rank order) in
// hand. cwdProject names the project the caller stands in, or "".
type mutation func(st *State, rows []Row, cwdProject string) (changed bool, err error)

// collect is List plus an optional mutation, all inside one Update: list the
// pieces, prune stale keys, rank, run fn, then re-annotate and re-rank so
// the returned rows reflect the new state. An error from fn saves nothing.
func (h *Handler) collect(ctx context.Context, opts Options, fn mutation) ([]Row, error) {
	projects, err := h.projects(opts.CwdRoot)
	if err != nil {
		return nil, err
	}
	now := h.now()
	var rows []Row
	err = Update(h.deps.FS, func(st *State) (bool, error) {
		changed := false
		cwdProject := ""
		for _, p := range projects {
			if p.Path == opts.CwdRoot {
				cwdProject = p.Name
			}
			items, err := h.pieces.ListPieces(ctx, p.Path)
			if err != nil {
				h.warn("skipping " + p.Name + ": " + err.Error())
				continue
			}
			prs, fetched := h.projectPRs(ctx, st, p, items, opts.Refresh, now)
			changed = changed || fetched
			for _, it := range items {
				rows = append(rows, buildRow(p, it, prs[p.Name+"/"+it.Name], st, now))
			}
		}
		present := make(map[string]bool, len(rows))
		for _, r := range rows {
			present[r.Key] = true
		}
		if pruneState(st, present) {
			changed = true
		}
		assignRanks(rows, st.Order)
		if fn == nil {
			return changed, nil
		}
		edited, err := fn(st, rows, cwdProject)
		if err != nil {
			return false, err
		}
		for i := range rows {
			annotate(&rows[i], st, now)
		}
		assignRanks(rows, st.Order)
		return changed || edited, nil
	})
	if err != nil {
		return nil, err
	}
	sortRows(rows, opts.Sort, now)
	if rows == nil {
		rows = []Row{}
	}
	return rows, nil
}

// projects enumerates local registered projects that exist and are init'd,
// plus the caller's own project when it isn't registered. Remote projects
// are skipped: their pieces live on the host.
func (h *Handler) projects(cwdRoot string) ([]projectcmd.Info, error) {
	infos, err := projectcmd.List()
	if err != nil {
		return nil, err
	}
	var out []projectcmd.Info
	seen := false
	for _, info := range infos {
		if info.Host != "" || !info.Exists || !info.IsProject {
			continue
		}
		seen = seen || info.Path == cwdRoot
		out = append(out, info)
	}
	if cwdRoot != "" && !seen && registry.IsProject(cwdRoot) {
		out = append(out, projectcmd.Describe(cwdRoot))
	}
	return out, nil
}

// projectPRs returns the PR per key for a project's pieces, from the cache
// when every piece's entry is younger than cacheTTL (unless refresh), else
// from one forge call whose result is written back to st.Cache. A forge
// failure warns once and yields no PRs. The bool reports a cache write.
func (h *Handler) projectPRs(ctx context.Context, st *State, p projectcmd.Info, items []piece.PieceListItem, refresh bool, now time.Time) (map[string]*PR, bool) {
	keys := make([]string, 0, len(items))
	for _, it := range items {
		keys = append(keys, p.Name+"/"+it.Name)
	}
	if len(keys) == 0 {
		return nil, false
	}
	if !refresh && cacheFresh(st.Cache, keys, now) {
		out := make(map[string]*PR, len(keys))
		for _, k := range keys {
			out[k] = st.Cache[k].PR
		}
		return out, false
	}
	provider, err := h.providerFor(p.Path)
	var prs []pr.PRInfo
	if err == nil {
		prs, err = provider.ListPRs(ctx, p.Path)
	}
	if err != nil {
		h.warn(p.Name + ": PR state unavailable: " + err.Error())
		return nil, false
	}
	byHead := stack.IndexPRsByHead(prs)
	if st.Cache == nil {
		st.Cache = map[string]CacheEntry{}
	}
	out := make(map[string]*PR, len(keys))
	for i, it := range items {
		var info *PR
		branch := it.Branch
		if branch == "" {
			branch = it.Name
		}
		if found, ok := byHead[branch]; ok {
			info = &PR{Number: found.Number, URL: found.URL, State: strings.ToLower(found.State), Draft: found.Draft}
		}
		out[keys[i]] = info
		st.Cache[keys[i]] = CacheEntry{PR: info, FetchedAt: now}
	}
	return out, true
}

// cacheFresh reports whether every key has a cache entry younger than cacheTTL.
func cacheFresh(cache map[string]CacheEntry, keys []string, now time.Time) bool {
	for _, k := range keys {
		entry, ok := cache[k]
		if !ok || now.Sub(entry.FetchedAt) >= cacheTTL {
			return false
		}
	}
	return true
}

func buildRow(p projectcmd.Info, it piece.PieceListItem, prInfo *PR, st *State, now time.Time) Row {
	key := p.Name + "/" + it.Name
	row := Row{
		Key:          key,
		Project:      p.Name,
		Piece:        it.Name,
		Host:         p.Host,
		Branch:       it.Branch,
		Parent:       it.Parent,
		WorktreePath: it.WorktreePath,
		SessionName:  it.SessionName,
		HasSession:   it.HasSession,
		AgentStatus:  it.AgentStatus,
		AgentCounts:  it.AgentCounts,
		PR:           prInfo,
		Merged:       prInfo != nil && prInfo.State == "merged",
		UpdatedAt:    it.ModTime,
	}
	annotate(&row, st, now)
	row.Urgency = deriveUrgency(it.AgentStatus, prInfo)
	return row
}

// annotate copies the row's note and snooze from the state and evaluates
// the snooze at now.
func annotate(row *Row, st *State, now time.Time) {
	row.Note = st.Notes[row.Key]
	row.SnoozedUntil = nil
	if until, ok := st.Snoozed[row.Key]; ok {
		row.SnoozedUntil = &until
	}
	row.IsSnoozed = row.Snoozed(now)
}

// deriveUrgency ranks a row from what mp already knows: blocked agents need
// a human now; an open non-draft PR or a finished agent wants a review;
// running agents are working; a merged PR is done; anything else is idle.
func deriveUrgency(agentStatus string, prInfo *PR) string {
	switch {
	case agentStatus == piece.AgentBlocked:
		return UrgencyBlocked
	case agentStatus == piece.AgentDone || (prInfo != nil && prInfo.State == "open" && !prInfo.Draft):
		return UrgencyReview
	case agentStatus == piece.AgentWorking:
		return UrgencyWorking
	case prInfo != nil && prInfo.State == "merged":
		return UrgencyMerged
	default:
		return UrgencyIdle
	}
}

// pruneState drops order/note/snooze/cache keys whose piece no longer exists.
// Returns true when anything was removed.
func pruneState(st *State, present map[string]bool) bool {
	changed := false
	kept := st.Order[:0]
	for _, k := range st.Order {
		if present[k] {
			kept = append(kept, k)
		} else {
			changed = true
		}
	}
	st.Order = kept
	for k := range st.Notes {
		if !present[k] {
			delete(st.Notes, k)
			changed = true
		}
	}
	for k := range st.Snoozed {
		if !present[k] {
			delete(st.Snoozed, k)
			changed = true
		}
	}
	for k := range st.Cache {
		if !present[k] {
			delete(st.Cache, k)
			changed = true
		}
	}
	return changed
}

// assignRanks numbers rows 1..n: keys in order first, in that order; the rest
// after them by urgency, then newest first. Rows end up in rank order.
func assignRanks(rows []Row, order []string) {
	pos := make(map[string]int, len(order))
	for i, k := range order {
		if _, dup := pos[k]; !dup {
			pos[k] = i
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		pi, ri := pos[rows[i].Key]
		pj, rj := pos[rows[j].Key]
		switch {
		case ri != rj:
			return ri
		case ri && rj:
			return pi < pj
		case urgencyOrder[rows[i].Urgency] != urgencyOrder[rows[j].Urgency]:
			return urgencyOrder[rows[i].Urgency] < urgencyOrder[rows[j].Urgency]
		case !rows[i].UpdatedAt.Equal(rows[j].UpdatedAt):
			return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
		default:
			return rows[i].Key < rows[j].Key
		}
	})
	for i := range rows {
		rows[i].Rank = i + 1
	}
}

// sortRows orders rows for display: snoozed rows last in every mode; within
// each group by rank, or by urgency then rank for SortUrgency.
func sortRows(rows []Row, mode string, now time.Time) {
	sort.SliceStable(rows, func(i, j int) bool {
		si, sj := rows[i].Snoozed(now), rows[j].Snoozed(now)
		if si != sj {
			return !si
		}
		if mode == SortUrgency && urgencyOrder[rows[i].Urgency] != urgencyOrder[rows[j].Urgency] {
			return urgencyOrder[rows[i].Urgency] < urgencyOrder[rows[j].Urgency]
		}
		return rows[i].Rank < rows[j].Rank
	})
}
