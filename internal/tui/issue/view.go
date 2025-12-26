package issue

import (
	"fmt"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/styles"
)

func (m Model) View() string {
	if m.Cancelled {
		return styles.Subtle.Render("Cancelled.\n")
	}

	switch m.Step {
	case StepTitle:
		return m.viewTitle()
	case StepDescription:
		return m.viewDescription()
	case StepConfirm:
		return m.viewConfirm()
	case StepDone:
		return ""
	}
	return ""
}

func (m Model) viewTitle() string {
	return fmt.Sprintf(
		"%s\n\n%s\n%s\n\n%s",
		styles.Title.Render("Create Issue"),
		styles.Label.Render("Title:"),
		m.Title.View(),
		styles.Subtle.Render("enter to continue • esc to cancel"),
	)
}

func (m Model) viewDescription() string {
	return fmt.Sprintf(
		"%s\n\n%s\n%s\n\n%s",
		styles.Title.Render("Create Issue"),
		styles.Label.Render("Description (optional):"),
		m.Description.View(),
		styles.Subtle.Render("enter to continue • esc to cancel"),
	)
}

func (m Model) viewConfirm() string {
	title := m.Title.Value()
	desc := m.Description.Value()
	if desc == "" {
		desc = "(none)"
	}

	return fmt.Sprintf(
		"%s\n\n%s\n  Title:       %s\n  Description: %s\n\n%s",
		styles.Title.Render("Create Issue"),
		styles.Label.Render("Summary:"),
		title,
		desc,
		styles.Subtle.Render("enter to create issue • esc to cancel"),
	)
}
