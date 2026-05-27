// Package dashboard renders the cross-project monkeypuzzle dashboard: a flat,
// always-expanded list of every registered project and its piece worktrees, with
// fuzzy-filterable rows for open issues and unadopted branches the user can
// turn into pieces on the fly.
package dashboard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/fuzzy"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// MaxVisibleRows caps how many rows the picker shows after fuzzy filtering.
// Anything past this prints as "… N more" so the picker stays scannable on
// dense setups.
const MaxVisibleRows = 20

// RowKind distinguishes a project's main worktree row from a piece row.
type RowKind int

const (
	RowProject RowKind = iota
	RowPiece
	RowIssue
	RowBranch
	RowNewPiece
)

// Row is one selectable line in the dashboard.
type Row struct {
	Kind         RowKind
	Project      string
	ProjectPath  string
	Piece        string // empty for non-piece rows
	WorktreePath string
	SessionName  string
	HasSession   bool

	// Issue-row fields (used when Kind == RowIssue).
	IssuePath   string
	IssueTitle  string
	IssueNumber string

	// Branch-row field (used when Kind == RowBranch). Also reused for the
	// project's current branch on RowProject rows for display.
	Branch string

	// Display-only metadata for RowProject rows.
	PieceCount int
	OpenIssues int
	Missing    bool // repo no longer on disk
}

// Model is the Bubble Tea model for the dashboard.
type Model struct {
	Rows     []Row
	Filtered []int // indices into Rows that pass the current filter
	Selected int   // index into Filtered
	Input    textinput.Model

	Cancelled bool
}

func New(rows []Row) Model {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()

	m := Model{Rows: rows, Input: ti}
	m.refilter()
	return m
}

func (m Model) Init() tea.Cmd { return textinput.Blink }

func (m *Model) refilter() {
	q := strings.TrimSpace(m.Input.Value())
	m.Filtered = m.Filtered[:0]
	for i, r := range m.Rows {
		if matchesQuery(r, q) {
			m.Filtered = append(m.Filtered, i)
		}
	}
	if m.Selected >= len(m.Filtered) {
		m.Selected = 0
	}
}

// matchesQuery returns true if the row should be visible under the given fuzzy
// query. Empty query passes everything.
func matchesQuery(r Row, q string) bool {
	if q == "" {
		return true
	}
	return fuzzy.Match(q, rowHaystack(r))
}

// rowHaystack flattens the searchable text of a row into one lowercased string.
// RowNewPiece rows match only on the project name — the literal "+ new piece"
// label is intentionally excluded from the haystack so a stray subsequence match
// (e.g. fuzzy "new" inside "feature-name") doesn't surface every project's
// create row.
func rowHaystack(r Row) string {
	if r.Kind == RowNewPiece {
		return strings.ToLower(r.Project)
	}
	parts := []string{r.Project, r.Piece, r.Branch, r.IssueTitle, r.IssuePath, r.IssueNumber}
	return strings.ToLower(strings.Join(parts, " "))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.Filtered) == 0 {
				m.Cancelled = true
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			if m.Selected > 0 {
				m.Selected--
			}
			return m, nil
		case "down", "ctrl+n":
			limit := len(m.Filtered)
			if limit > MaxVisibleRows {
				limit = MaxVisibleRows
			}
			if m.Selected < limit-1 {
				m.Selected++
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	m.refilter()
	return m, cmd
}

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	var b strings.Builder
	b.WriteString(styles.Title.Render("monkeypuzzle"))
	b.WriteString("\n")
	b.WriteString(m.Input.View())
	b.WriteString("\n\n")

	if len(m.Rows) == 0 {
		b.WriteString(styles.Subtle.Render("No registered projects. Run `mp init` in a repo, or `mp project add <path>`.\n"))
		return b.String()
	}

	if len(m.Filtered) == 0 {
		b.WriteString(styles.Subtle.Render("No matches.\n"))
		return b.String()
	}

	visible := len(m.Filtered)
	truncated := 0
	if visible > MaxVisibleRows {
		truncated = visible - MaxVisibleRows
		visible = MaxVisibleRows
	}

	for vi := 0; vi < visible; vi++ {
		r := m.Rows[m.Filtered[vi]]
		cursor := "  "
		if vi == m.Selected {
			cursor = styles.Cursor.Render("→ ")
		}
		b.WriteString(cursor)
		b.WriteString(renderRow(r, vi == m.Selected))
		b.WriteString("\n")
	}

	if truncated > 0 {
		b.WriteString("\n")
		b.WriteString(styles.Subtle.Render(fmt.Sprintf("… %d more (narrow your query)", truncated)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("type to filter • ↑/↓ move • enter select • esc cancel"))
	return b.String()
}

func renderRow(r Row, selected bool) string {
	switch r.Kind {
	case RowProject:
		label := r.Project
		if selected {
			label = styles.Selected.Render(label)
		}
		meta := []string{}
		if r.Missing {
			meta = append(meta, "missing")
		} else {
			if r.Branch != "" {
				meta = append(meta, r.Branch)
			}
			meta = append(meta, fmt.Sprintf("%d pieces", r.PieceCount))
			meta = append(meta, fmt.Sprintf("%d open issues", r.OpenIssues))
		}
		return fmt.Sprintf("%s  %s", label, styles.Subtle.Render("("+strings.Join(meta, " · ")+")"))
	case RowPiece:
		name := r.Piece
		if selected {
			name = styles.Selected.Render(name)
		}
		indicator := ""
		if r.HasSession {
			indicator = styles.Subtle.Render(" [tmux]")
		}
		return fmt.Sprintf("  %s%s", name, indicator)
	case RowIssue:
		title := r.IssueTitle
		if title == "" {
			title = r.IssuePath
		}
		if selected {
			title = styles.Selected.Render(title)
		}
		tag := "issue"
		if r.IssueNumber != "" {
			tag = "issue " + r.IssueNumber
		}
		return fmt.Sprintf("  %s %s", styles.Subtle.Render("["+tag+"]"), title)
	case RowBranch:
		name := r.Branch
		if selected {
			name = styles.Selected.Render(name)
		}
		return fmt.Sprintf("  %s %s", styles.Subtle.Render("[branch]"), name)
	case RowNewPiece:
		label := "+ new piece"
		if selected {
			label = styles.Selected.Render(label)
		}
		return "  " + label
	}
	return ""
}

// SelectedRow returns the highlighted row, or false if the model was cancelled
// or empty.
func (m Model) SelectedRow() (Row, bool) {
	if m.Cancelled || len(m.Filtered) == 0 || m.Selected < 0 || m.Selected >= len(m.Filtered) {
		return Row{}, false
	}
	idx := m.Filtered[m.Selected]
	if idx < 0 || idx >= len(m.Rows) {
		return Row{}, false
	}
	return m.Rows[idx], true
}
