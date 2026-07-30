package main

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
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/dashboard"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// maxBranchesPerProject caps the cross-project dashboard's per-project branch
// list to keep the picker bounded before fuzzy filtering.
const maxBranchesPerProject = 10

var goCmd = &cobra.Command{
	Use:   "go",
	Short: "Switch between registered repos (and drill into their worktrees)",
	Long: `Pick a registered monkeypuzzle project and jump to its worktree session — from
anywhere, regardless of the current repo.

With a terminal, opens an interactive fuzzy picker. Each repo starts collapsed
(one row per repo); press → to expand it and reveal its pieces and branches, or
just press Enter on a collapsed repo to jump to its main worktree. Typing filters
across everything (expanded or not); ↑/↓ and PgUp/PgDn scroll.

With --json (or no TTY) it prints the full per-project detail so automation can
build its own pickers.`,
	RunE: runGo,
}

var flagDashJSON bool

func init() {
	goCmd.Flags().BoolVar(&flagDashJSON, "json", false, "Output JSON instead of the interactive picker")
	rootCmd.AddCommand(goCmd)

	// Bare `mp` opens a fuzzy picker scoped to the current project (repo-local).
	// Outside a monkeypuzzle project it prints context-aware guidance rather than
	// falling back to a cross-project view — use `mp go` for that.
	rootCmd.Flags().BoolVar(&flagDashJSON, "json", false, "Output JSON instead of the interactive dashboard")
	rootCmd.RunE = runRoot
}

// dashPiece is the JSON shape for a piece worktree in the dashboard.
type dashPiece struct {
	Name         string `json:"name"`
	WorktreePath string `json:"worktree_path"`
	SessionName  string `json:"session_name"`
	HasSession   bool   `json:"has_session"`
	// AgentStatus / AgentCounts mirror PieceListItem: the aggregate status of
	// agents reported in the piece, for badges and blocked-first sorting.
	AgentStatus string         `json:"agent_status,omitempty"`
	AgentCounts map[string]int `json:"agent_counts,omitempty"`
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
	Branches    []dashBranch `json:"branches,omitempty"`
	Error       string       `json:"error,omitempty"`
}

// cwdState classifies the current working directory for bare `mp`.
type cwdState int

const (
	cwdInProject      cwdState = iota // inside a monkeypuzzle project (.monkeypuzzle/monkeypuzzle.json present)
	cwdRepoNotProject                 // inside a git repo that hasn't been `mp init`-ed
	cwdNotRepo                        // not inside a git repo at all
)

// classifyCwd determines whether the current working directory is inside a
// monkeypuzzle project, inside a plain (non-init'd) git repo, or outside any
// repo. It resolves the main repo root so it also works from inside a piece
// worktree, and returns that root for the in-project and repo-not-project cases.
func classifyCwd(ctx context.Context) (root string, state cwdState) {
	wd, err := os.Getwd()
	if err != nil {
		return "", cwdNotRepo
	}
	git := adapters.NewGit(adapters.NewOSExec())
	root, err = git.GetMainRepoRoot(ctx, wd)
	if err != nil {
		if root, err = git.RepoRoot(ctx, wd); err != nil {
			return "", cwdNotRepo
		}
	}
	// The registry stores symlink-resolved paths; match that so lookups succeed
	// on systems where the repo lives under a symlinked prefix.
	if resolved, rerr := filepath.EvalSymlinks(root); rerr == nil {
		root = resolved
	}
	if registry.IsProject(root) {
		return root, cwdInProject
	}
	return root, cwdRepoNotProject
}

func collectDashboard(ctx context.Context, infos []projectcmd.Info) ([]dashProject, error) {
	// No-op loading: the dashboard renders interactively (and prints JSON in
	// non-TTY mode), so a stray "⏳ Fetching projects..." line printed to stderr
	// during collection would just linger above the picker.
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupNoopLoading(),
	)
	handler := newPieceHandler(deps)
	git := adapters.NewGit(adapters.NewOSExec())

	out := make([]dashProject, 0, len(infos))
	for _, info := range infos {
		dp := dashProject{Info: info, MainSession: session.MainName(info.Name)}
		// Remote projects are listed but not enumerated: their pieces live on
		// the host (`mp --project <name> list` proxies into them).
		if info.Host == "" && info.Exists && info.IsProject {
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
						AgentStatus:  pc.AgentStatus,
						AgentCounts:  pc.AgentCounts,
					})
				}
			}
			piecePaths := make([]string, 0, len(dp.Pieces))
			for _, pc := range dp.Pieces {
				piecePaths = append(piecePaths, pc.WorktreePath)
			}
			dp.Branches = collectProjectBranches(ctx, git, info.Path, piecePaths)
		}
		out = append(out, dp)
	}
	return out, nil
}

// collectProjectBranches returns git branches that are candidates for adoption:
// not main/master, not checked out in the main repo or an existing piece
// worktree, and not held by a locked worktree. A branch checked out in some
// *other* worktree (e.g. one an agent created) still qualifies — adopting it
// relocates that worktree into the pieces dir. Local branches come first,
// then remote-only refs (e.g. "origin/foo") that have no local branch yet —
// adopting one fetches the remote and creates a tracking worktree.
func collectProjectBranches(ctx context.Context, git *adapters.Git, projectPath string, pieceWorktrees []string) []dashBranch {
	locals, err := git.ListLocalBranches(ctx, projectPath)
	if err != nil {
		return nil
	}
	checkedOut := unadoptableBranches(ctx, git, projectPath, pieceWorktrees)

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

// unadoptableBranches returns the set of branch names `mp adopt` cannot take
// over: those checked out in the main repo checkout, in an existing piece
// worktree, or in a locked worktree. On probe failure it returns an empty set —
// better to show too much than to silently hide everything.
func unadoptableBranches(ctx context.Context, git *adapters.Git, projectPath string, pieceWorktrees []string) map[string]bool {
	worktrees, err := git.Worktrees(ctx, projectPath)
	if err != nil {
		return map[string]bool{}
	}
	pieceSet := make(map[string]bool, len(pieceWorktrees))
	for _, p := range pieceWorktrees {
		pieceSet[filepath.Clean(p)] = true
	}
	mainPath := filepath.Clean(projectPath)
	out := map[string]bool{}
	for _, wt := range worktrees {
		if wt.Branch == "" || wt.Prunable {
			// Detached worktrees hold no branch; a prunable record's branch is
			// still adoptable (adopt prunes it first).
			continue
		}
		path := filepath.Clean(wt.Path)
		if path == mainPath || pieceSet[path] || wt.Locked {
			out[wt.Branch] = true
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

// runRoot handles bare `mp`: a fuzzy picker scoped to the current monkeypuzzle
// project. Outside a project it prints context-aware guidance (run `mp init`, or
// `mp go` to jump to a known project) instead of falling back to anything.
func runRoot(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	root, state := classifyCwd(ctx)
	if state != cwdInProject {
		return guideOutsideProject(state, root)
	}
	return renderDashboard(ctx, []projectcmd.Info{projectcmd.Describe(root)})
}

// runGo handles `mp go`: a repo switcher across every registered project. The
// interactive picker shows each repo collapsed (one row per repo) and lets you
// expand a repo to reveal its pieces/branches; Enter on a collapsed repo jumps
// straight to its main worktree. With --json (or no TTY) it prints the full
// per-project detail.
func runGo(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	infos, err := projectcmd.List()
	if err != nil {
		return err
	}
	return renderDashboard(ctx, infos)
}

// guideOutsideProject prints context-aware guidance when bare `mp` runs outside
// a monkeypuzzle project. It points an un-init'd git repo at `mp init`, and a
// non-repo directory at cloning/cd-ing into one; both mention `mp go` when there
// are registered projects to jump to. In JSON / non-TTY mode the same guidance
// is emitted as a structured object so automation gets a loud, parseable signal.
func guideOutsideProject(state cwdState, root string) error {
	hasProjects := false
	if infos, err := projectcmd.List(); err == nil {
		hasProjects = len(infos) > 0
	}

	var reason, action string
	switch state {
	case cwdRepoNotProject:
		reason = fmt.Sprintf("This repo (%s) isn't set up for monkeypuzzle yet.", root)
		action = "Run `mp init` here to get started."
	default: // cwdNotRepo
		reason = "You're not inside a git repository."
		action = "cd into a repo and run `mp init`, or clone one first."
	}
	goHint := ""
	if hasProjects {
		goHint = "Run `mp go` to jump to one of your registered projects."
	}

	// Human-readable guidance always goes to stderr (per the output convention),
	// regardless of TTY — there is nothing to render and nothing to attach to.
	fmt.Fprintln(os.Stderr, reason)
	fmt.Fprintln(os.Stderr, action)
	if goHint != "" {
		fmt.Fprintln(os.Stderr, goHint)
	}

	// In explicit JSON mode, also emit a structured, parseable signal on stdout
	// so automation gets a loud "no project here" answer instead of an empty list
	// it might mistake for "no projects exist".
	if flagDashJSON {
		return cli.PrintJSON(map[string]any{
			"projects":     []any{},
			"in_project":   false,
			"reason":       reason,
			"suggestion":   action,
			"has_projects": hasProjects,
		})
	}
	return nil
}

func renderDashboard(ctx context.Context, infos []projectcmd.Info) error {
	// Non-interactive: collect synchronously and print JSON.
	if flagDashJSON || !cli.IsStdoutTerminal() || !cli.IsTerminal() {
		projects, err := collectDashboard(ctx, infos)
		if err != nil {
			return err
		}
		return cli.PrintJSON(map[string]any{"projects": projects})
	}

	// Interactive: collect inside the Bubble Tea program so the spinner animates
	// while loading and clears once the rows are ready.
	return runDashboardTUI(ctx, dashboardLoadCmd(ctx, infos))
}

// dashboardLoadCmd returns a Bubble Tea command that collects the dashboard for
// the given projects and delivers the rows (or an error) as a RowsMsg.
func dashboardLoadCmd(ctx context.Context, infos []projectcmd.Info) tea.Cmd {
	return func() tea.Msg {
		projects, err := collectDashboard(ctx, infos)
		if err != nil {
			return dashboard.RowsMsg{Err: err}
		}
		return dashboard.RowsMsg{Rows: dashboardRows(projects)}
	}
}

// runDashboardTUI runs the interactive picker with a spinner that loads via
// loadCmd, then dispatches the chosen row. Shared by `mp go` and `mp switch`.
func runDashboardTUI(ctx context.Context, loadCmd tea.Cmd) error {
	initial := dashboard.NewLoading(loadCmd)
	if cfg, err := config.LoadUserConfig(); err == nil && cfg.Multiplexer != "" && cfg.Multiplexer != "none" {
		initial.SessionLabel = cfg.Multiplexer
	}
	p := tea.NewProgram(initial)
	m, err := p.Run()
	if err != nil {
		return err
	}
	model := m.(dashboard.Model)
	if model.Err != nil {
		return model.Err
	}
	row, ok := model.SelectedRow()
	if !ok {
		return nil // cancelled
	}
	return dispatchPickedRow(ctx, row)
}

// attachSession switches to (creating if needed) the given multiplexer session.
// If no multiplexer is configured, or mp is not in an interactive session
// context (agents/scripts, or a terminal outside the multiplexer — see
// chooseMultiplexer), it prints the worktree path so callers can `cd` into it
// instead. This keeps `mp switch` / `mp go` from creating or hijacking a
// session when driven through the stateless API.
func attachSession(ctx context.Context, sessionName, workDir string) error {
	mux := chooseMultiplexer(adapters.NewOSExec())
	if adapters.IsNoopMultiplexer(mux) || !mux.IsInstalled(ctx) {
		// No session management here: print the path for `cd $(mp switch ...)`.
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
