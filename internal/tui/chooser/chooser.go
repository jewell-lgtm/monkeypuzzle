// Package chooser is a tiny single-select Bubble Tea list: a title, an optional
// detail line per option, arrow-key navigation, enter to pick, esc to cancel.
package chooser

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// Option is one selectable entry.
type Option struct {
	Label string // shown in the list
	Desc  string // optional dimmed detail under the label when highlighted
	Value string // returned to the caller
}

// Model is the Bubble Tea model.
type Model struct {
	Title     string
	Lines     []string // optional context lines shown above the options
	Options   []Option
	Selected  int
	Cancelled bool
}

func New(title string, lines []string, options []Option) Model {
	return Model{Title: title, Lines: lines, Options: options}
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
	if m.Title != "" {
		b.WriteString(styles.Title.Render(m.Title))
		b.WriteString("\n")
	}
	for _, l := range m.Lines {
		b.WriteString(styles.Subtle.Render(l))
		b.WriteString("\n")
	}
	b.WriteString("\n")
	for i, opt := range m.Options {
		cursor := "  "
		label := opt.Label
		if i == m.Selected {
			cursor = styles.Cursor.Render("→ ")
			label = styles.Selected.Render(label)
		}
		b.WriteString(fmt.Sprintf("%s%s\n", cursor, label))
		if i == m.Selected && opt.Desc != "" {
			b.WriteString("    ")
			b.WriteString(styles.Subtle.Render(opt.Desc))
			b.WriteString("\n")
		}
	}
	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("↑/↓ move • enter select • esc cancel"))
	return b.String()
}

// Chosen returns the highlighted option, or false if the chooser was cancelled.
func (m Model) Chosen() (Option, bool) {
	if m.Cancelled || len(m.Options) == 0 || m.Selected < 0 || m.Selected >= len(m.Options) {
		return Option{}, false
	}
	return m.Options[m.Selected], true
}

// Run shows the chooser and returns the picked option's Value, or ("", false)
// if the user cancelled.
func Run(title string, lines []string, options []Option) (string, bool, error) {
	p := tea.NewProgram(New(title, lines, options))
	m, err := p.Run()
	if err != nil {
		return "", false, err
	}
	opt, ok := m.(Model).Chosen()
	if !ok {
		return "", false, nil
	}
	return opt.Value, true, nil
}
