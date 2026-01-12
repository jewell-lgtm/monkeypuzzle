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
	StepLinearAPIKey
	StepLinearTeam
	StepPRMethod
	StepConfirm
	StepDone
)

// IssueProviders maps selection index to provider name
var IssueProviders = []string{"markdown", "linear"}

type Model struct {
	Step         Step
	ProjectName  textinput.Model
	IssueMethod  int
	LinearAPIKey textinput.Model
	LinearTeam   textinput.Model
	PRMethod     int
	Cancelled    bool
}

func New() Model {
	ti := textinput.New()
	ti.Placeholder = detectDirName()
	ti.Focus()
	ti.CharLimit = 100
	ti.Width = 40

	apiKey := textinput.New()
	apiKey.Placeholder = "(leave blank to use LINEAR_API_KEY env var)"
	apiKey.CharLimit = 200
	apiKey.Width = 60
	apiKey.EchoMode = textinput.EchoPassword

	team := textinput.New()
	team.Placeholder = "e.g. ENG"
	team.CharLimit = 50
	team.Width = 40

	return Model{
		Step:         StepProjectName,
		ProjectName:  ti,
		LinearAPIKey: apiKey,
		LinearTeam:   team,
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
