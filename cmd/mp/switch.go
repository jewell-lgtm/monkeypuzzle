package mp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/session"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/dashboard"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagSwitchProject   string
	flagSwitchPiece     string
	flagSwitchIssue     string
	flagSwitchBranch    string
	flagSwitchAllSchema bool
)

var switchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Jump to a project worktree, or create one from an open issue or local branch",
	Long: `Attach the tmux session for a worktree in any registered project. The picker
unifies three entry points: existing pieces (attach), open todo issues (create
a piece from the issue, then attach), and local git branches not yet adopted
(adopt as a piece, then attach).

Modes:
  Flags:      mp switch --project NAME [--piece NAME | --issue PATH | --branch NAME]
  Stdin JSON: echo '{"project":"app","piece":"fix-x"}' | mp switch
              echo '{"project":"app","issue":"issues/foo.md"}' | mp switch
              echo '{"project":"app","branch":"my-spike"}' | mp switch
  --schema:   Output expected JSON format
  Interactive (default with a terminal): pick from the fuzzy-filtered dashboard

--piece, --issue and --branch are mutually exclusive. Omit all three to attach
the project's main worktree.`,
	RunE: runSwitchAll,
}

func init() {
	switchCmd.Flags().StringVar(&flagSwitchProject, "project", "", "Project name or path")
	switchCmd.Flags().StringVar(&flagSwitchPiece, "piece", "", "Existing piece name to attach")
	switchCmd.Flags().StringVar(&flagSwitchIssue, "issue", "", "Issue id or path; creates a piece, then attaches")
	switchCmd.Flags().StringVar(&flagSwitchBranch, "branch", "", "Local git branch; adopts as a piece, then attaches")
	switchCmd.Flags().BoolVar(&flagSwitchAllSchema, "schema", false, "Output JSON schema and exit")
	rootCmd.AddCommand(switchCmd)
}

type switchAllInput struct {
	Project string `json:"project"`
	Piece   string `json:"piece"`
	Issue   string `json:"issue"`
	Branch  string `json:"branch"`
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
		if !cli.IsTerminal() {
			return fmt.Errorf("no input; use --project plus --piece/--issue/--branch, stdin JSON, or run with a terminal")
		}
		return runSwitchInteractive(ctx)
	}

	if err := validateSwitchSelectors(in); err != nil {
		return err
	}

	reg, err := registry.Load()
	if err != nil {
		return err
	}
	proj, ok := reg.Find(in.Project)
	if !ok {
		return fmt.Errorf("no registered project matching %q", in.Project)
	}

	switch {
	case in.Issue != "":
		return runSwitchFromIssue(ctx, proj, in.Issue)
	case in.Branch != "":
		return runSwitchFromBranch(ctx, proj, in.Branch)
	case in.Piece != "":
		return runSwitchToExistingPiece(ctx, proj, in.Piece)
	default:
		return attachSession(ctx, session.MainName(proj.Name), proj.Path)
	}
}

// validateSwitchSelectors enforces that at most one of piece/issue/branch is
// set per invocation. Project must always be present at this point.
func validateSwitchSelectors(in switchAllInput) error {
	if in.Project == "" {
		return fmt.Errorf("project is required")
	}
	picked := 0
	if in.Piece != "" {
		picked++
	}
	if in.Issue != "" {
		picked++
	}
	if in.Branch != "" {
		picked++
	}
	if picked > 1 {
		return fmt.Errorf("--piece, --issue and --branch are mutually exclusive")
	}
	return nil
}

func runSwitchInteractive(ctx context.Context) error {
	infos, err := projectcmd.List()
	if err != nil {
		return err
	}
	// Collect inside the Bubble Tea program (via the shared spinner-loading
	// picker) so the same loading affordance as `mp go` applies here.
	return runDashboardTUI(ctx, dashboardLoadCmd(ctx, infos))
}

// dispatchPickedRow takes a dashboard.Row chosen by the user and runs the
// matching workflow: attach an existing piece/project, create a piece from an
// open issue, adopt an unadopted branch, or kick off the create-piece TUI.
func dispatchPickedRow(ctx context.Context, row dashboard.Row) error {
	switch row.Kind {
	case dashboard.RowProject, dashboard.RowPiece:
		return attachSession(ctx, row.SessionName, row.WorktreePath)
	case dashboard.RowIssue:
		return runSwitchFromIssue(ctx, projectFromRow(row), row.IssuePath)
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

func runSwitchFromIssue(ctx context.Context, proj registry.Project, issueQuery string) error {
	deps, handler := pieceHandlerForSwitch()
	ref, err := resolveIssueRef(deps, proj.Path, issueQuery)
	if err != nil {
		return err
	}

	// CreatePieceFromIssue currently reads cwd to find the repo root; the
	// simplest correct way to target the right project without refactoring
	// that path is to chdir for the duration of the call. This is a one-shot
	// CLI process, so the side effect is contained.
	restore, err := tempChdir(proj.Path)
	if err != nil {
		return err
	}
	defer restore()

	info, err := handler.CreatePieceWithInput(ctx, piececmd.NewPieceInput{
		Issue:      ref,
		SkipSwitch: true,
	}, piececmd.CreatePieceOptions{})
	if err != nil {
		return err
	}
	return attachSession(ctx, info.SessionName, info.WorktreePath)
}

func runSwitchFromBranch(ctx context.Context, proj registry.Project, branch string) error {
	_, handler := pieceHandlerForSwitch()
	info, err := handler.AdoptPiece(ctx, piececmd.AdoptPieceInput{
		RepoRoot: proj.Path,
		Branch:   branch,
	})
	if err != nil {
		return err
	}
	return attachSession(ctx, info.SessionName, info.WorktreePath)
}

// runSwitchNewPiece launches the existing piece-create TUI (modepicker →
// issuepicker or promptinput) targeted at the given project, then attaches.
// We chdir into proj.Path for the duration because the underlying create flow
// resolves its repo root from cwd.
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
	return func() { _ = os.Chdir(prev) }, nil
}

func getSwitchAllInput() (switchAllInput, bool, error) {
	switch {
	case flagSwitchProject != "" || flagSwitchPiece != "" || flagSwitchIssue != "" || flagSwitchBranch != "":
		return switchAllInput{
			Project: flagSwitchProject,
			Piece:   flagSwitchPiece,
			Issue:   flagSwitchIssue,
			Branch:  flagSwitchBranch,
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
		if in.Project == "" {
			return switchAllInput{}, false, fmt.Errorf("project is required")
		}
		return in, true, nil
	default:
		return switchAllInput{}, false, nil
	}
}
