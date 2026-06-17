package dashboard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleRows() []Row {
	// Every row of a project carries the same ProjectPath, mirroring what
	// dashboardRows produces — the collapse key is derived from it.
	return []Row{
		{Kind: RowProject, Project: "alpha", ProjectPath: "/repos/alpha"},
		{Kind: RowNewPiece, Project: "alpha", ProjectPath: "/repos/alpha"},
		{Kind: RowPiece, Project: "alpha", ProjectPath: "/repos/alpha", Piece: "feature-x"},
		{Kind: RowBranch, Project: "alpha", ProjectPath: "/repos/alpha", Branch: "stray-spike"},
		{Kind: RowProject, Project: "bravo", ProjectPath: "/repos/bravo"},
		{Kind: RowNewPiece, Project: "bravo", ProjectPath: "/repos/bravo"},
	}
}

func sendKey(m Model, s string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return updated.(Model)
}

func updateKey(m Model, kt tea.KeyType) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: kt})
	return updated.(Model)
}

func TestCollapse_MultiProjectStartsCollapsed(t *testing.T) {
	m := New(sampleRows())
	// alpha + bravo headers only — one row per repo.
	if got := len(m.Filtered); got != 2 {
		t.Fatalf("collapsed multi-project: len(Filtered)=%d, want 2", got)
	}
	for _, idx := range m.Filtered {
		if m.Rows[idx].Kind != RowProject {
			t.Errorf("expected only project rows when collapsed, got %v", m.Rows[idx].Kind)
		}
	}
}

func TestCollapse_RightExpandsSelectedProject(t *testing.T) {
	m := New(sampleRows())
	m = updateKey(m, tea.KeyRight) // expand alpha (the selected first row)
	// alpha's 4 rows + bravo header.
	if got := len(m.Filtered); got != 5 {
		t.Fatalf("after expanding alpha: len(Filtered)=%d, want 5", got)
	}
	if r, ok := m.SelectedRow(); !ok || r.Kind != RowProject || r.Project != "alpha" {
		t.Errorf("selection should stay on alpha header, got %+v ok=%v", r, ok)
	}
}

func TestCollapse_LeftCollapsesAndReselectsHeader(t *testing.T) {
	m := New(sampleRows())
	m = updateKey(m, tea.KeyRight) // expand alpha
	m = updateKey(m, tea.KeyDown)  // move into an alpha child
	m = updateKey(m, tea.KeyLeft)  // collapse alpha
	if got := len(m.Filtered); got != 2 {
		t.Fatalf("after collapsing alpha: len(Filtered)=%d, want 2", got)
	}
	if r, ok := m.SelectedRow(); !ok || r.Kind != RowProject || r.Project != "alpha" {
		t.Errorf("selection should snap back to alpha header, got %+v ok=%v", r, ok)
	}
}

func TestCollapse_QueryRevealsCollapsedChildren(t *testing.T) {
	m := New(sampleRows())    // collapsed: alpha's piece is hidden
	m = sendKey(m, "feature") // a query overrides collapse
	row, ok := m.SelectedRow()
	if !ok || row.Kind != RowPiece {
		t.Fatalf("query should surface the collapsed child piece, got %+v ok=%v", row, ok)
	}
}

func TestCollapse_SingleProjectStartsExpanded(t *testing.T) {
	rows := []Row{
		{Kind: RowProject, Project: "solo", ProjectPath: "/repos/solo"},
		{Kind: RowPiece, Project: "solo", ProjectPath: "/repos/solo", Piece: "feat"},
	}
	m := New(rows)
	if got := len(m.Filtered); got != 2 {
		t.Fatalf("single project should start expanded: len(Filtered)=%d, want 2", got)
	}
}

// TestFuzzy_NewPieceFiltersByProject covers the deliberate choice that
// RowNewPiece's haystack is just the project name — so filtering by project
// surfaces its create row alongside its other rows, without the literal label
// triggering stray subsequence matches.
func TestFuzzy_NewPieceFiltersByProject(t *testing.T) {
	m := New(sampleRows())
	m = sendKey(m, "bravo")
	for _, idx := range m.Filtered {
		r := m.Rows[idx]
		if r.Project != "bravo" {
			t.Errorf("filter 'bravo' returned row from project %q", r.Project)
		}
	}
	foundNewPiece := false
	for _, idx := range m.Filtered {
		if m.Rows[idx].Kind == RowNewPiece {
			foundNewPiece = true
		}
	}
	if !foundNewPiece {
		t.Error("expected bravo's new-piece row in filtered set")
	}
}

func TestFuzzy_FiltersAcrossRowKinds(t *testing.T) {
	m := New(sampleRows())
	m = sendKey(m, "feature")
	if got := len(m.Filtered); got != 1 {
		t.Fatalf("after 'feature' query: len(Filtered)=%d, want 1; rows=%v", got, m.Filtered)
	}
	row, ok := m.SelectedRow()
	if !ok {
		t.Fatal("expected a selected row")
	}
	if row.Kind != RowPiece {
		t.Errorf("matched row kind = %v, want RowPiece", row.Kind)
	}
}

func TestFuzzy_MatchesBranchName(t *testing.T) {
	m := New(sampleRows())
	m = sendKey(m, "stray")
	row, ok := m.SelectedRow()
	if !ok {
		t.Fatal("expected a match for 'stray'")
	}
	if row.Kind != RowBranch {
		t.Errorf("matched row kind = %v, want RowBranch", row.Kind)
	}
	if row.Branch != "stray-spike" {
		t.Errorf("matched branch = %q, want stray-spike", row.Branch)
	}
}

func TestFuzzy_NoMatch_RendersFallback(t *testing.T) {
	m := New(sampleRows())
	m = sendKey(m, "zzzzzz")
	if len(m.Filtered) != 0 {
		t.Errorf("len(Filtered) = %d, want 0", len(m.Filtered))
	}
	if _, ok := m.SelectedRow(); ok {
		t.Error("SelectedRow should not return ok when nothing matches")
	}
}

func TestScroll_ShowsHiddenBelowCount(t *testing.T) {
	rows := make([]Row, 0, MaxVisibleRows+5)
	for i := 0; i < cap(rows); i++ {
		rows = append(rows, Row{Kind: RowPiece, Project: "alpha", Piece: "piece"})
	}
	m := New(rows)
	view := m.View()
	// With no terminal size yet the window is MaxVisibleRows; the 5 extra rows
	// are below the fold and advertised as scrollable rather than truncated.
	want := "↓ 5 more"
	if !contains(view, want) {
		t.Errorf("expected view to contain %q, view:\n%s", want, view)
	}
}

func TestScroll_FollowsSelectionPastWindow(t *testing.T) {
	rows := make([]Row, 0, MaxVisibleRows+10)
	for i := 0; i < cap(rows); i++ {
		rows = append(rows, Row{Kind: RowPiece, Project: "alpha", Piece: "p"})
	}
	m := New(rows)
	const downs = MaxVisibleRows + 4
	for i := 0; i < downs; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	// Selection now moves past the old visible cap...
	if m.Selected != downs {
		t.Fatalf("Selected = %d, want %d", m.Selected, downs)
	}
	// ...and the window slides so it stays on screen...
	if m.Selected < m.offset || m.Selected >= m.offset+m.windowSize() {
		t.Errorf("Selected %d not within window [%d,%d)", m.Selected, m.offset, m.offset+m.windowSize())
	}
	// ...exposing an up-scroll hint for the rows now above the fold.
	if !contains(m.View(), "↑") {
		t.Errorf("expected an up-scroll hint after scrolling, view:\n%s", m.View())
	}
}

func TestScroll_DownStopsAtLastRow(t *testing.T) {
	rows := make([]Row, 0, MaxVisibleRows+10)
	for i := 0; i < cap(rows); i++ {
		rows = append(rows, Row{Kind: RowPiece, Project: "alpha", Piece: "p"})
	}
	m := New(rows)
	for i := 0; i < len(rows)*2; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.Selected != len(rows)-1 {
		t.Errorf("Selected = %d, want last row %d", m.Selected, len(rows)-1)
	}
}

func TestEscCancels(t *testing.T) {
	m := New(sampleRows())
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if !m.Cancelled {
		t.Error("expected Cancelled after esc")
	}
	if _, ok := m.SelectedRow(); ok {
		t.Error("SelectedRow should return false after cancel")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
