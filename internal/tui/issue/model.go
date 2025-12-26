package issue

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type Step int

const (
	StepTitle Step = iota
	StepDescription
	StepConfirm
	StepDone
)

type Model struct {
	Step        Step
	Title       textinput.Model
	Description textinput.Model
	Cancelled   bool
}

func New() Model {
	title := textinput.New()
	title.Placeholder = "Issue title"
	title.Focus()
	title.CharLimit = 100
	title.Width = 50

	desc := textinput.New()
	desc.Placeholder = "Optional description"
	desc.CharLimit = 500
	desc.Width = 50

	return Model{
		Step:        StepTitle,
		Title:       title,
		Description: desc,
	}
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}
