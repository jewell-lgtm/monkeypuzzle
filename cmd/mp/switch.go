package mp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/dashboard"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagSwitchProject   string
	flagSwitchPiece     string
	flagSwitchAllSchema bool
)

var switchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Jump to any project or piece worktree across all registered projects",
	Long: `Attach the tmux session for a worktree in any registered project.

Modes:
  Flags:      mp switch --project NAME [--piece NAME]
  Stdin JSON: echo '{"project":"app","piece":"fix-x"}' | mp switch
  --schema:   Output expected JSON format
  Interactive (default with a terminal): pick from the cross-project dashboard

Omit --piece to attach a project's main worktree.`,
	RunE: runSwitchAll,
}

func init() {
	switchCmd.Flags().StringVar(&flagSwitchProject, "project", "", "Project name or path")
	switchCmd.Flags().StringVar(&flagSwitchPiece, "piece", "", "Piece name (omit for the main worktree)")
	switchCmd.Flags().BoolVar(&flagSwitchAllSchema, "schema", false, "Output JSON schema and exit")
	rootCmd.AddCommand(switchCmd)
}

type switchAllInput struct {
	Project string `json:"project"`
	Piece   string `json:"piece"`
}

func runSwitchAll(cmd *cobra.Command, args []string) error {
	if flagSwitchAllSchema {
		return cli.PrintJSON(switchAllInput{})
	}
	ctx := cmd.Context()

	in, haveInput, err := getSwitchAllInput()
	if err != nil {
		return err
	}

	if !haveInput {
		// Interactive: reuse the dashboard picker.
		if !cli.IsTerminal() {
			return fmt.Errorf("no input; use --project/--piece, stdin JSON, or run with a terminal")
		}
		projects, err := collectDashboard(ctx)
		if err != nil {
			return err
		}
		p := tea.NewProgram(dashboard.New(dashboardRows(projects)))
		m, err := p.Run()
		if err != nil {
			return err
		}
		row, ok := m.(dashboard.Model).SelectedRow()
		if !ok {
			return nil
		}
		return attachSession(ctx, row.SessionName, row.WorktreePath)
	}

	reg, err := registry.Load()
	if err != nil {
		return err
	}
	proj, ok := reg.Find(in.Project)
	if !ok {
		return fmt.Errorf("no registered project matching %q", in.Project)
	}

	if in.Piece == "" {
		return attachSession(ctx, session.MainName(proj.Name), proj.Path)
	}

	piecesDir, err := projectdir.PiecesDir(proj.Path)
	if err != nil {
		return err
	}
	worktreePath := filepath.Join(piecesDir, in.Piece)
	if fi, err := os.Stat(worktreePath); err != nil || !fi.IsDir() {
		return fmt.Errorf("piece %q not found in project %q", in.Piece, proj.Name)
	}
	return attachSession(ctx, session.Name(proj.Name, in.Piece), worktreePath)
}

func getSwitchAllInput() (switchAllInput, bool, error) {
	switch {
	case flagSwitchProject != "":
		return switchAllInput{Project: flagSwitchProject, Piece: flagSwitchPiece}, true, nil
	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return switchAllInput{}, false, fmt.Errorf("failed to read stdin: %w", err)
		}
		var in switchAllInput
		if err := json.Unmarshal(data, &in); err != nil {
			return switchAllInput{}, false, fmt.Errorf("invalid JSON: %w", err)
		}
		if in.Project == "" {
			return switchAllInput{}, false, fmt.Errorf("project is required")
		}
		return in, true, nil
	default:
		return switchAllInput{}, false, nil
	}
}
