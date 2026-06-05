package mp

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

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
the same data as JSON. Bare 'mp' shows the same dashboard scoped to the current
project (repo-local); 'mp dash' always shows every registered project.`,
	RunE: runDash,
}

var flagDashJSON bool

func init() {
	dashCmd.Flags().BoolVar(&flagDashJSON, "json", false, "Output JSON instead of the interactive dashboard")
	rootCmd.AddCommand(dashCmd)

	// Bare `mp` opens a dashboard scoped to the current project (repo-local),
	// falling back to the cross-project view when run outside a registered repo.
	// `mp dash` always shows every project.
	rootCmd.Flags().BoolVar(&flagDashJSON, "json", false, "Output JSON instead of the interactive dashboard")
	rootCmd.RunE = runRoot
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
}

// dashBranch is the JSON shape for a git branch that is not yet adopted as a
// piece. The picker uses these rows to adopt a branch on the fly. Remote marks
// a remote-only ref (e.g. "origin/foo") with no local branch yet.
type dashBranch struct {
	Name   string `json:"name"`
	Remote bool   `json:"remote,omitempty"`
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

// currentProjectInfo returns the enriched Info for the registered project that
// contains the current working directory, resolving the main repo root so it
// also works from inside a piece worktree. The bool is false when cwd is not
// inside a registered project (e.g. an unrelated repo, or no repo at all).
func currentProjectInfo(ctx context.Context) (projectcmd.Info, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return projectcmd.Info{}, false
	}
	git := adapters.NewGit(adapters.NewOSExec())
	root, err := git.GetMainRepoRoot(ctx, wd)
	if err != nil {
		if root, err = git.RepoRoot(ctx, wd); err != nil {
			return projectcmd.Info{}, false
		}
	}
	// The registry stores symlink-resolved paths; match that so lookups succeed
	// on systems where the repo lives under a symlinked prefix.
	if resolved, rerr := filepath.EvalSymlinks(root); rerr == nil {
		root = resolved
	}
	info, ok, err := projectcmd.Get(root)
	if err != nil || !ok {
		return projectcmd.Info{}, false
	}
	return info, true
}

// dashboardInfos returns the projects to show. Bare `mp` scopes to the current
// project (repo-local); when cwd isn't in a registered project it falls back to
// every registered project so `mp` still works as a launcher from anywhere.
func dashboardInfos(ctx context.Context, scopeToCurrent bool) ([]projectcmd.Info, error) {
	if scopeToCurrent {
		if info, ok := currentProjectInfo(ctx); ok {
			return []projectcmd.Info{info}, nil
		}
	}
	return projectcmd.List()
}

func collectDashboard(ctx context.Context, infos []projectcmd.Info) ([]dashProject, error) {
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

// collectProjectIssues returns up to maxIssuesPerProject issues for a project,
// excluding any issue that already has a piece. The exclusion set is built from
// each existing piece's recorded issue ref (in piece-metadata.json): there is no
// longer an issue status to filter on, so "has a piece?" is the dedup rule that
// keeps the list useful.
func collectProjectIssues(deps core.Deps, projectPath string, pieces []dashPiece) []dashIssue {
	handler := issue.NewHandler(deps, projectPath)
	items, err := handler.ListIssues()
	if err != nil {
		return nil
	}

	worked := workedIssueIDs(deps, pieces)

	out := make([]dashIssue, 0, len(items))
	for _, it := range items {
		if worked[it.ID] {
			continue
		}
		out = append(out, dashIssue{
			Path:   it.Path,
			Title:  it.Title,
			Number: it.Number,
		})
		if len(out) >= maxIssuesPerProject {
			break
		}
	}
	return out
}

// workedIssueIDs returns the set of issue IDs that already have a piece, derived
// from each piece's recorded issue ref in its metadata.
func workedIssueIDs(deps core.Deps, pieces []dashPiece) map[string]bool {
	worked := make(map[string]bool)
	for _, pc := range pieces {
		meta, err := piececmd.ReadPieceMetadata(pc.WorktreePath, deps.FS)
		if err != nil || meta == nil || meta.Issue.IsEmpty() {
			continue
		}
		worked[meta.Issue.ID] = true
	}
	return worked
}

// collectProjectBranches returns git branches that are candidates for adoption:
// not main/master and not currently checked out in any worktree (which covers
// both the main repo and existing piece worktrees). Local branches come first,
// then remote-only refs (e.g. "origin/foo") that have no local branch yet —
// adopting one fetches the remote and creates a tracking worktree.
func collectProjectBranches(ctx context.Context, git *adapters.Git, projectPath string) []dashBranch {
	locals, err := git.ListLocalBranches(ctx, projectPath)
	if err != nil {
		return nil
	}
	checkedOut, err := git.CheckedOutBranches(ctx, projectPath)
	if err != nil {
		// On error, fall back to skipping main/master only — better to show too
		// much than to silently hide everything.
		checkedOut = map[string]bool{}
	}

	localSet := make(map[string]bool, len(locals))
	for _, b := range locals {
		localSet[b] = true
	}

	out := make([]dashBranch, 0, maxBranchesPerProject)
	for _, b := range locals {
		if b == "main" || b == "master" || checkedOut[b] {
			continue
		}
		out = append(out, dashBranch{Name: b})
		if len(out) >= maxBranchesPerProject {
			return out
		}
	}

	// Remote-only branches: the bug we're fixing is that you couldn't create a
	// piece from a branch that exists on origin but not locally.
	remotes, err := git.ListRemoteBranches(ctx, projectPath)
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	for _, r := range remotes {
		short := remoteBranchShortName(r)
		if short == "" || short == "main" || short == "master" {
			continue
		}
		// Skip remotes that already have a local branch (the local row covers
		// them) or whose branch is checked out, and dedupe across remotes.
		if localSet[short] || checkedOut[short] || seen[short] {
			continue
		}
		seen[short] = true
		out = append(out, dashBranch{Name: r, Remote: true})
		if len(out) >= maxBranchesPerProject {
			break
		}
	}
	return out
}

// remoteBranchShortName strips the leading "<remote>/" from a remote ref like
// "origin/foo" → "foo" (preserving any further slashes, e.g.
// "origin/feature/x" → "feature/x"). Returns "" if there's nothing after the
// remote name.
func remoteBranchShortName(ref string) string {
	if i := strings.IndexByte(ref, '/'); i > 0 && i < len(ref)-1 {
		return ref[i+1:]
	}
	return ""
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
		// Only existing project repos can host a new piece. Missing projects
		// would just fail downstream.
		if p.Exists && p.IsProject {
			rows = append(rows, dashboard.Row{
				Kind:        dashboard.RowNewPiece,
				Project:     p.Name,
				ProjectPath: p.Path,
			})
		}
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
				Remote:      br.Remote,
			})
		}
	}
	return rows
}

// runRoot handles bare `mp`: a dashboard scoped to the current project. When
// cwd isn't inside a registered project it falls back to the cross-project view
// so `mp` still works as a launcher from anywhere.
func runRoot(cmd *cobra.Command, args []string) error {
	return runDashScoped(cmd.Context(), true)
}

// runDash handles `mp dash`: the explicit cross-project view over every
// registered project, regardless of where it's run from.
func runDash(cmd *cobra.Command, args []string) error {
	return runDashScoped(cmd.Context(), false)
}

func runDashScoped(ctx context.Context, scopeToCurrent bool) error {
	infos, err := dashboardInfos(ctx, scopeToCurrent)
	if err != nil {
		return err
	}
	projects, err := collectDashboard(ctx, infos)
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
	return dispatchPickedRow(ctx, row)
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
