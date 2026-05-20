// Package dashboard renders the cross-project monkeypuzzle dashboard: a flat,
// always-expanded list of every registered project and its piece worktrees,
// from which the user can pick a tmux session to attach.
package dashboard

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// RowKind distinguishes a project's main worktree row from a piece row.
type RowKind int

const (
	RowProject RowKind = iota
	RowPiece
)

// Row is one selectable line in the dashboard.
type Row struct {
	Kind         RowKind
	Project      string
	ProjectPath  string
	Piece        string // empty for RowProject
	WorktreePath string
	SessionName  string
	HasSession   bool

	// Display-only metadata for RowProject rows.
	Branch     string
	PieceCount int
	OpenIssues int
	Missing    bool // repo no longer on disk
}

// Model is the Bubble Tea model for the dashboard.
type Model struct {
	Rows      []Row
	Selected  int
	Cancelled bool
}

func New(rows []Row) Model {
	m := Model{Rows: rows}
	// Start on the first selectable row (any row is selectable, but prefer a piece
	// if the very first project has none we still land on the project row).
	return m
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.Rows) == 0 {
				m.Cancelled = true
			}
			return m, tea.Quit
		case "up", "k":
			if m.Selected > 0 {
				m.Selected--
			}
		case "down", "j":
			if m.Selected < len(m.Rows)-1 {
				m.Selected++
			}
		case "g", "home":
			m.Selected = 0
		case "G", "end":
			m.Selected = len(m.Rows) - 1
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	var b strings.Builder
	b.WriteString(styles.Title.Render("monkeypuzzle"))
	b.WriteString("\n\n")

	if len(m.Rows) == 0 {
		b.WriteString(styles.Subtle.Render("No registered projects. Run `mp init` in a repo, or `mp project add <path>`.\n"))
		return b.String()
	}

	for i, r := range m.Rows {
		cursor := "  "
		if i == m.Selected {
			cursor = styles.Cursor.Render("→ ")
		}

		var line string
		switch r.Kind {
		case RowProject:
			label := r.Project
			if i == m.Selected {
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
			line = fmt.Sprintf("%s%s  %s", cursor, label, styles.Subtle.Render("("+strings.Join(meta, " · ")+")"))
		case RowPiece:
			name := r.Piece
			if i == m.Selected {
				name = styles.Selected.Render(name)
			}
			indicator := ""
			if r.HasSession {
				indicator = styles.Subtle.Render(" [tmux]")
			}
			line = fmt.Sprintf("%s  %s%s", cursor, name, indicator)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("↑/↓ move • enter attach • q cancel"))
	return b.String()
}

// SelectedRow returns the highlighted row, or false if the model was cancelled
// or empty.
func (m Model) SelectedRow() (Row, bool) {
	if m.Cancelled || len(m.Rows) == 0 || m.Selected < 0 || m.Selected >= len(m.Rows) {
		return Row{}, false
	}
	return m.Rows[m.Selected], true
}
