package issuepicker

import tea "github.com/charmbracelet/bubbletea"

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.Filtered) > 0 {
				return m, tea.Quit
			}
			return m, nil
		case "up", "ctrl+p":
			if m.Selected > 0 {
				m.Selected--
			}
			return m, nil
		case "down", "ctrl+n":
			if m.Selected < len(m.Filtered)-1 {
				m.Selected++
			}
			return m, nil
		}
	}

	// Update text input
	prevQuery := m.Input.Value()
	m.Input, cmd = m.Input.Update(msg)

	// Refilter if query changed
	if m.Input.Value() != prevQuery {
		m.Filtered = filterIssues(m.AllIssues, m.Input.Value())
		m.Selected = 0
	}

	return m, cmd
}
