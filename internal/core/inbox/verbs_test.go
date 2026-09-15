package inbox

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func fixtureRows() []Row {
	return []Row{
		{Key: "api/auth", Project: "api", Piece: "auth", WorktreePath: "/api/.mp/auth", Rank: 1},
		{Key: "web/auth", Project: "web", Piece: "auth", WorktreePath: "/web/.mp/auth", Rank: 2},
		{Key: "web/nav", Project: "web", Piece: "nav", WorktreePath: "/web/.mp/nav", Rank: 3},
	}
}

func TestResolve(t *testing.T) {
	rows := fixtureRows()
	cases := []struct {
		name, selector, cwd, want string
		err                       error
	}{
		{"project/piece", "web/auth", "", "web/auth", nil},
		{"project/piece ignores cwd", "web/auth", "api", "web/auth", nil},
		{"bare unique", "nav", "", "web/nav", nil},
		{"bare unique from other repo", "nav", "api", "web/nav", nil},
		{"bare ambiguous", "auth", "", "", ErrAmbiguous},
		{"bare prefers cwd project", "auth", "web", "web/auth", nil},
		{"bare prefers cwd project (api)", "  auth ", "api", "api/auth", nil},
		{"unknown", "nope", "web", "", ErrNoSuchPiece},
		{"unknown project/piece", "api/nav", "", "", ErrNoSuchPiece},
		{"empty", "", "", "", ErrNoSuchPiece},
	}
	for _, c := range cases {
		got, err := Resolve(rows, c.selector, c.cwd)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: want err %v, got %v", c.name, c.err, err)
			continue
		}
		if got.Key != c.want {
			t.Errorf("%s: want %q, got %q", c.name, c.want, got.Key)
		}
	}
	_, err := Resolve(rows, "auth", "")
	if err == nil || !strings.Contains(err.Error(), "api/auth, web/auth") {
		t.Errorf("ambiguous error must list candidates: %v", err)
	}
}

func TestMoveInput_Position(t *testing.T) {
	cases := []struct {
		name string
		in   MoveInput
		want Position
		err  error
	}{
		{"top", MoveInput{Top: true}, Position{Kind: PosTop}, nil},
		{"up 2", MoveInput{Up: 2}, Position{Kind: PosUp, N: 2}, nil},
		{"after", MoveInput{After: "x"}, Position{Kind: PosAfter, Target: "x"}, nil},
		{"none", MoveInput{}, Position{}, ErrBadPosition},
		{"two", MoveInput{Top: true, Bottom: true}, Position{}, ErrBadPosition},
		{"negative", MoveInput{Down: -1}, Position{}, ErrBadPosition},
	}
	for _, c := range cases {
		got, err := c.in.position()
		if !errors.Is(err, c.err) || got != c.want {
			t.Errorf("%s: want (%+v, %v), got (%+v, %v)", c.name, c.want, c.err, got, err)
		}
	}
}

func TestMoveKey(t *testing.T) {
	order := []string{"a", "b", "c", "d"}
	cases := []struct {
		name string
		key  string
		pos  Position
		want string
		err  error
	}{
		{"top", "c", Position{Kind: PosTop}, "c a b d", nil},
		{"top already", "a", Position{Kind: PosTop}, "a b c d", nil},
		{"bottom", "b", Position{Kind: PosBottom}, "a c d b", nil},
		{"up 1", "c", Position{Kind: PosUp, N: 1}, "a c b d", nil},
		{"up clamps", "b", Position{Kind: PosUp, N: 5}, "b a c d", nil},
		{"down 1", "b", Position{Kind: PosDown, N: 1}, "a c b d", nil},
		{"down clamps", "b", Position{Kind: PosDown, N: 9}, "a c d b", nil},
		{"before", "d", Position{Kind: PosBefore, Target: "b"}, "a d b c", nil},
		{"before earlier→later", "a", Position{Kind: PosBefore, Target: "d"}, "b c a d", nil},
		{"after", "a", Position{Kind: PosAfter, Target: "c"}, "b c a d", nil},
		{"after later→earlier", "d", Position{Kind: PosAfter, Target: "a"}, "a d b c", nil},
		{"before self is no-op", "b", Position{Kind: PosBefore, Target: "b"}, "a b c d", nil},
		{"after self is no-op", "b", Position{Kind: PosAfter, Target: "b"}, "a b c d", nil},
		{"unknown key", "z", Position{Kind: PosTop}, "", ErrNoSuchPiece},
		{"unknown target", "a", Position{Kind: PosAfter, Target: "z"}, "", ErrNoSuchPiece},
		{"bad kind", "a", Position{Kind: "sideways"}, "", ErrBadPosition},
	}
	for _, c := range cases {
		got, err := moveKey(order, c.key, c.pos)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: want err %v, got %v", c.name, c.err, err)
			continue
		}
		if err == nil && strings.Join(got, " ") != c.want {
			t.Errorf("%s: want %q, got %q", c.name, c.want, strings.Join(got, " "))
		}
	}
	if strings.Join(order, " ") != "a b c d" {
		t.Errorf("input mutated: %v", order)
	}
}

// Materialisation: assignRanks puts ranked keys first, then unranked rows in
// derived order, so the row keys in rank order are the order Move pins.
func TestMoveKey_MaterialisedFromRanks(t *testing.T) {
	rows := []Row{
		row("a/unranked-old", UrgencyIdle, time.Hour),
		row("a/ranked", UrgencyIdle, 0),
		row("a/unranked-blocked", UrgencyBlocked, 2*time.Hour),
	}
	assignRanks(rows, []string{"a/ranked"})
	order, err := moveKey(rowKeys(rows), "a/unranked-old", Position{Kind: PosUp, N: 1})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, " ") != "a/ranked a/unranked-old a/unranked-blocked" {
		t.Errorf("got %v", order)
	}
}

func TestApplyNote(t *testing.T) {
	st := &State{}
	if !applyNote(st, "a/x", "ship it") || st.Notes["a/x"] != "ship it" {
		t.Fatalf("set: %+v", st.Notes)
	}
	if applyNote(st, "a/x", "ship it") {
		t.Error("same text must report unchanged")
	}
	if applyNote(st, "a/y", "") {
		t.Error("clearing an absent note must report unchanged")
	}
	if !applyNote(st, "a/x", "") || len(st.Notes) != 0 {
		t.Errorf("clear: %+v", st.Notes)
	}
}

func TestSnoozeInput_Until(t *testing.T) {
	in2h := t0.Add(2 * time.Hour)
	in2d := t0.Add(48 * time.Hour)
	cases := []struct {
		name string
		in   SnoozeInput
		want *time.Time
		err  error
	}{
		{"for hours", SnoozeInput{For: "2h"}, &in2h, nil},
		{"for days", SnoozeInput{For: "2d"}, &in2d, nil},
		{"until", SnoozeInput{Until: "2026-09-09T14:00:00Z"}, &in2h, nil},
		{"until with zone", SnoozeInput{Until: "2026-09-09T16:00:00+02:00"}, &in2h, nil},
		{"clear", SnoozeInput{Clear: true}, nil, nil},
		{"none", SnoozeInput{}, nil, ErrBadSnooze},
		{"two", SnoozeInput{For: "1h", Clear: true}, nil, ErrBadSnooze},
		{"bad duration", SnoozeInput{For: "soon"}, nil, ErrBadSnooze},
		{"bad days", SnoozeInput{For: "xd"}, nil, ErrBadSnooze},
		{"negative", SnoozeInput{For: "-1h"}, nil, ErrBadSnooze},
		{"bad until", SnoozeInput{Until: "tomorrow"}, nil, ErrBadSnooze},
		{"past until", SnoozeInput{Until: "2026-09-09T11:00:00Z"}, nil, ErrBadSnooze},
	}
	for _, c := range cases {
		got, err := c.in.until(t0)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: want err %v, got %v", c.name, c.err, err)
			continue
		}
		switch {
		case c.want == nil && got != nil:
			t.Errorf("%s: want nil, got %v", c.name, got)
		case c.want != nil && (got == nil || !got.Equal(*c.want)):
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

func TestApplySnooze(t *testing.T) {
	st := &State{}
	until := t0.Add(time.Hour)
	if !applySnooze(st, "a/x", &until) || !st.Snoozed["a/x"].Equal(until) {
		t.Fatalf("set: %+v", st.Snoozed)
	}
	if applySnooze(st, "a/x", &until) {
		t.Error("same time must report unchanged")
	}
	if applySnooze(st, "a/y", nil) {
		t.Error("clearing an absent snooze must report unchanged")
	}
	if !applySnooze(st, "a/x", nil) || len(st.Snoozed) != 0 {
		t.Errorf("clear: %+v", st.Snoozed)
	}
}

func TestStep(t *testing.T) {
	future := t0.Add(time.Hour)
	rows := fixtureRows()
	rows[1].SnoozedUntil = &future // web/auth skipped
	cases := []struct {
		name, cwd string
		dir       int
		want      string
	}{
		{"next", "/api/.mp/auth", Next, "web/nav"},
		{"next wraps", "/web/.mp/nav", Next, "api/auth"},
		{"prev wraps", "/api/.mp/auth", Prev, "web/nav"},
		{"prev", "/web/.mp/nav/", Prev, "api/auth"},
		{"outside: next is first", "", Next, "api/auth"},
		{"outside: prev is last", "/elsewhere", Prev, "web/nav"},
		{"from snoozed piece: next is first", "/web/.mp/auth", Next, "api/auth"},
	}
	for _, c := range cases {
		got, ok := Step(rows, c.cwd, c.dir, t0)
		if !ok || got.Key != c.want {
			t.Errorf("%s: want %s, got %s (ok=%v)", c.name, c.want, got.Key, ok)
		}
	}

	single := rows[:1]
	if got, ok := Step(single, "/api/.mp/auth", Next, t0); !ok || got.Key != "api/auth" {
		t.Errorf("single row: want itself, got %s", got.Key)
	}
	if _, ok := Step(nil, "", Next, t0); ok {
		t.Error("empty inbox must report false")
	}
	if _, ok := Step(rows[1:2], "", Next, t0); ok {
		t.Error("all snoozed must report false")
	}
}
