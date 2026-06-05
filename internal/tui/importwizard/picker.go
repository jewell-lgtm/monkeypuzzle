// Package importwizard implements the interactive multi-step flow for
// `mp issue import`: pick an import source, search, pick a remote issue.
package importwizard

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// ListPicker is a simple single-select list.
type ListPicker struct {
	title     string
	options   []string
	selected  int
	Chosen    int
	Cancelled bool
	Done      bool
}

// NewListPicker creates a list picker with the given title and option labels.
func NewListPicker(title string, options []string) ListPicker {
	return ListPicker{title: title, options: options}
}

func (m ListPicker) Init() tea.Cmd { return nil }

func (m ListPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc", "q":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			m.Done = true
			m.Chosen = m.selected
			return m, tea.Quit
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.options)-1 {
				m.selected++
			}
		}
	}
	return m, nil
}

func (m ListPicker) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}
	var b strings.Builder
	b.WriteString(styles.Title.Render(m.title))
	b.WriteString("\n\n")
	for i, opt := range m.options {
		cursor := "  "
		name := opt
		if i == m.selected {
			cursor = styles.Cursor.Render("→ ")
			name = styles.Selected.Render(name)
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, name))
	}
	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("↑/↓ to move • enter to select • esc to cancel"))
	return b.String()
}

// QueryPrompt is a single-line text input for a search query.
type QueryPrompt struct {
	title     string
	input     textinput.Model
	Query     string
	Cancelled bool
	Done      bool
}

// NewQueryPrompt creates a query prompt with the given title.
func NewQueryPrompt(title string) QueryPrompt {
	ti := textinput.New()
	ti.Placeholder = "search remote issues (blank = recent)"
	ti.Focus()
	ti.Width = 50
	return QueryPrompt{title: title, input: ti}
}

func (m QueryPrompt) Init() tea.Cmd { return textinput.Blink }

func (m QueryPrompt) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if key, ok := msg.(tea.KeyMsg); ok {
		switch key.String() {
		case "ctrl+c", "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			m.Done = true
			m.Query = m.input.Value()
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m QueryPrompt) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}
	return fmt.Sprintf("%s\n\n%s\n%s\n\n%s",
		styles.Title.Render(m.title),
		styles.Label.Render("Search query:"),
		m.input.View(),
		styles.Subtle.Render("enter to search • esc to cancel"),
	)
}
