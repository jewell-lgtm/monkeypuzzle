package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/chooser"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/dashboard"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagSwitchProject   string
	flagSwitchPiece     string
	flagSwitchBranch    string
	flagSwitchCreate    bool
	flagSwitchAll       bool
	flagSwitchAllSchema bool
)

var switchCmd = &cobra.Command{
	Use:   "switch [target]",
	Short: "Jump to a piece, branch, or project worktree — creating on demand",
	Long: `The single switching entry point: give it whatever you have in your head or
clipboard. TARGET resolves, in order, to an existing piece (by name or by the
branch checked out in it — attach), a local or remote branch (adopt as a piece,
then attach), or a brand-new name (create a piece whose branch is TARGET
verbatim — confirmed on a terminal, requires --create otherwise).

The project defaults to the repo you're standing in; --project overrides it
(and is required outside a repo).

Modes:
  Positional: mp switch feat/login-rework [--create]
  Flags:      mp switch --project NAME [--piece NAME | --branch NAME]
  Stdin JSON: echo '{"target":"fix-x"}' | mp switch
              echo '{"project":"app","branch":"my-spike"}' | mp switch
  --schema:   Output expected JSON format
  Interactive (default with a terminal): fuzzy picker scoped to the current
  project, or across every registered project with --all (same as ` + "`mp go`" + `)

--piece and --branch are explicit escape hatches that skip target resolution;
they are mutually exclusive with each other and with TARGET. Omit everything to
pick interactively; pass only --project to attach that project's main worktree.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSwitchAll,
}

func init() {
	switchCmd.Flags().StringVar(&flagSwitchProject, "project", "", "Project name or path (default: the repo you're in)")
	switchCmd.Flags().StringVar(&flagSwitchPiece, "piece", "", "Existing piece name to attach")
	switchCmd.Flags().StringVar(&flagSwitchBranch, "branch", "", "Git branch; adopts as a piece if needed, then attaches")
	switchCmd.Flags().BoolVar(&flagSwitchCreate, "create", false, "Create a new piece when the target matches nothing")
	switchCmd.Flags().BoolVar(&flagSwitchAll, "all", false, "Interactive picker across all registered projects")
	switchCmd.Flags().BoolVar(&flagSwitchAllSchema, "schema", false, "Print an example input document and exit")
	rootCmd.AddCommand(switchCmd)
}

type switchAllInput struct {
	Target  string `json:"target"`
	Project string `json:"project"`
	Piece   string `json:"piece"`
	Branch  string `json:"branch"`
	Create  bool   `json:"create"`
}

func runSwitchAll(cmd *cobra.Command, args []string) error {
	if flagSwitchAllSchema {
		return cli.PrintJSON(switchAllInput{})
	}
	ctx := cmd.Context()

	in, haveInput, err := getSwitchAllInput(args)
	if err != nil {
		return err
	}

	if !haveInput {
		if !cli.IsTerminal() {
			return fmt.Errorf("no input; pass a target, --piece/--branch, stdin JSON, or run with a terminal")
		}
		return runSwitchInteractive(ctx, flagSwitchAll)
	}

	if err := validateSwitchSelectors(in); err != nil {
		return err
	}

	proj, err := resolveSwitchProject(ctx, in.Project)
	if err != nil {
		return err
	}

	switch {
	case in.Target != "":
		return runSwitchTarget(ctx, proj, in.Target, in.Create)
	case in.Branch != "":
		return runSwitchFromBranch(ctx, proj, in.Branch)
	case in.Piece != "":
		return runSwitchToExistingPiece(ctx, proj, in.Piece)
	default:
		return attachSession(ctx, session.MainName(proj.Name), proj.Path)
	}
}

// validateSwitchSelectors enforces that at most one of target/piece/branch is
// set per invocation. Project is optional — it defaults to the current repo.
func validateSwitchSelectors(in switchAllInput) error {
	picked := 0
	if in.Target != "" {
		picked++
	}
	if in.Piece != "" {
		picked++
	}
	if in.Branch != "" {
		picked++
	}
	if picked > 1 {
		return fmt.Errorf("target, --piece, and --branch are mutually exclusive")
	}
	return nil
}

// resolveSwitchProject finds the project to operate on: an explicit name/path
// via the registry, else the repo the caller is standing in. The cwd path works
// for any init'd repo — registered or not — because detection is the on-disk
// monkeypuzzle config, matching bare `mp`.
func resolveSwitchProject(ctx context.Context, explicit string) (registry.Project, error) {
	if explicit != "" {
		reg, err := registry.Load()
		if err != nil {
			return registry.Project{}, err
		}
		proj, ok := reg.Find(explicit)
		if !ok {
			return registry.Project{}, fmt.Errorf("no registered project matching %q", explicit)
		}
		return proj, nil
	}
	root, state := classifyCwd(ctx)
	if state != cwdInProject {
		return registry.Project{}, fmt.Errorf("not inside a monkeypuzzle project; pass --project <name>, or run `mp init` here first")
	}
	name, ok := registry.ProjectName(root)
	if !ok || name == "" {
		name = filepath.Base(root)
	}
	return registry.Project{Name: name, Path: root}, nil
}

// runSwitchInteractive opens the fuzzy picker: scoped to the current project
// when inside one (the bare-`mp` view), across every registered project when
// outside one or when --all forces it (the `mp go` view).
func runSwitchInteractive(ctx context.Context, all bool) error {
	if !all {
		if root, state := classifyCwd(ctx); state == cwdInProject {
			return runDashboardTUI(ctx, dashboardLoadCmd(ctx, []projectcmd.Info{projectcmd.Describe(root)}))
		}
	}
	infos, err := projectcmd.List()
	if err != nil {
		return err
	}
	// Collect inside the Bubble Tea program (via the shared spinner-loading
	// picker) so the same loading affordance as `mp go` applies here.
	return runDashboardTUI(ctx, dashboardLoadCmd(ctx, infos))
}

// runSwitchTarget resolves a free-form target (piece name, branch, or new name)
// and dispatches: attach, adopt-then-attach, or create-then-attach. Creation is
// gated — confirmed interactively on a terminal, and refused without --create
// otherwise — so a typo'd target never silently mints a piece.
func runSwitchTarget(ctx context.Context, proj registry.Project, target string, create bool) error {
	_, handler := pieceHandlerForSwitch()
	res, err := handler.ResolveSwitchTarget(ctx, proj.Path, target)
	if err != nil {
		return err
	}
	switch res.Kind {
	case piececmd.TargetMain:
		return attachSession(ctx, session.MainName(proj.Name), proj.Path)
	case piececmd.TargetPiece:
		return attachSession(ctx, res.Piece.SessionName, res.Piece.WorktreePath)
	case piececmd.TargetAdoptLocal, piececmd.TargetAdoptRemote:
		info, err := handler.AdoptPiece(ctx, piececmd.AdoptPieceInput{
			RepoRoot: proj.Path,
			Branch:   res.Branch,
		})
		if err != nil {
			return err
		}
		return attachSession(ctx, info.SessionName, info.WorktreePath)
	case piececmd.TargetNew:
		if !create {
			if cli.IsTerminal() && !cli.HasStdinData() {
				ok, err := confirmCreateTarget(res.Branch, res.PieceName)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			} else {
				return fmt.Errorf("nothing named %q in %s (no piece, local branch, or remote branch); pass --create to start a new piece", target, proj.Name)
			}
		}
		input := piececmd.WithNewPieceDefaults(piececmd.NewPieceInput{
			Name:   res.PieceName,
			Branch: res.Branch,
		})
		info, err := handler.CreatePieceWithInput(ctx, input, piececmd.CreatePieceOptions{RepoRoot: proj.Path})
		if err != nil {
			return err
		}
		return attachSession(ctx, info.SessionName, info.WorktreePath)
	}
	return fmt.Errorf("unknown switch target kind: %v", res.Kind)
}

// confirmCreateTarget asks (on a real terminal) whether an unmatched target
// should become a new piece.
func confirmCreateTarget(branch, pieceName string) (bool, error) {
	summary := fmt.Sprintf("piece %q on new branch %q", pieceName, branch)
	if branch == pieceName {
		summary = fmt.Sprintf("piece %q on a new branch of the same name", pieceName)
	}
	choice, ok, err := chooser.Run(
		fmt.Sprintf("Nothing named %q here — create it?", branch),
		[]string{summary},
		[]chooser.Option{
			{Label: "Create", Desc: "create the piece and switch to it", Value: "create"},
			{Label: "Cancel", Desc: "do nothing", Value: ""},
		},
	)
	if err != nil {
		return false, err
	}
	return ok && choice == "create", nil
}

// dispatchPickedRow takes a dashboard.Row chosen by the user and runs the
// matching workflow: attach an existing piece/project, adopt an unadopted
// branch, or kick off the create-piece TUI.
func dispatchPickedRow(ctx context.Context, row dashboard.Row) error {
	switch row.Kind {
	case dashboard.RowProject, dashboard.RowPiece:
		return attachSession(ctx, row.SessionName, row.WorktreePath)
	case dashboard.RowBranch:
		return runSwitchFromBranch(ctx, projectFromRow(row), row.Branch)
	case dashboard.RowNewPiece:
		return runSwitchNewPiece(ctx, projectFromRow(row))
	}
	return fmt.Errorf("unknown dashboard row kind: %v", row.Kind)
}

func projectFromRow(row dashboard.Row) registry.Project {
	return registry.Project{Name: row.Project, Path: row.ProjectPath}
}

func runSwitchToExistingPiece(ctx context.Context, proj registry.Project, pieceName string) error {
	piecesDir, err := projectdir.PiecesDir(proj.Path)
	if err != nil {
		return err
	}
	worktreePath := filepath.Join(piecesDir, pieceName)
	if fi, err := os.Stat(worktreePath); err != nil || !fi.IsDir() {
		return fmt.Errorf("piece %q not found in project %q", pieceName, proj.Name)
	}
	return attachSession(ctx, session.Name(proj.Name, pieceName), worktreePath)
}

func runSwitchFromBranch(ctx context.Context, proj registry.Project, branch string) error {
	_, handler := pieceHandlerForSwitch()
	// Resolve first: a branch that is already a piece attaches instead of
	// erroring out of AdoptPiece, keeping --branch idempotent. A lookup
	// failure (e.g. can't read the pieces dir) must surface, not silently
	// fall through to adopt.
	piece, err := handler.PieceForBranch(ctx, proj.Path, branch)
	if err != nil {
		return err
	}
	if piece != nil {
		return attachSession(ctx, piece.SessionName, piece.WorktreePath)
	}
	info, err := handler.AdoptPiece(ctx, piececmd.AdoptPieceInput{
		RepoRoot: proj.Path,
		Branch:   branch,
	})
	if err != nil {
		return err
	}
	return attachSession(ctx, info.SessionName, info.WorktreePath)
}

// runSwitchNewPiece launches the existing piece-create TUI (promptinput)
// targeted at the given project, then attaches. We chdir into proj.Path for the
// duration because the underlying create flow resolves its repo root from cwd.
func runSwitchNewPiece(ctx context.Context, proj registry.Project) error {
	deps, handler := pieceHandlerForSwitch()

	restore, err := tempChdir(proj.Path)
	if err != nil {
		return err
	}
	defer restore()

	input, err := runPieceCreateTUI(deps, proj.Path)
	if err != nil {
		if err.Error() == "cancelled" {
			return nil
		}
		return err
	}

	input = piececmd.WithNewPieceDefaults(input)
	if err := piececmd.ValidateNewPieceInput(input); err != nil {
		return err
	}

	info, err := handler.CreatePieceWithInput(ctx, input, piececmd.CreatePieceOptions{})
	if err != nil {
		return err
	}
	return attachSession(ctx, info.SessionName, info.WorktreePath)
}

func pieceHandlerForSwitch() (core.Deps, *piececmd.Handler) {
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	return deps, newPieceHandler(deps)
}

// tempChdir changes the working directory to dir and returns a function that
// restores the previous cwd. Callers should defer the returned restore.
func tempChdir(dir string) (func(), error) {
	prev, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	if err := os.Chdir(dir); err != nil {
		return nil, fmt.Errorf("failed to chdir to %s: %w", dir, err)
	}
	return func() {
		if err := os.Chdir(prev); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to restore working directory to %s: %v\n", prev, err)
		}
	}, nil
}

func getSwitchAllInput(args []string) (switchAllInput, bool, error) {
	target := ""
	if len(args) > 0 {
		target = strings.TrimSpace(args[0])
	}
	switch {
	case target != "" || flagSwitchProject != "" || flagSwitchPiece != "" || flagSwitchBranch != "":
		return switchAllInput{
			Target:  target,
			Project: flagSwitchProject,
			Piece:   flagSwitchPiece,
			Branch:  flagSwitchBranch,
			Create:  flagSwitchCreate,
		}, true, nil
	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return switchAllInput{}, false, fmt.Errorf("failed to read stdin: %w", err)
		}
		var in switchAllInput
		if err := json.Unmarshal(data, &in); err != nil {
			return switchAllInput{}, false, fmt.Errorf("invalid JSON: %w", err)
		}
		if in.Target == "" && in.Project == "" && in.Piece == "" && in.Branch == "" {
			return switchAllInput{}, false, fmt.Errorf("provide a target, piece, branch, or project")
		}
		if flagSwitchCreate {
			in.Create = true
		}
		return in, true, nil
	default:
		return switchAllInput{}, false, nil
	}
}
