// Package textprompt is a tiny single-line Bubble Tea prompt: a title, a text
// input pre-filled with a default, and enter/esc handling. Pressing enter on an
// empty input accepts the default. It is the interactive fallback for commands
// that take a single string (e.g. the sync source for `mp stack sync`).
package textprompt

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// Model is a Bubble Tea model for entering a single string with a default.
type Model struct {
	title     string
	Default   string
	Input     textinput.Model
	Value     string
	Cancelled bool
}

// New builds a prompt titled title, showing def as the placeholder and the
// value returned when the user accepts an empty input.
func New(title, def string) Model {
	ti := textinput.New()
	ti.Placeholder = def
	ti.CharLimit = 200
	ti.Width = 60
	ti.Focus()
	return Model{title: title, Default: def, Input: ti}
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
			m.Value = strings.TrimSpace(m.Input.Value())
			if m.Value == "" {
				m.Value = m.Default
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	return m, cmd
}

// Run shows the prompt and blocks until the user accepts or cancels. It returns
// the entered value (or def on empty input) and ok=false if the user cancelled.
func Run(title, def string) (value string, ok bool, err error) {
	p := tea.NewProgram(New(title, def))
	m, err := p.Run()
	if err != nil {
		return "", false, err
	}
	model := m.(Model)
	if model.Cancelled {
		return "", false, nil
	}
	return model.Value, true, nil
}

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	return fmt.Sprintf(
		"%s\n\n%s\n\n%s",
		styles.Title.Render(m.title),
		m.Input.View(),
		styles.Subtle.Render("enter to accept default • esc to cancel"),
	)
}
