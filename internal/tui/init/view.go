package init

import (
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	switch m.Step {
	case StepProjectName:
		return m.viewProjectName()
	case StepIssueMethod:
		return m.viewIssueMethod()
	case StepPRMethod:
		return m.viewPRMethod()
	case StepConfirm:
		return m.viewConfirm()
	case StepDone:
		return m.viewDone()
	}
	return ""
}

func (m Model) viewProjectName() string {
	return fmt.Sprintf(
		"%s\n\n%s\n%s\n\n%s",
		styles.Title.Render("Monkeypuzzle Init"),
		styles.Label.Render("Project name:"),
		m.ProjectName.View(),
		styles.Subtle.Render("enter to continue • esc to cancel"),
	)
}

func (m Model) viewIssueMethod() string {
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\n%s",
		styles.Title.Render("Monkeypuzzle Init"),
		styles.Label.Render("Issue/feature management:"),
		renderOptions([]string{
			"Markdown files in .monkeypuzzle/issues/",
		}, m.IssueMethod),
		styles.Subtle.Render("enter to continue • esc to cancel"),
	)
}

func (m Model) viewPRMethod() string {
	return fmt.Sprintf(
		"%s\n\n%s\n\n%s\n\n%s",
		styles.Title.Render("Monkeypuzzle Init"),
		styles.Label.Render("PR management:"),
		renderOptions([]string{
			"GitHub via gh CLI",
		}, m.PRMethod),
		styles.Subtle.Render("enter to continue • esc to cancel"),
	)
}

func (m Model) viewConfirm() string {
	name := m.ProjectName.Value()
	if name == "" {
		name = m.ProjectName.Placeholder
	}
	return fmt.Sprintf(
		"%s\n\n%s\n  Project: %s\n  Issues:  %s\n  PR:      %s\n\n%s",
		styles.Title.Render("Monkeypuzzle Init"),
		styles.Label.Render("Configuration:"),
		name,
		"markdown",
		"github",
		styles.Subtle.Render("enter to create config • esc to cancel"),
	)
}

func (m Model) viewDone() string {
	return "" // Output handled by handler now
}

func renderOptions(options []string, selected int) string {
	var b strings.Builder
	for i, opt := range options {
		if i == selected {
			b.WriteString(styles.Cursor.Render("→ "))
			b.WriteString(styles.Selected.Render(opt))
		} else {
			b.WriteString("  ")
			b.WriteString(opt)
		}
		if i < len(options)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}
