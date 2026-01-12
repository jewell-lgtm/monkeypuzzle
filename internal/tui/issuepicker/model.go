package issuepicker

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

type Model struct {
	AllIssues []issue.IssueListItem // Original unfiltered list
	Filtered  []issue.IssueListItem // Filtered by query
	Selected  int
	Cancelled bool
	Input     textinput.Model
}

func New(issues []issue.IssueListItem) Model {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()

	return Model{
		AllIssues: issues,
		Filtered:  issues,
		Selected:  0,
		Input:     ti,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// fuzzyMatch returns true if query fuzzy-matches target
func fuzzyMatch(query, target string) bool {
	query = strings.ToLower(query)
	target = strings.ToLower(target)

	qi := 0
	for ti := 0; ti < len(target) && qi < len(query); ti++ {
		if target[ti] == query[qi] {
			qi++
		}
	}
	return qi == len(query)
}

// filterIssues returns issues matching the query
func filterIssues(issues []issue.IssueListItem, query string) []issue.IssueListItem {
	if query == "" {
		return issues
	}

	var filtered []issue.IssueListItem
	for _, iss := range issues {
		if fuzzyMatch(query, iss.Title) || fuzzyMatch(query, iss.Path) {
			filtered = append(filtered, iss)
		}
	}
	return filtered
}

// SelectedIssue returns the currently selected issue from the original list
func (m Model) SelectedIssue() (issue.IssueListItem, bool) {
	if m.Selected < 0 || m.Selected >= len(m.Filtered) {
		return issue.IssueListItem{}, false
	}
	return m.Filtered[m.Selected], true
}

// SelectedIndex returns the index in AllIssues of the selected filtered item
func (m Model) SelectedIndex() int {
	if m.Selected < 0 || m.Selected >= len(m.Filtered) {
		return -1
	}
	selected := m.Filtered[m.Selected]
	for i, iss := range m.AllIssues {
		if iss.Path == selected.Path {
			return i
		}
	}
	return -1
}
