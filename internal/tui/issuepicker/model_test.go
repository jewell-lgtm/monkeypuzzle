package issuepicker

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
)

func TestModel_AsyncSearch_DebouncesAndCaches(t *testing.T) {
	// Track search calls
	searchCalls := []string{}

	searchFn := func(query string) tea.Cmd {
		return func() tea.Msg {
			searchCalls = append(searchCalls, query)
			return IssuesLoadedMsg{
				Query: query,
				Issues: []issue.IssueListItem{
					{Title: "Result for " + query, Path: "test.md"},
				},
			}
		}
	}

	initial := []issue.IssueListItem{
		{Title: "Initial Issue", Path: "initial.md"},
	}

	m := NewWithSearch(initial, searchFn)

	// Verify initial state
	if !m.HasAsyncSearch() {
		t.Fatal("expected HasAsyncSearch to be true")
	}
	if len(m.AllIssues) != 1 {
		t.Fatalf("expected 1 initial issue, got %d", len(m.AllIssues))
	}

	// Simulate typing - should start debounce
	m.Input.SetValue("test")
	m2, cmd := m.handleQueryChange(nil)

	// Should have started debounce (DebounceID incremented)
	if m2.DebounceID != 1 {
		t.Errorf("expected DebounceID 1, got %d", m2.DebounceID)
	}

	// Command should be a batch containing debounce tick
	if cmd == nil {
		t.Fatal("expected command to be returned for debounce")
	}

	// Simulate debounce tick
	m3, cmd2 := m2.handleDebounceTick(DebounceTickMsg{ID: 1, Query: "test"})

	// Should have triggered search (Loading = true)
	if !m3.Loading {
		t.Error("expected Loading to be true after debounce")
	}
	if cmd2 == nil {
		t.Fatal("expected search command")
	}

	// Simulate search result
	resultMsg := IssuesLoadedMsg{
		Query: "test",
		Issues: []issue.IssueListItem{
			{Title: "Result for test", Path: "test.md"},
		},
	}
	m4, _ := m3.handleIssuesLoaded(resultMsg)

	// Should have updated issues and cleared loading
	if m4.Loading {
		t.Error("expected Loading to be false after results")
	}
	if len(m4.AllIssues) != 1 || m4.AllIssues[0].Title != "Result for test" {
		t.Error("expected issues to be updated with search results")
	}

	// Cache should have the result
	if m4.Cache != nil {
		cached, ok := m4.Cache.Get("test")
		if !ok {
			t.Error("expected results to be cached")
		}
		if len(cached) != 1 {
			t.Errorf("expected 1 cached result, got %d", len(cached))
		}
	}
}

func TestModel_AsyncSearch_IgnoresStaleResults(t *testing.T) {
	searchFn := func(query string) tea.Cmd {
		return func() tea.Msg {
			return IssuesLoadedMsg{Query: query, Issues: nil}
		}
	}

	m := NewWithSearch(nil, searchFn)
	m.Input.SetValue("current")
	m.Loading = true

	// Receive result for different query (stale)
	staleMsg := IssuesLoadedMsg{
		Query:  "old",
		Issues: []issue.IssueListItem{{Title: "Stale"}},
	}
	m2, _ := m.handleIssuesLoaded(staleMsg)

	// Should ignore - still loading, issues unchanged
	if !m2.Loading {
		t.Error("expected Loading to still be true (stale result ignored)")
	}
}

func TestModel_BasicFilter_NoAsyncSearch(t *testing.T) {
	issues := []issue.IssueListItem{
		{Title: "Add authentication", Path: "auth.md"},
		{Title: "Fix bug", Path: "bug.md"},
		{Title: "Add logging", Path: "log.md"},
	}

	m := New(issues)

	// Should not have async search
	if m.HasAsyncSearch() {
		t.Fatal("expected HasAsyncSearch to be false for New()")
	}

	// Filter locally
	m.Input.SetValue("add")
	m2, _ := m.handleQueryChange(nil)

	// Should have 2 filtered results
	if len(m2.Filtered) != 2 {
		t.Errorf("expected 2 filtered results, got %d", len(m2.Filtered))
	}
}

func TestModel_FilterByIssueNumber(t *testing.T) {
	issues := []issue.IssueListItem{
		{Title: "Add authentication", Path: "auth.md", Number: "ABC-123"},
		{Title: "Fix bug", Path: "bug.md", Number: "ABC-456"},
		{Title: "Add logging", Path: "log.md", Number: "DEF-789"},
	}

	m := New(issues)

	// Search by issue number should match
	m.Input.SetValue("ABC-123")
	m2, _ := m.handleQueryChange(nil)

	if len(m2.Filtered) != 1 {
		t.Errorf("expected 1 result for 'ABC-123', got %d", len(m2.Filtered))
	}
	if len(m2.Filtered) > 0 && m2.Filtered[0].Number != "ABC-123" {
		t.Errorf("expected ABC-123, got %s", m2.Filtered[0].Number)
	}

	// Partial number match should work (fuzzy)
	m.Input.SetValue("456")
	m3, _ := m.handleQueryChange(nil)

	if len(m3.Filtered) != 1 {
		t.Errorf("expected 1 result for '456', got %d", len(m3.Filtered))
	}

	// Team prefix match
	m.Input.SetValue("DEF")
	m4, _ := m.handleQueryChange(nil)

	if len(m4.Filtered) != 1 {
		t.Errorf("expected 1 result for 'DEF', got %d", len(m4.Filtered))
	}
}

func TestIssueCache_TTL(t *testing.T) {
	cache := NewIssueCache(50 * time.Millisecond)

	issues := []issue.IssueListItem{{Title: "Test"}}
	cache.Set("query", issues)

	// Should hit cache immediately
	cached, ok := cache.Get("query")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(cached) != 1 {
		t.Errorf("expected 1 cached item, got %d", len(cached))
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should miss cache
	_, ok = cache.Get("query")
	if ok {
		t.Error("expected cache miss after TTL")
	}
}
