package dashboard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func sampleRows() []Row {
	return []Row{
		{Kind: RowProject, Project: "alpha"},
		{Kind: RowNewPiece, Project: "alpha", ProjectPath: "/repos/alpha"},
		{Kind: RowPiece, Project: "alpha", Piece: "feature-x"},
		{Kind: RowIssue, Project: "alpha", IssuePath: "issues/wire-the-picker.md", IssueTitle: "Wire the picker"},
		{Kind: RowBranch, Project: "alpha", Branch: "stray-spike"},
		{Kind: RowProject, Project: "bravo"},
		{Kind: RowNewPiece, Project: "bravo", ProjectPath: "/repos/bravo"},
	}
}

func sendKey(m Model, s string) Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)})
	return updated.(Model)
}

func TestNew_EmptyQuery_ShowsAllRows(t *testing.T) {
	m := New(sampleRows())
	if got := len(m.Filtered); got != 7 {
		t.Fatalf("len(Filtered) = %d, want 7", got)
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
	m = sendKey(m, "wire")
	if got := len(m.Filtered); got != 1 {
		t.Fatalf("after 'wire' query: len(Filtered)=%d, want 1; rows=%v", got, m.Filtered)
	}
	row, ok := m.SelectedRow()
	if !ok {
		t.Fatal("expected a selected row")
	}
	if row.Kind != RowIssue {
		t.Errorf("matched row kind = %v, want RowIssue", row.Kind)
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

func TestMaxVisibleRows_TruncatesFilteredList(t *testing.T) {
	rows := make([]Row, 0, MaxVisibleRows+5)
	for i := 0; i < cap(rows); i++ {
		rows = append(rows, Row{Kind: RowIssue, Project: "alpha", IssueTitle: "issue", IssuePath: "issues/i.md"})
	}
	m := New(rows)
	view := m.View()
	want := "… 5 more"
	if !contains(view, want) {
		t.Errorf("expected view to contain %q, view:\n%s", want, view)
	}
}

func TestSelection_BoundedToVisibleSlice(t *testing.T) {
	rows := make([]Row, 0, MaxVisibleRows+10)
	for i := 0; i < cap(rows); i++ {
		rows = append(rows, Row{Kind: RowIssue, Project: "alpha", IssueTitle: "x", IssuePath: "p"})
	}
	m := New(rows)
	for i := 0; i < MaxVisibleRows*2; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = updated.(Model)
	}
	if m.Selected >= MaxVisibleRows {
		t.Errorf("Selected = %d, want < MaxVisibleRows (%d)", m.Selected, MaxVisibleRows)
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
