package styles

import "github.com/charmbracelet/lipgloss"

var (
	Title = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("205"))

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
