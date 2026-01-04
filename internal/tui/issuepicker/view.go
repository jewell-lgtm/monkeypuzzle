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

	if len(m.Issues) == 0 {
		return fmt.Sprintf(
			"%s\n\n%s\n",
			styles.Title.Render("Select Issue"),
			styles.Subtle.Render("No todo issues found. Create one with 'mp issue create'."),
		)
	}

	var b strings.Builder
	b.WriteString(styles.Title.Render("Select Issue"))
	b.WriteString("\n\n")

	for i, iss := range m.Issues {
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

	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("enter to select • esc to cancel"))

	return b.String()
}
