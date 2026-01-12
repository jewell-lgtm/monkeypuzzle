package issuepicker

import (
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	var b strings.Builder
	b.WriteString(styles.Title.Render("Select Issue"))
	b.WriteString("\n\n")

	// Filter input
	b.WriteString(m.Input.View())
	b.WriteString("\n\n")

	if len(m.Filtered) == 0 {
		if m.Input.Value() == "" {
			b.WriteString(styles.Subtle.Render("No todo issues found. Create one with 'mp issue create'."))
		} else {
			b.WriteString(styles.Subtle.Render("No matches found."))
		}
		b.WriteString("\n")
	} else {
		// Show count if filtering
		if m.Input.Value() != "" {
			b.WriteString(styles.Subtle.Render(fmt.Sprintf("Showing %d of %d issues", len(m.Filtered), len(m.AllIssues))))
			b.WriteString("\n\n")
		}

		// List issues (limit visible to avoid scroll issues)
		maxVisible := 10
		start := 0
		if m.Selected >= maxVisible {
			start = m.Selected - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.Filtered) {
			end = len(m.Filtered)
		}

		for i := start; i < end; i++ {
			iss := m.Filtered[i]
			cursor := "  "
			if i == m.Selected {
				cursor = styles.Cursor.Render("> ")
			}

			title := iss.Title
			if i == m.Selected {
				title = styles.Selected.Render(title)
			}

			b.WriteString(fmt.Sprintf("%s%s\n", cursor, title))
		}

		// Scroll indicator
		if len(m.Filtered) > maxVisible {
			b.WriteString(styles.Subtle.Render(fmt.Sprintf("\n  ... %d more", len(m.Filtered)-maxVisible)))
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("↑↓ navigate • enter select • esc cancel"))

	return b.String()
}
