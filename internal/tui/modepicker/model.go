package modepicker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// Mode represents the selected creation mode.
type Mode int

const (
	ModeIssue Mode = iota
	ModePrompt
)

// Model is a Bubble Tea model for choosing between "From issue" and "From prompt".
type Model struct {
	Options   []string
	Selected  int
	Chosen    Mode
	Cancelled bool
	Done      bool
}

func New() Model {
	return Model{
		Options: []string{"From issue", "From prompt"},
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc", "q":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			m.Done = true
			m.Chosen = Mode(m.Selected)
			return m, tea.Quit
		case "up", "k":
			if m.Selected > 0 {
				m.Selected--
			}
		case "down", "j":
			if m.Selected < len(m.Options)-1 {
				m.Selected++
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	var b strings.Builder
	b.WriteString(styles.Title.Render("Create Piece"))
	b.WriteString("\n\n")

	for i, opt := range m.Options {
		cursor := "  "
		if i == m.Selected {
			cursor = styles.Cursor.Render("→ ")
		}

		name := opt
		if i == m.Selected {
			name = styles.Selected.Render(name)
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, name))
	}

	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("enter to select • esc to cancel"))

	return b.String()
}
