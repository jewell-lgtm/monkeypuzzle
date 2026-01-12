package issuepicker

import tea "github.com/charmbracelet/bubbletea"

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case IssuesLoadedMsg:
		// Handle async search result
		return m.handleIssuesLoaded(msg)

	case DebounceTickMsg:
		// Handle debounce timer expiry
		return m.handleDebounceTick(msg)

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			if len(m.Filtered) > 0 && !m.Loading {
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

	// Handle query change
	if m.Input.Value() != prevQuery {
		return m.handleQueryChange(cmd)
	}

	return m, cmd
}

// handleIssuesLoaded processes async search results
func (m Model) handleIssuesLoaded(msg IssuesLoadedMsg) (Model, tea.Cmd) {
	// Ignore stale results (query changed while loading)
	if msg.Query != m.Input.Value() {
		return m, nil
	}

	m.Loading = false

	if msg.Err != nil {
		m.Error = msg.Err.Error()
		return m, nil
	}

	m.Error = ""

	// Update cache
	if m.Cache != nil {
		m.Cache.Set(msg.Query, msg.Issues)
	}

	// Update issues and re-filter
	m.AllIssues = msg.Issues
	m.Filtered = filterIssues(msg.Issues, msg.Query)
	m.Selected = 0

	return m, nil
}

// handleDebounceTick processes debounce timer expiry
func (m Model) handleDebounceTick(msg DebounceTickMsg) (Model, tea.Cmd) {
	// Ignore if this isn't the latest debounce or query changed
	if msg.ID != m.DebounceID || msg.Query != m.Input.Value() {
		return m, nil
	}

	// Check cache first
	if m.Cache != nil {
		if cached, ok := m.Cache.Get(msg.Query); ok {
			m.AllIssues = cached
			m.Filtered = filterIssues(cached, msg.Query)
			m.Selected = 0
			return m, nil
		}
	}

	// Trigger async search
	if m.SearchFunc != nil {
		m.Loading = true
		m.LastQuery = msg.Query
		return m, m.SearchFunc(msg.Query)
	}

	return m, nil
}

// handleQueryChange processes text input changes
func (m Model) handleQueryChange(inputCmd tea.Cmd) (Model, tea.Cmd) {
	query := m.Input.Value()

	// If no async search, just filter locally
	if !m.HasAsyncSearch() {
		m.Filtered = filterIssues(m.AllIssues, query)
		m.Selected = 0
		return m, inputCmd
	}

	// Check cache for instant results
	if m.Cache != nil {
		if cached, ok := m.Cache.Get(query); ok {
			m.AllIssues = cached
			m.Filtered = filterIssues(cached, query)
			m.Selected = 0
			return m, inputCmd
		}
	}

	// Start debounce timer
	m.DebounceID++
	m.Error = ""

	// Meanwhile, filter current issues locally for immediate feedback
	m.Filtered = filterIssues(m.AllIssues, query)
	m.Selected = 0

	return m, tea.Batch(inputCmd, debounceCmd(m.DebounceID, query))
}
