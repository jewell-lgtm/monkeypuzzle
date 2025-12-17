package init

import (
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
		case "up", "k":
			m = m.moveCursor(-1)
		case "down", "j":
			m = m.moveCursor(1)
		}
	}

	if m.Step == StepProjectName {
		var cmd tea.Cmd
		m.ProjectName, cmd = m.ProjectName.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) moveCursor(dir int) Model {
	switch m.Step {
	case StepIssueMethod:
		m.IssueMethod += dir
		if m.IssueMethod < 0 {
			m.IssueMethod = 0
		}
		if m.IssueMethod > 0 {
			m.IssueMethod = 0
		}
	case StepPRMethod:
		m.PRMethod += dir
		if m.PRMethod < 0 {
			m.PRMethod = 0
		}
		if m.PRMethod > 0 {
			m.PRMethod = 0
		}
	}
	return m
}

func (m Model) nextStep() (tea.Model, tea.Cmd) {
	switch m.Step {
	case StepProjectName:
		m.Step = StepIssueMethod
	case StepIssueMethod:
		m.Step = StepPRMethod
	case StepPRMethod:
		m.Step = StepConfirm
	case StepConfirm:
		m.Step = StepDone
		return m, tea.Quit
	}
	return m, nil
}
