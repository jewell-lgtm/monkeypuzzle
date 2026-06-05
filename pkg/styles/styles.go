package styles

import "github.com/charmbracelet/lipgloss"

var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

	// Brand is the app banner color. It is deliberately distinct from Selected /
	// Cursor (both pink, 205) so the "monkeypuzzle" header never reads as a
	// selectable repo row — especially in this repo, whose project is also named
	// "monkeypuzzle".
	Brand = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("39"))

	Label = lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	Subtle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("241"))

	Selected = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	Cursor = lipgloss.NewStyle().
		Foreground(lipgloss.Color("205"))

	Success = lipgloss.NewStyle().
		Foreground(lipgloss.Color("82"))
)
