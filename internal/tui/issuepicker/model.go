package issuepicker

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

type Model struct {
	Issues    []issue.IssueListItem
	Selected  int
	Cancelled bool
}

func New(issues []issue.IssueListItem) Model {
	return Model{
		Issues:   issues,
		Selected: 0,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}
