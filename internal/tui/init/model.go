package init

import (
	"os"
	"path/filepath"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type Step int

const (
	StepProjectName Step = iota
	StepIssueMethod
	StepPRMethod
	StepConfirm
	StepDone
)

type Model struct {
	Step        Step
	ProjectName textinput.Model
	IssueMethod int
	PRMethod    int
	Cancelled   bool
}

func New() Model {
	ti := textinput.New()
	ti.Placeholder = detectDirName()
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40

	return Model{
		Step:        StepProjectName,
		ProjectName: ti,
	}
}

func detectDirName() string {
	wd, err := os.Getwd()
	if err != nil {
		return "my-project"
	}
	return filepath.Base(wd)
}

func (m Model) Init() tea.Cmd {
	return textinput.Blink
}
