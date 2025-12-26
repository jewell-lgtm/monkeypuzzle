package issue

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			return m.nextStep()
		}
	}

	// Update the active text input
	var cmd tea.Cmd
	switch m.Step {
	case StepTitle:
		m.Title, cmd = m.Title.Update(msg)
	case StepDescription:
		m.Description, cmd = m.Description.Update(msg)
	}

	return m, cmd
}

func (m Model) nextStep() (tea.Model, tea.Cmd) {
	switch m.Step {
	case StepTitle:
		// Validate title is not empty
		if m.Title.Value() == "" {
			return m, nil // Stay on this step
		}
		m.Step = StepDescription
		m.Title.Blur()
		m.Description.Focus()
		return m, textinput.Blink
	case StepDescription:
		m.Step = StepConfirm
		m.Description.Blur()
	case StepConfirm:
		m.Step = StepDone
		return m, tea.Quit
	}
	return m, nil
}
