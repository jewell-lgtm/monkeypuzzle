package mp

import (
	"context"
	"fmt"
	"net/http"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/dashboard"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Cross-project dashboard (projects, pieces, issues)",
	Long: `Show all registered monkeypuzzle projects and their piece worktrees, and jump
into any worktree's tmux session.

With a terminal, opens an interactive dashboard. Otherwise (or with --json) prints
the same data as JSON. Running bare 'mp' is equivalent to 'mp dash'.`,
	RunE: runDash,
}

var flagDashJSON bool

func init() {
	dashCmd.Flags().BoolVar(&flagDashJSON, "json", false, "Output JSON instead of the interactive dashboard")
	rootCmd.AddCommand(dashCmd)

	// Bare `mp` opens the dashboard.
	rootCmd.RunE = runDash
}

// dashPiece is the JSON shape for a piece worktree in the dashboard.
type dashPiece struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktree_path"`
	SessionName  string `json:"session_name"`
	HasSession   bool   `json:"has_session"`
}

// dashProject is the JSON shape for one project in the dashboard.
type dashProject struct {
	projectcmd.Info
	MainSession string      `json:"main_session"`
	Pieces      []dashPiece `json:"pieces"`
	Error       string      `json:"error,omitempty"`
}

func collectDashboard(ctx context.Context) ([]dashProject, error) {
	infos, err := projectcmd.List()
	if err != nil {
		return nil, err
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	out := make([]dashProject, 0, len(infos))
	for _, info := range infos {
		dp := dashProject{Info: info, MainSession: session.MainName(info.Name)}
		if info.Exists && info.IsProject {
			pieces, err := handler.ListPieces(ctx, info.Path)
			if err != nil {
				dp.Error = err.Error()
			} else {
				for _, pc := range pieces {
					dp.Pieces = append(dp.Pieces, dashPiece{
						Name:         pc.Name,
						WorktreePath: pc.WorktreePath,
						SessionName:  pc.SessionName,
						HasSession:   pc.HasSession,
					})
				}
			}
		}
		out = append(out, dp)
	}
	return out, nil
}

func dashboardRows(projects []dashProject) []dashboard.Row {
	var rows []dashboard.Row
	for _, p := range projects {
		rows = append(rows, dashboard.Row{
			Kind:         dashboard.RowProject,
			Project:      p.Name,
			ProjectPath:  p.Path,
			WorktreePath: p.Path,
			SessionName:  p.MainSession,
			Branch:       p.Branch,
			PieceCount:   p.PieceCount,
			OpenIssues:   p.OpenIssues,
			Missing:      !p.Exists,
		})
		for _, pc := range p.Pieces {
			rows = append(rows, dashboard.Row{
				Kind:         dashboard.RowPiece,
				Project:      p.Name,
				ProjectPath:  p.Path,
				Piece:        pc.Name,
				WorktreePath: pc.WorktreePath,
				SessionName:  pc.SessionName,
				HasSession:   pc.HasSession,
			})
		}
	}
	return rows
}

func runDash(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	projects, err := collectDashboard(ctx)
	if err != nil {
		return err
	}

	if flagDashJSON || !cli.IsStdoutTerminal() || !cli.IsTerminal() {
		return cli.PrintJSON(map[string]any{"projects": projects})
	}

	rows := dashboardRows(projects)
	p := tea.NewProgram(dashboard.New(rows))
	m, err := p.Run()
	if err != nil {
		return err
	}
	model := m.(dashboard.Model)
	row, ok := model.SelectedRow()
	if !ok {
		return nil // cancelled
	}
	return attachSession(ctx, row.SessionName, row.WorktreePath)
}

// attachSession switches to (creating if needed) the given tmux session, using
// the user's configured multiplexer. If no multiplexer is configured it prints
// the worktree path so callers can `cd` into it.
func attachSession(ctx context.Context, sessionName, workDir string) error {
	userCfg, err := config.LoadUserConfig()
	if err != nil {
		return err
	}
	if userCfg.Multiplexer == "" || userCfg.Multiplexer == "none" {
		// No multiplexer configured: print the path for `cd $(mp switch ...)`.
		fmt.Println(workDir)
		return nil
	}
	mux, err := adapters.NewMultiplexer(userCfg.Multiplexer, adapters.NewOSExec())
	if err != nil {
		return err
	}
	if !mux.IsInstalled(ctx) {
		fmt.Println(workDir)
		return nil
	}
	if err := mux.SwitchTo(ctx, sessionName, workDir); err != nil {
		// Fall back to printing the path.
		fmt.Println(workDir)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Attached %s\n", sessionName)
	return nil
}
