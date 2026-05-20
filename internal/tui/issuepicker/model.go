package issuepicker

import (
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/fuzzy"
)

// SearchFn is a function that performs async issue search
type SearchFn func(query string) tea.Cmd

// IssuesLoadedMsg is sent when async search completes
type IssuesLoadedMsg struct {
	Query  string
	Issues []issue.IssueListItem
	Err    error
}

// DebounceTickMsg is sent after debounce timer expires
type DebounceTickMsg struct {
	ID    int
	Query string
}

// Model represents the issue picker TUI state
type Model struct {
	AllIssues []issue.IssueListItem // Current issue list (may be from cache or API)
	Filtered  []issue.IssueListItem // Filtered by local fuzzy match
	Selected  int
	Cancelled bool
	Input     textinput.Model
	Error     string

	// Async search support
	Loading    bool
	SearchFunc SearchFn
	Cache      *IssueCache
	DebounceID int
	LastQuery  string
}

const (
	debounceDuration = 300 * time.Millisecond
	cacheTTL         = 60 * time.Second
)

// New creates a basic issue picker (no async search).
// Use NewWithSearch for async Linear API search.
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

// NewWithSearch creates an issue picker with async search capability.
// searchFn is called when debounced search should execute.
// initialIssues are shown immediately and cached for empty query.
func NewWithSearch(initialIssues []issue.IssueListItem, searchFn SearchFn) Model {
	ti := textinput.New()
	ti.Placeholder = "Type to search..."
	ti.Focus()

	cache := NewIssueCache(cacheTTL)
	cache.Set("", initialIssues) // Cache empty query results

	return Model{
		AllIssues:  initialIssues,
		Filtered:   initialIssues,
		Selected:   0,
		Input:      ti,
		SearchFunc: searchFn,
		Cache:      cache,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}

// HasAsyncSearch returns true if this picker supports async search
func (m Model) HasAsyncSearch() bool {
	return m.SearchFunc != nil
}

// filterIssues returns issues matching the query
func filterIssues(issues []issue.IssueListItem, query string) []issue.IssueListItem {
	if query == "" {
		return issues
	}

	var filtered []issue.IssueListItem
	for _, iss := range issues {
		if fuzzy.Match(query, iss.Title) || fuzzy.Match(query, iss.Path) || fuzzy.Match(query, iss.Number) {
			filtered = append(filtered, iss)
		}
	}
	return filtered
}

// SelectedIssue returns the currently selected issue
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

// debounceCmd returns a command that fires after the debounce duration
func debounceCmd(id int, query string) tea.Cmd {
	return tea.Tick(debounceDuration, func(time.Time) tea.Msg {
		return DebounceTickMsg{ID: id, Query: query}
	})
}
