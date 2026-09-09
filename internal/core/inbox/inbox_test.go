package inbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
)

var t0 = time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)

func TestDeriveUrgency(t *testing.T) {
	open := &PR{State: "open"}
	draft := &PR{State: "open", Draft: true}
	merged := &PR{State: "merged"}
	cases := []struct {
		name  string
		agent string
		pr    *PR
		want  string
	}{
		{"nothing", "", nil, UrgencyIdle},
		{"idle agent", piece.AgentIdle, nil, UrgencyIdle},
		{"working", piece.AgentWorking, nil, UrgencyWorking},
		{"done wants review", piece.AgentDone, nil, UrgencyReview},
		{"open pr wants review", "", open, UrgencyReview},
		{"draft pr stays idle", "", draft, UrgencyIdle},
		{"draft pr with working agent", piece.AgentWorking, draft, UrgencyWorking},
		{"blocked beats open pr", piece.AgentBlocked, open, UrgencyBlocked},
		{"open pr beats working", piece.AgentWorking, open, UrgencyReview},
		{"merged", "", merged, UrgencyMerged},
		{"merged with working agent", piece.AgentWorking, merged, UrgencyWorking},
		{"closed pr is idle", "", &PR{State: "closed"}, UrgencyIdle},
	}
	for _, c := range cases {
		if got := deriveUrgency(c.agent, c.pr); got != c.want {
			t.Errorf("%s: want %s, got %s", c.name, c.want, got)
		}
	}
}

func row(key, urgency string, age time.Duration) Row {
	return Row{Key: key, Urgency: urgency, UpdatedAt: t0.Add(-age)}
}

func keys(rows []Row) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Key
	}
	return out
}

func assertKeys(t *testing.T, rows []Row, want ...string) {
	t.Helper()
	got := keys(rows)
	if len(got) != len(want) {
		t.Fatalf("want %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("want %v, got %v", want, got)
		}
	}
}

func TestAssignRanks_RankedThenUnrankedByUrgencyAndAge(t *testing.T) {
	rows := []Row{
		row("a/old-idle", UrgencyIdle, 3*time.Hour),
		row("a/ranked-2", UrgencyIdle, time.Hour),
		row("a/new-idle", UrgencyIdle, time.Minute),
		row("a/blocked", UrgencyBlocked, 5*time.Hour),
		row("a/ranked-1", UrgencyMerged, 0),
	}
	assignRanks(rows, []string{"a/ranked-1", "a/ranked-2", "a/ranked-1"})
	assertKeys(t, rows, "a/ranked-1", "a/ranked-2", "a/blocked", "a/new-idle", "a/old-idle")
	for i, r := range rows {
		if r.Rank != i+1 {
			t.Errorf("%s: want rank %d, got %d", r.Key, i+1, r.Rank)
		}
	}
}

func TestSortRows_SnoozedLastInBothModes(t *testing.T) {
	future := t0.Add(time.Hour)
	past := t0.Add(-time.Hour)
	mk := func() []Row {
		rows := []Row{
			row("a/one", UrgencyIdle, 0),
			row("a/two", UrgencyBlocked, 0),
			row("a/three", UrgencyReview, 0),
			row("a/four", UrgencyBlocked, 0),
		}
		rows[0].SnoozedUntil = &future // snoozed: bottom regardless of rank 1
		rows[3].SnoozedUntil = &past   // expired snooze: ordinary row
		for i := range rows {
			rows[i].Rank = i + 1
		}
		return rows
	}

	rows := mk()
	sortRows(rows, SortRank, t0)
	assertKeys(t, rows, "a/two", "a/three", "a/four", "a/one")

	rows = mk()
	sortRows(rows, SortUrgency, t0)
	assertKeys(t, rows, "a/two", "a/four", "a/three", "a/one")
	if rows[3].Urgency != UrgencyIdle {
		t.Error("snoozing must not change urgency")
	}
}

func TestPruneState_DropsStaleKeys(t *testing.T) {
	st := State{
		Order:   []string{"a/live", "a/gone", "b/live"},
		Notes:   map[string]string{"a/gone": "n", "b/live": "keep"},
		Snoozed: map[string]time.Time{"a/gone": t0},
		Cache:   map[string]CacheEntry{"a/gone": {}, "a/live": {}},
	}
	present := map[string]bool{"a/live": true, "b/live": true}
	if !pruneState(&st, present) {
		t.Fatal("want changed")
	}
	if len(st.Order) != 2 || st.Order[0] != "a/live" || st.Order[1] != "b/live" {
		t.Errorf("order: %v", st.Order)
	}
	if _, ok := st.Notes["a/gone"]; ok || st.Notes["b/live"] != "keep" {
		t.Errorf("notes: %v", st.Notes)
	}
	if len(st.Snoozed) != 0 || len(st.Cache) != 1 {
		t.Errorf("snoozed=%v cache=%v", st.Snoozed, st.Cache)
	}
	if pruneState(&st, present) {
		t.Error("second prune must report no change")
	}
}

func TestCacheFresh(t *testing.T) {
	cache := map[string]CacheEntry{
		"a/fresh": {FetchedAt: t0.Add(-cacheTTL + time.Second)},
		"a/stale": {FetchedAt: t0.Add(-cacheTTL)},
	}
	if !cacheFresh(cache, []string{"a/fresh"}, t0) {
		t.Error("fresh entry rejected")
	}
	if cacheFresh(cache, []string{"a/fresh", "a/stale"}, t0) {
		t.Error("stale entry accepted")
	}
	if cacheFresh(cache, []string{"a/fresh", "a/new"}, t0) {
		t.Error("missing entry accepted")
	}
}

// stubProvider counts ListPRs calls; every other method is unused.
type stubProvider struct {
	pr.Provider
	calls int
	prs   []pr.PRInfo
	err   error
}

func (s *stubProvider) ListPRs(context.Context, string) ([]pr.PRInfo, error) {
	s.calls++
	return s.prs, s.err
}

func newTestHandler(stub *stubProvider, now time.Time) (*Handler, *adapters.BufferOutput) {
	out := adapters.NewBufferOutput()
	deps := core.NewDeps(adapters.NewMemoryFS(), out, adapters.NewMockExec(), nil, adapters.SetupNoopLoading())
	h := &Handler{deps: deps, now: func() time.Time { return now }}
	h.providerFor = func(string) (pr.Provider, error) { return stub, nil }
	return h, out
}

func TestProjectPRs_CacheTTLAndRefresh(t *testing.T) {
	stub := &stubProvider{prs: []pr.PRInfo{
		{Number: 12, HeadRefName: "feat/x", State: "OPEN", URL: "u12"},
		{Number: 9, HeadRefName: "old", State: "MERGED", URL: "u9"},
		{Number: 13, HeadRefName: "old", State: "OPEN", URL: "u13", Draft: true},
	}}
	h, _ := newTestHandler(stub, t0)
	proj := projectcmd.Info{Name: "api", Path: "/api"}
	items := []piece.PieceListItem{{Name: "x", Branch: "feat/x"}, {Name: "old"}, {Name: "none", Branch: "none"}}
	st := &State{}

	prs, fetched := h.projectPRs(context.Background(), st, proj, items, false, t0)
	if !fetched || stub.calls != 1 {
		t.Fatalf("first call must fetch: fetched=%v calls=%d", fetched, stub.calls)
	}
	if p := prs["api/x"]; p == nil || p.Number != 12 || p.State != "open" || p.Draft {
		t.Errorf("api/x: %+v", p)
	}
	// Reused branch name: the open PR wins over the merged one (IndexPRsByHead).
	if p := prs["api/old"]; p == nil || p.Number != 13 || !p.Draft {
		t.Errorf("api/old: %+v", p)
	}
	if prs["api/none"] != nil || st.Cache["api/none"].FetchedAt.IsZero() {
		t.Errorf("piece without PR must still be cached: %+v", st.Cache["api/none"])
	}

	// Within TTL: served from cache, no forge call.
	prs, fetched = h.projectPRs(context.Background(), st, proj, items, false, t0.Add(cacheTTL-time.Second))
	if fetched || stub.calls != 1 || prs["api/x"].Number != 12 {
		t.Fatalf("cached call: fetched=%v calls=%d", fetched, stub.calls)
	}
	// Refresh bypasses a fresh cache.
	if _, fetched = h.projectPRs(context.Background(), st, proj, items, true, t0); !fetched || stub.calls != 2 {
		t.Fatalf("refresh: fetched=%v calls=%d", fetched, stub.calls)
	}
	// Past TTL: refetched.
	if _, fetched = h.projectPRs(context.Background(), st, proj, items, false, t0.Add(cacheTTL)); !fetched || stub.calls != 3 {
		t.Fatalf("expired: fetched=%v calls=%d", fetched, stub.calls)
	}
	// A new piece invalidates the project's cache.
	more := append(items, piece.PieceListItem{Name: "fresh", Branch: "fresh"})
	if _, fetched = h.projectPRs(context.Background(), st, proj, more, false, t0.Add(cacheTTL+time.Second)); !fetched || stub.calls != 4 {
		t.Fatalf("new piece: fetched=%v calls=%d", fetched, stub.calls)
	}
}

func TestProjectPRs_ForgeFailureWarnsOnceAndLeavesPREmpty(t *testing.T) {
	stub := &stubProvider{err: pr.ErrProviderUnavailable}
	h, out := newTestHandler(stub, t0)
	st := &State{}
	items := []piece.PieceListItem{{Name: "a", Branch: "a"}, {Name: "b", Branch: "b"}}

	prs, fetched := h.projectPRs(context.Background(), st, projectcmd.Info{Name: "api", Path: "/api"}, items, false, t0)
	if fetched || len(prs) != 0 || len(st.Cache) != 0 {
		t.Fatalf("failure must not write cache: fetched=%v prs=%v cache=%v", fetched, prs, st.Cache)
	}
	if len(out.Messages) != 1 || out.Messages[0].Type != core.MsgWarning {
		t.Fatalf("want one warning, got %+v", out.Messages)
	}
	if !errors.Is(stub.err, pr.ErrProviderUnavailable) {
		t.Fatal("sentinel lost")
	}
}

func TestAnnotate_IsSnoozedEvaluatedAtNow(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	future, past := now.Add(time.Hour), now.Add(-time.Hour)
	st := State{Snoozed: map[string]time.Time{"p/future": future, "p/past": past}}
	cases := map[string]bool{"p/future": true, "p/past": false, "p/none": false}
	for key, want := range cases {
		row := Row{Key: key}
		annotate(&row, &st, now)
		if row.IsSnoozed != want {
			t.Errorf("%s: snoozed=%v want %v", key, row.IsSnoozed, want)
		}
	}
	if row := (Row{Key: "p/past"}); func() bool { annotate(&row, &st, now); return row.SnoozedUntil == nil }() {
		t.Error("expired snooze must still expose snoozed_until")
	}
}
