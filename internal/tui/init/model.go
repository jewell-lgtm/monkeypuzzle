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
	StepPRMethod
	StepConfirm
	StepDone
)

// prMethodOptions lists the PR/MR providers the wizard offers, in display order.
// It is the single source of truth shared by the view (labels), the update
// cursor (bounds), and PRMethodValue (selection -> provider value).
var prMethodOptions = []struct{ Label, Value string }{
	{"GitHub via gh CLI", "github"},
	{"GitLab via glab CLI", "gitlab"},
}

// PRMethodValue maps a wizard selection index to its provider value, defaulting
// to github when the index is out of range.
func PRMethodValue(i int) string {
	if i < 0 || i >= len(prMethodOptions) {
		return "github"
	}
	return prMethodOptions[i].Value
}

type Model struct {
	Step        Step
	ProjectName textinput.Model
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
