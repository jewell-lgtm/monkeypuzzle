// Package dashboard renders the cross-project monkeypuzzle dashboard: a
// fuzzy-filterable list of every registered project, its piece worktrees, and
// unadopted branches the user can turn into pieces on the fly.
package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/fuzzy"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

// MaxVisibleRows is the fallback height of the scrolling window used before the
// terminal size is known. Once a WindowSizeMsg arrives the window grows or
// shrinks to fit the screen (see Model.windowSize).
const MaxVisibleRows = 20

// dashboardChrome is the number of non-row lines the view reserves (banner,
// blank lines, the scroll hints, the filter input, and the help line) when
// sizing the scrolling window to the terminal height.
const dashboardChrome = 7

// minVisibleRows keeps at least a few rows visible on very short terminals.
const minVisibleRows = 3

// RowKind distinguishes a project's main worktree row from a piece row.
type RowKind int

const (
	RowProject RowKind = iota
	RowPiece
	RowBranch
	RowNewPiece
)

// Row is one selectable line in the dashboard.
type Row struct {
	Kind         RowKind
	Project      string
	ProjectPath  string
	Piece        string // empty for non-piece rows
	WorktreePath string
	SessionName  string
	HasSession   bool

	// Branch-row field (used when Kind == RowBranch). Also reused for the
	// project's current branch on RowProject rows for display.
	Branch string
	// Remote marks a RowBranch as a remote-only ref (e.g. "origin/foo") with no
	// local branch yet. Adopting it fetches the remote and creates a tracking
	// worktree.
	Remote bool

	// Display-only metadata for RowProject rows.
	PieceCount int
	Missing    bool // repo no longer on disk
}

// RowsMsg delivers the collected rows to a loading dashboard (see NewLoading).
// A non-nil Err means collection failed; the program quits and the caller
// surfaces it via Model.Err.
type RowsMsg struct {
	Rows []Row
	Err  error
}

// spinnerTickMsg advances the loading spinner one frame.
type spinnerTickMsg struct{}

// spinnerFrames is a braille spinner; spinnerInterval is its frame rate.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

const spinnerInterval = 80 * time.Millisecond

func spinnerTick() tea.Cmd {
	return tea.Tick(spinnerInterval, func(time.Time) tea.Msg { return spinnerTickMsg{} })
}

// Model is the Bubble Tea model for the dashboard.
type Model struct {
	Rows     []Row
	Filtered []int // indices into Rows that pass the current filter
	Selected int   // index into Filtered
	Input    textinput.Model

	// SessionLabel names the multiplexer behind HasSession rows ("tmux",
	// "zellij", ...); empty renders as the generic "session".
	SessionLabel string

	// Scrolling. offset is the index of the first visible filtered row; height
	// is the last known terminal height (0 until the first WindowSizeMsg).
	offset int
	height int

	// Expand/collapse. collapsed[projectKey] hides a project's child rows
	// (pieces/branches/new-piece) until expanded; hasChild marks which
	// projects have any children to expand. A non-empty filter query overrides
	// collapse so search can reveal children anywhere.
	collapsed map[string]bool
	hasChild  map[string]bool

	// Loading state. When Loading is true the picker shows a spinner until a
	// RowsMsg arrives; loadCmd is the command that produces it. Err captures a
	// collection failure for the caller to return after the program exits.
	Loading bool
	frame   int
	loadCmd tea.Cmd
	Err     error

	Cancelled bool
}

// windowSize returns how many rows fit on screen: the terminal height minus the
// fixed chrome, falling back to MaxVisibleRows before the size is known.
func (m Model) windowSize() int {
	if m.height <= 0 {
		return MaxVisibleRows
	}
	n := m.height - dashboardChrome
	if n < minVisibleRows {
		return minVisibleRows
	}
	return n
}

// clampScroll keeps Selected in range and slides the visible window so the
// selection stays on screen.
func (m *Model) clampScroll() {
	if len(m.Filtered) == 0 {
		m.Selected, m.offset = 0, 0
		return
	}
	if m.Selected < 0 {
		m.Selected = 0
	}
	if m.Selected > len(m.Filtered)-1 {
		m.Selected = len(m.Filtered) - 1
	}
	win := m.windowSize()
	if m.Selected < m.offset {
		m.offset = m.Selected
	}
	if m.Selected >= m.offset+win {
		m.offset = m.Selected - win + 1
	}
	if maxOffset := len(m.Filtered) - win; m.offset > maxOffset {
		m.offset = maxOffset
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

// projectKey identifies the project a row belongs to (ProjectPath when set, else
// the project name). All rows of a project share the same key.
func projectKey(r Row) string {
	if r.ProjectPath != "" {
		return r.ProjectPath
	}
	return r.Project
}

// initCollapse derives the default expand state from the current rows: with more
// than one project everything starts collapsed (a clean one-row-per-repo
// switcher); a single project stays expanded so bare `mp` shows its detail.
func (m *Model) initCollapse() {
	m.collapsed = map[string]bool{}
	m.hasChild = map[string]bool{}
	projects := make(map[string]bool)
	for _, r := range m.Rows {
		key := projectKey(r)
		if r.Kind == RowProject {
			projects[key] = true
		} else {
			m.hasChild[key] = true
		}
	}
	if len(projects) > 1 {
		for key := range projects {
			m.collapsed[key] = true
		}
	}
}

// rowVisible reports whether a row passes the collapse filter. Project rows are
// always visible; a child is hidden only when its project is collapsed.
func (m Model) rowVisible(r Row) bool {
	if r.Kind == RowProject {
		return true
	}
	return !m.collapsed[projectKey(r)]
}

// setCollapsed expands/collapses the project owning the currently selected row.
// On collapse the selection snaps back to that project's header so it stays on
// screen.
func (m *Model) setCollapsed(collapse bool) {
	r, ok := m.SelectedRow()
	if !ok {
		return
	}
	key := projectKey(r)
	if collapse && !m.hasChild[key] {
		return
	}
	m.collapsed[key] = collapse
	m.refilter()
	if collapse {
		for fi, ri := range m.Filtered {
			if pr := m.Rows[ri]; pr.Kind == RowProject && projectKey(pr) == key {
				m.Selected = fi
				break
			}
		}
		m.clampScroll()
	}
}

func newFilterInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Type to filter..."
	ti.Focus()
	return ti
}

// New builds a dashboard over already-collected rows.
func New(rows []Row) Model {
	m := Model{Rows: rows, Input: newFilterInput()}
	m.initCollapse()
	m.refilter()
	return m
}

// NewLoading builds a dashboard that shows a spinner until loadCmd delivers a
// RowsMsg with the collected rows (or an error). Collection runs inside the
// Bubble Tea program so the spinner animates and then disappears on its own —
// no stray progress lines linger above the picker.
func NewLoading(loadCmd tea.Cmd) Model {
	return Model{Input: newFilterInput(), Loading: true, loadCmd: loadCmd}
}

func (m Model) Init() tea.Cmd {
	if m.Loading {
		return tea.Batch(textinput.Blink, m.loadCmd, spinnerTick())
	}
	return textinput.Blink
}

func (m *Model) refilter() {
	q := strings.TrimSpace(m.Input.Value())
	m.Filtered = m.Filtered[:0]
	for i, r := range m.Rows {
		if !matchesQuery(r, q) {
			continue
		}
		// With no query, collapsed projects hide their children. A query
		// overrides collapse so matches surface wherever they live.
		if q == "" && !m.rowVisible(r) {
			continue
		}
		m.Filtered = append(m.Filtered, i)
	}
	if m.Selected >= len(m.Filtered) {
		m.Selected = 0
	}
	m.clampScroll()
}

// matchesQuery returns true if the row should be visible under the given fuzzy
// query. Empty query passes everything.
func matchesQuery(r Row, q string) bool {
	if q == "" {
		return true
	}
	return fuzzy.Match(q, rowHaystack(r))
}

// rowHaystack flattens the searchable text of a row into one lowercased string.
// RowNewPiece rows match only on the project name — the literal "+ new piece"
// label is intentionally excluded from the haystack so a stray subsequence match
// (e.g. fuzzy "new" inside "feature-name") doesn't surface every project's
// create row.
func rowHaystack(r Row) string {
	if r.Kind == RowNewPiece {
		return strings.ToLower(r.Project)
	}
	parts := []string{r.Project, r.Piece, r.Branch}
	return strings.ToLower(strings.Join(parts, " "))
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case RowsMsg:
		m.Loading = false
		if msg.Err != nil {
			m.Err = msg.Err
			return m, tea.Quit
		}
		m.Rows = msg.Rows
		m.initCollapse()
		m.refilter()
		return m, nil
	case spinnerTickMsg:
		if !m.Loading {
			return m, nil // stop ticking once loaded
		}
		m.frame++
		return m, spinnerTick()
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.clampScroll()
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			m.Cancelled = true
			return m, tea.Quit
		case "enter":
			if m.Loading {
				return m, nil // ignore until rows arrive
			}
			if len(m.Filtered) == 0 {
				m.Cancelled = true
			}
			return m, tea.Quit
		case "up", "ctrl+p":
			m.Selected--
			m.clampScroll()
			return m, nil
		case "down", "ctrl+n":
			m.Selected++
			m.clampScroll()
			return m, nil
		case "pgup":
			m.Selected -= m.windowSize()
			m.clampScroll()
			return m, nil
		case "pgdown":
			m.Selected += m.windowSize()
			m.clampScroll()
			return m, nil
		case "right", "tab":
			m.setCollapsed(false)
			return m, nil
		case "left", "shift+tab":
			m.setCollapsed(true)
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.Input, cmd = m.Input.Update(msg)
	m.refilter()
	return m, cmd
}

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	var b strings.Builder
	// Banner in the brand color (distinct from the pink used for the selected
	// row) so it doesn't read as another repo you could switch to.
	b.WriteString(styles.Brand.Render("monkeypuzzle"))
	b.WriteString("\n\n")

	// Body: the result rows, or a status line when there's nothing to show.
	switch {
	case m.Loading:
		frame := spinnerFrames[m.frame%len(spinnerFrames)]
		b.WriteString(styles.Subtle.Render(frame + " Loading…"))
		b.WriteString("\n")
	case len(m.Rows) == 0:
		b.WriteString(styles.Subtle.Render("No registered projects. Run `mp init` in a repo, or `mp project add <path>`."))
		b.WriteString("\n")
	case len(m.Filtered) == 0:
		b.WriteString(styles.Subtle.Render("No matches."))
		b.WriteString("\n")
	default:
		start := m.offset
		end := start + m.windowSize()
		if end > len(m.Filtered) {
			end = len(m.Filtered)
		}

		// Expand markers only make sense for the collapsed tree; a search query
		// flattens results, so suppress them then.
		showMarkers := strings.TrimSpace(m.Input.Value()) == ""

		if start > 0 {
			b.WriteString(styles.Subtle.Render(fmt.Sprintf("  ↑ %d more", start)))
			b.WriteString("\n")
		}
		for i := start; i < end; i++ {
			r := m.Rows[m.Filtered[i]]
			cursor := "  "
			if i == m.Selected {
				cursor = styles.Cursor.Render("→ ")
			}
			b.WriteString(cursor)
			if showMarkers && r.Kind == RowProject && m.hasChild[projectKey(r)] {
				if m.collapsed[projectKey(r)] {
					b.WriteString(styles.Subtle.Render("▸ "))
				} else {
					b.WriteString(styles.Subtle.Render("▾ "))
				}
			}
			b.WriteString(renderRow(r, i == m.Selected, m.sessionLabel()))
			b.WriteString("\n")
		}
		if end < len(m.Filtered) {
			b.WriteString(styles.Subtle.Render(fmt.Sprintf("  ↓ %d more", len(m.Filtered)-end)))
			b.WriteString("\n")
		}
	}

	// Filter input sits at the bottom, just above the help line.
	b.WriteString("\n")
	b.WriteString(m.Input.View())
	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("type to filter • ↑/↓ move • →/← expand • enter select • esc cancel"))
	return b.String()
}

func (m Model) sessionLabel() string {
	if m.SessionLabel != "" {
		return m.SessionLabel
	}
	return "session"
}

func renderRow(r Row, selected bool, sessionLabel string) string {
	switch r.Kind {
	case RowProject:
		label := r.Project
		if selected {
			label = styles.Selected.Render(label)
		}
		meta := []string{}
		if r.Missing {
			meta = append(meta, "missing")
		} else {
			if r.Branch != "" {
				meta = append(meta, r.Branch)
			}
			meta = append(meta, fmt.Sprintf("%d pieces", r.PieceCount))
		}
		return fmt.Sprintf("%s  %s", label, styles.Subtle.Render("("+strings.Join(meta, " · ")+")"))
	case RowPiece:
		name := r.Piece
		if selected {
			name = styles.Selected.Render(name)
		}
		indicator := ""
		if r.HasSession {
			indicator = styles.Subtle.Render(" [" + sessionLabel + "]")
		}
		return fmt.Sprintf("  %s%s", name, indicator)
	case RowBranch:
		name := r.Branch
		if selected {
			name = styles.Selected.Render(name)
		}
		tag := "branch"
		if r.Remote {
			tag = "remote"
		}
		return fmt.Sprintf("  %s %s", styles.Subtle.Render("["+tag+"]"), name)
	case RowNewPiece:
		label := "+ new piece"
		if selected {
			label = styles.Selected.Render(label)
		}
		return "  " + label
	}
	return ""
}

// SelectedRow returns the highlighted row, or false if the model was cancelled
// or empty.
func (m Model) SelectedRow() (Row, bool) {
	if m.Cancelled || len(m.Filtered) == 0 || m.Selected < 0 || m.Selected >= len(m.Filtered) {
		return Row{}, false
	}
	idx := m.Filtered[m.Selected]
	if idx < 0 || idx >= len(m.Rows) {
		return Row{}, false
	}
	return m.Rows[idx], true
}
