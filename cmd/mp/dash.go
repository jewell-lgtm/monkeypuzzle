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
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/dashboard"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Caps applied to the cross-project dashboard's per-project issue and branch
// lists to keep the picker bounded before fuzzy filtering.
const (
	maxIssuesPerProject   = 10
	maxBranchesPerProject = 10
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

// dashIssue is the JSON shape for an open issue surfaced by the dashboard.
// Only issues without an associated piece appear here — the picker uses these
// rows to create a piece on the fly.
type dashIssue struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Number string `json:"number,omitempty"`
	Status string `json:"status"`
}

// dashBranch is the JSON shape for a local git branch that is not yet adopted
// as a piece. The picker uses these rows to adopt a branch on the fly.
type dashBranch struct {
	Name string `json:"name"`
}

// dashProject is the JSON shape for one project in the dashboard.
type dashProject struct {
	projectcmd.Info
	MainSession string       `json:"main_session"`
	Pieces      []dashPiece  `json:"pieces"`
	Issues      []dashIssue  `json:"issues,omitempty"`
	Branches    []dashBranch `json:"branches,omitempty"`
	Error       string       `json:"error,omitempty"`
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
	git := adapters.NewGit(adapters.NewOSExec())

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
			dp.Issues = collectProjectIssues(deps, info.Path, dp.Pieces)
			dp.Branches = collectProjectBranches(ctx, git, info.Path)
		}
		out = append(out, dp)
	}
	return out, nil
}

// collectProjectIssues returns up to maxIssuesPerProject open todo issues for a
// project. Issues claimed by an existing piece are already filtered out by the
// "todo"-only status filter — creating a piece from an issue transitions its
// status to in-progress.
func collectProjectIssues(deps core.Deps, projectPath string, pieces []dashPiece) []dashIssue {
	handler := issue.NewHandler(deps, projectPath)
	items, err := handler.ListIssues([]string{piececmd.StatusTodo})
	if err != nil {
		return nil
	}

	out := make([]dashIssue, 0, len(items))
	for _, it := range items {
		out = append(out, dashIssue{
			Path:   it.Path,
			Title:  it.Title,
			Number: it.Number,
			Status: it.Status,
		})
		if len(out) >= maxIssuesPerProject {
			break
		}
	}
	_ = pieces
	return out
}

// collectProjectBranches returns local git branches that are candidates for
// adoption: not main/master, not currently checked out in any worktree (which
// covers both the main repo and existing piece worktrees).
func collectProjectBranches(ctx context.Context, git *adapters.Git, projectPath string) []dashBranch {
	branches, err := git.ListLocalBranches(ctx, projectPath)
	if err != nil {
		return nil
	}
	checkedOut, err := git.CheckedOutBranches(ctx, projectPath)
	if err != nil {
		// On error, fall back to skipping main/master only — better to show too
		// much than to silently hide everything.
		checkedOut = map[string]bool{}
	}

	out := make([]dashBranch, 0, len(branches))
	for _, b := range branches {
		if b == "main" || b == "master" {
			continue
		}
		if checkedOut[b] {
			continue
		}
		out = append(out, dashBranch{Name: b})
		if len(out) >= maxBranchesPerProject {
			break
		}
	}
	return out
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
		for _, is := range p.Issues {
			rows = append(rows, dashboard.Row{
				Kind:        dashboard.RowIssue,
				Project:     p.Name,
				ProjectPath: p.Path,
				IssuePath:   is.Path,
				IssueTitle:  is.Title,
				IssueNumber: is.Number,
			})
		}
		for _, br := range p.Branches {
			rows = append(rows, dashboard.Row{
				Kind:        dashboard.RowBranch,
				Project:     p.Name,
				ProjectPath: p.Path,
				Branch:      br.Name,
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
