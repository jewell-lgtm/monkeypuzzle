// Package configwizard implements the first-run interactive wizard that
// populates the user config when it's missing.
package configwizard

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// Choice represents a multiplexer option presented in the wizard.
type Choice struct {
	Value       string
	Label       string
	Description string
}

// Choices are the multiplexer options offered by the wizard, in display order.
// Maybe one day this grows a zellij choice and friends.
var Choices = []Choice{
	{Value: "tmux", Label: "tmux", Description: "Manage piece sessions with tmux"},
	{Value: "none", Label: "none", Description: "No session management — print paths for `cd $(mp switch ...)`"},
}

// Model is the Bubble Tea model for the wizard.
type Model struct {
	selected  int
	Chosen    string
	Cancelled bool
}

// New returns a wizard with an initial selection inferred from the current
// environment ($TMUX).
func New() Model {
	return Model{selected: defaultSelection()}
}

func defaultSelection() int {
	if os.Getenv("TMUX") != "" {
		return 0
	}
	return 1
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc", "q":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			m.Chosen = Choices[m.selected].Value
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(Choices)-1 {
				m.selected++
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	var b strings.Builder
	b.WriteString(styles.Title.Render("Welcome to monkeypuzzle"))
	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("Choose how mp should manage piece sessions. You can change this later with `mp config set multiplexer <value>`."))
	b.WriteString("\n\n")

	for i, c := range Choices {
		cursor := "  "
		label := c.Label
		if i == m.selected {
			cursor = styles.Cursor.Render("→ ")
			label = styles.Selected.Render(label)
		}
		b.WriteString(fmt.Sprintf("%s%s — %s\n", cursor, label, styles.Subtle.Render(c.Description)))
	}

	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("↑/↓ to move • enter to select • esc to cancel"))
	return b.String()
}
