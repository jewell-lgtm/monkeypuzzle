package promptinput

import (
	"fmt"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// Model is a Bubble Tea model for entering a prompt string.
type Model struct {
	Input     textinput.Model
	Value     string
	Cancelled bool
	Done      bool
}

func New() Model {
	ti := textinput.New()
	ti.Placeholder = "describe the work..."
	ti.CharLimit = 200
	ti.Width = 60
	ti.Focus()
	return Model{Input: ti}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			m.Value = m.Input.Value()
			if m.Value != "" {
				m.Done = true
				return m, tea.Quit
			}
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		styles.Title.Render("Enter prompt"),
		m.Input.View(),
		styles.Subtle.Render("enter to create • esc to cancel"),
	)
}
