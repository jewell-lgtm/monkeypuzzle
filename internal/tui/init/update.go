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

	// Handle text input steps
	switch m.Step {
	case StepProjectName:
		var cmd tea.Cmd
		m.ProjectName, cmd = m.ProjectName.Update(msg)
		return m, cmd
	case StepLinearAPIKey:
		var cmd tea.Cmd
		m.LinearAPIKey, cmd = m.LinearAPIKey.Update(msg)
		return m, cmd
	case StepLinearTeam:
		var cmd tea.Cmd
		m.LinearTeam, cmd = m.LinearTeam.Update(msg)
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
		maxIssue := len(IssueProviders) - 1
		if m.IssueMethod > maxIssue {
			m.IssueMethod = maxIssue
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
		// If linear selected, go to API key step
		if IssueProviders[m.IssueMethod] == "linear" {
			m.Step = StepLinearAPIKey
			m.LinearAPIKey.Focus()
		} else {
			m.Step = StepPRMethod
		}
	case StepLinearAPIKey:
		m.Step = StepLinearTeam
		m.LinearTeam.Focus()
	case StepLinearTeam:
		m.Step = StepPRMethod
	case StepPRMethod:
		m.Step = StepConfirm
	case StepConfirm:
		m.Step = StepDone
		return m, tea.Quit
	}
	return m, nil
}
