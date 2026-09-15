package inbox

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/history"
)

// Sentinels for the editing verbs.
var (
	ErrNoSuchPiece = errors.New("inbox: no such piece")
	ErrAmbiguous   = errors.New("inbox: ambiguous piece; use project/piece")
	ErrBadPosition = errors.New("inbox: pick exactly one of top, bottom, up, down, before, after")
	ErrBadSnooze   = errors.New("inbox: pick exactly one of for, until, clear")
	ErrEmpty       = errors.New("inbox: no pieces")
)

// Resolve finds the row a selector names. "project/piece" matches a key
// exactly. A bare piece name prefers the row in cwdProject (the project the
// caller stands in), else the unique match across projects; several matches
// fail with ErrAmbiguous naming the candidates.
func Resolve(rows []Row, selector, cwdProject string) (Row, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Row{}, fmt.Errorf("%w: empty selector", ErrNoSuchPiece)
	}
	var matches []Row
	for _, r := range rows {
		if r.Key == selector || (r.Piece == selector && r.Project == cwdProject) {
			return r, nil
		}
		if r.Piece == selector {
			matches = append(matches, r)
		}
	}
	switch len(matches) {
	case 0:
		return Row{}, fmt.Errorf("%w: %q", ErrNoSuchPiece, selector)
	case 1:
		return matches[0], nil
	}
	return Row{}, fmt.Errorf("%w: %q is %s", ErrAmbiguous, selector, strings.Join(rowKeys(matches), ", "))
}

func rowKeys(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Key
	}
	return out
}

// MoveInput is `mp inbox move`: a piece selector and exactly one placement.
type MoveInput struct {
	Piece  string `json:"piece"`
	Top    bool   `json:"top,omitempty"`
	Bottom bool   `json:"bottom,omitempty"`
	Up     int    `json:"up,omitempty"`
	Down   int    `json:"down,omitempty"`
	Before string `json:"before,omitempty"` // selector
	After  string `json:"after,omitempty"`  // selector
}

// Placement kinds.
const (
	PosTop    = "top"
	PosBottom = "bottom"
	PosUp     = "up"
	PosDown   = "down"
	PosBefore = "before"
	PosAfter  = "after"
)

// Position is one resolved placement for moveKey.
type Position struct {
	Kind   string
	N      int    // steps for up/down
	Target string // resolved key for before/after
}

// position validates that exactly one placement is set.
func (in MoveInput) position() (Position, error) {
	var found []Position
	if in.Top {
		found = append(found, Position{Kind: PosTop})
	}
	if in.Bottom {
		found = append(found, Position{Kind: PosBottom})
	}
	if in.Up != 0 {
		found = append(found, Position{Kind: PosUp, N: in.Up})
	}
	if in.Down != 0 {
		found = append(found, Position{Kind: PosDown, N: in.Down})
	}
	if in.Before != "" {
		found = append(found, Position{Kind: PosBefore, Target: in.Before})
	}
	if in.After != "" {
		found = append(found, Position{Kind: PosAfter, Target: in.After})
	}
	if len(found) != 1 {
		return Position{}, ErrBadPosition
	}
	if found[0].N < 0 {
		return Position{}, fmt.Errorf("%w: steps must be positive", ErrBadPosition)
	}
	return found[0], nil
}

// MoveResult is the stdout of `mp inbox move`.
type MoveResult struct {
	Key      string `json:"key"`
	Rank     int    `json:"rank"`
	FromRank int    `json:"from_rank"`
}

// Move re-ranks a piece. The current derived order is materialised into
// State.Order first (every row keeps its rank), then the placement applies.
func (h *Handler) Move(ctx context.Context, opts Options, in MoveInput) (MoveResult, error) {
	pos, err := in.position()
	if err != nil {
		return MoveResult{}, err
	}
	var res MoveResult
	var row Row
	_, err = h.collect(ctx, opts, func(st *State, rows []Row, cwdProject string) (bool, error) {
		row, err = Resolve(rows, in.Piece, cwdProject)
		if err != nil {
			return false, err
		}
		if pos.Target != "" {
			target, err := Resolve(rows, pos.Target, cwdProject)
			if err != nil {
				return false, err
			}
			pos.Target = target.Key
		}
		order, err := moveKey(rowKeys(rows), row.Key, pos)
		if err != nil {
			return false, err
		}
		res = MoveResult{Key: row.Key, Rank: indexOf(order, row.Key) + 1, FromRank: row.Rank}
		st.Order = order
		return true, nil
	})
	if err != nil {
		return MoveResult{}, err
	}
	if res.Rank != res.FromRank {
		h.record("inbox.moved", row, map[string]any{"from_rank": res.FromRank, "to_rank": res.Rank})
	}
	return res, nil
}

// moveKey returns order with key placed per pos. up/down clamp at the ends;
// before/after the key itself is a no-op.
func moveKey(order []string, key string, pos Position) ([]string, error) {
	from := indexOf(order, key)
	if from < 0 {
		return nil, fmt.Errorf("%w: %q", ErrNoSuchPiece, key)
	}
	rest := append(append(make([]string, 0, len(order)), order[:from]...), order[from+1:]...)
	to := from
	switch pos.Kind {
	case PosTop:
		to = 0
	case PosBottom:
		to = len(rest)
	case PosUp:
		to = max(from-pos.N, 0)
	case PosDown:
		to = min(from+pos.N, len(rest))
	case PosBefore, PosAfter:
		if pos.Target == key {
			break
		}
		t := indexOf(rest, pos.Target)
		if t < 0 {
			return nil, fmt.Errorf("%w: %q", ErrNoSuchPiece, pos.Target)
		}
		to = t
		if pos.Kind == PosAfter {
			to = t + 1
		}
	default:
		return nil, ErrBadPosition
	}
	out := append(append(append(make([]string, 0, len(order)), rest[:to]...), key), rest[to:]...)
	return out, nil
}

func indexOf(list []string, key string) int {
	for i, k := range list {
		if k == key {
			return i
		}
	}
	return -1
}

// NoteInput is `mp inbox note`: empty Note (or Clear) removes the note.
type NoteInput struct {
	Piece string `json:"piece"`
	Note  string `json:"note,omitempty"`
	Clear bool   `json:"clear,omitempty"`
}

// NoteResult is the stdout of `mp inbox note`.
type NoteResult struct {
	Key  string `json:"key"`
	Note string `json:"note"`
}

// Note sets or clears a piece's note.
func (h *Handler) Note(ctx context.Context, opts Options, in NoteInput) (NoteResult, error) {
	text := strings.TrimSpace(in.Note)
	if in.Clear {
		text = ""
	}
	var row Row
	changed := false
	_, err := h.collect(ctx, opts, func(st *State, rows []Row, cwdProject string) (bool, error) {
		var err error
		if row, err = Resolve(rows, in.Piece, cwdProject); err != nil {
			return false, err
		}
		changed = applyNote(st, row.Key, text)
		return changed, nil
	})
	if err != nil {
		return NoteResult{}, err
	}
	if changed {
		h.record("inbox.noted", row, map[string]any{"note": text})
	}
	return NoteResult{Key: row.Key, Note: text}, nil
}

// applyNote writes (or, for empty text, deletes) the note; false when unchanged.
func applyNote(st *State, key, text string) bool {
	if st.Notes[key] == text {
		return false
	}
	if text == "" {
		delete(st.Notes, key)
		return true
	}
	if st.Notes == nil {
		st.Notes = map[string]string{}
	}
	st.Notes[key] = text
	return true
}

// SnoozeInput is `mp inbox snooze`: exactly one of For (duration, days
// allowed: "2d"), Until (RFC3339) or Clear.
type SnoozeInput struct {
	Piece string `json:"piece"`
	For   string `json:"for,omitempty"`
	Until string `json:"until,omitempty"`
	Clear bool   `json:"clear,omitempty"`
}

// SnoozeResult is the stdout of `mp inbox snooze`; SnoozedUntil is null after --clear.
type SnoozeResult struct {
	Key          string     `json:"key"`
	SnoozedUntil *time.Time `json:"snoozed_until"`
}

// until resolves the input to a snooze end (nil = clear).
func (in SnoozeInput) until(now time.Time) (*time.Time, error) {
	set := 0
	for _, on := range []bool{in.For != "", in.Until != "", in.Clear} {
		if on {
			set++
		}
	}
	if set != 1 {
		return nil, ErrBadSnooze
	}
	var t time.Time
	switch {
	case in.Clear:
		return nil, nil
	case in.For != "":
		d, err := parseDuration(in.For)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBadSnooze, err)
		}
		t = now.Add(d)
	default:
		var err error
		if t, err = time.Parse(time.RFC3339, in.Until); err != nil {
			return nil, fmt.Errorf("%w: until must be RFC3339: %v", ErrBadSnooze, err)
		}
	}
	if !t.After(now) {
		return nil, fmt.Errorf("%w: %s is not in the future", ErrBadSnooze, t.Format(time.RFC3339))
	}
	t = t.UTC().Truncate(time.Second)
	return &t, nil
}

// parseDuration is time.ParseDuration plus a "d" (days) suffix.
func parseDuration(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}

// Snooze hides a piece at the bottom of the inbox until a time, or clears that.
func (h *Handler) Snooze(ctx context.Context, opts Options, in SnoozeInput) (SnoozeResult, error) {
	until, err := in.until(h.now())
	if err != nil {
		return SnoozeResult{}, err
	}
	var row Row
	changed := false
	_, err = h.collect(ctx, opts, func(st *State, rows []Row, cwdProject string) (bool, error) {
		var err error
		if row, err = Resolve(rows, in.Piece, cwdProject); err != nil {
			return false, err
		}
		changed = applySnooze(st, row.Key, until)
		return changed, nil
	})
	if err != nil {
		return SnoozeResult{}, err
	}
	if changed {
		var data any
		if until != nil {
			data = until.Format(time.RFC3339)
		}
		h.record("inbox.snoozed", row, map[string]any{"until": data})
	}
	return SnoozeResult{Key: row.Key, SnoozedUntil: until}, nil
}

// applySnooze writes (or, for nil, deletes) the snooze; false when unchanged.
func applySnooze(st *State, key string, until *time.Time) bool {
	cur, ok := st.Snoozed[key]
	if until == nil {
		if ok {
			delete(st.Snoozed, key)
		}
		return ok
	}
	if ok && cur.Equal(*until) {
		return false
	}
	if st.Snoozed == nil {
		st.Snoozed = map[string]time.Time{}
	}
	st.Snoozed[key] = *until
	return true
}

// Directions for Step.
const (
	Next = 1
	Prev = -1
)

// Step picks the row after (Next) or before (Prev) the piece whose worktree
// is currentWorktree, in the displayed order with snoozed rows excluded,
// wrapping around. Outside any listed piece, Next is the first row and Prev
// the last. false when nothing is listed.
func Step(rows []Row, currentWorktree string, dir int, now time.Time) (Row, bool) {
	var live []Row
	cur := -1
	for _, r := range rows {
		if r.Snoozed(now) {
			continue
		}
		if currentWorktree != "" && filepath.Clean(r.WorktreePath) == filepath.Clean(currentWorktree) {
			cur = len(live)
		}
		live = append(live, r)
	}
	n := len(live)
	if n == 0 {
		return Row{}, false
	}
	switch {
	case cur < 0 && dir > 0:
		return live[0], true
	case cur < 0:
		return live[n-1], true
	}
	return live[((cur+dir)%n+n)%n], true
}

func (h *Handler) record(event string, row Row, data map[string]any) {
	history.Record(h.deps.Output, history.Event{Event: event, Project: row.Project, Piece: row.Piece, Branch: row.Branch, Data: data})
}
