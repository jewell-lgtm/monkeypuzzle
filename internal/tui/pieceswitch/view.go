package pieceswitch

import (
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	if len(m.Pieces) == 0 {
		return fmt.Sprintf(
			"%s\n\n%s\n",
			styles.Title.Render("Switch Piece"),
			styles.Subtle.Render("No pieces found. Use 'mp piece new' to create one."),
		)
	}

	var b strings.Builder
	b.WriteString(styles.Title.Render("Switch Piece"))
	b.WriteString("\n\n")

	for i, p := range m.Pieces {
		cursor := "  "
		if i == m.Selected {
			cursor = styles.Cursor.Render("→ ")
		}

		name := p.Name
		if i == m.Selected {
			name = styles.Selected.Render(name)
		}

		sessionIndicator := ""
		if p.HasSession {
			sessionIndicator = styles.Subtle.Render(" [tmux]")
		}

		b.WriteString(fmt.Sprintf("%s%s%s\n", cursor, name, sessionIndicator))
	}

	b.WriteString("\n")
	b.WriteString(styles.Subtle.Render("enter to switch • esc to cancel"))

	return b.String()
}
