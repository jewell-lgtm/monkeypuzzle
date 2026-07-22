package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	stackcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/textprompt"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Manage stacks of pieces (git-town-style)",
	Long: `Whole-stack operations over pieces: sync a stack against main and itself,
inspect the tree against the forge's PR/MR list, and append/prepend pieces.

Anything risky aborts cleanly and prints plain-English next steps (e.g. which
PR/MR base to change on the forge). 'sync' is dry-run by default and asks to
confirm in an interactive terminal; everything else runs straight through.`,
}

var stackStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the stack tree, PR/MR state, and drift vs the forge's PR/MR list",
	RunE:  runStackStatus,
}

var stackSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Propagate main and each parent down through the stack",
	Long: `Propagate main and each parent down through the stack (default merge strategy).

Dry-run by default: it previews which pieces would be synced and changes
nothing. Pass --apply to actually sync. In an interactive terminal you are shown
the preview and asked to confirm; non-interactive callers (flags/stdin JSON)
preview unless --apply (or "apply": true) is given.`,
	RunE: runStackSync,
}

var stackAppendCmd = &cobra.Command{
	Use:   "append [name]",
	Short: "Create a new piece as a child of the current piece",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStackAppend,
}

var stackPrependCmd = &cobra.Command{
	Use:   "prepend [name]",
	Short: "Insert a new piece between the current piece and its parent",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runStackPrepend,
}

var stackContinueCmd = &cobra.Command{
	Use:   "continue",
	Short: "Resume a conflicted rebase started by 'mp stack sync --strategy rebase'",
	RunE:  runStackContinue,
}

var stackSetParentCmd = &cobra.Command{
	Use:   "set-parent",
	Short: "Re-point a piece onto a new parent (metadata only; sync restacks)",
	Long: `Move a piece to a different parent in the stack. Defaults to the current
piece when run from a piece worktree. The target parent is another piece's
name, or "main" to make it a root piece.

Only piece metadata changes; run 'mp stack sync' afterwards to restack the
branches onto the new lineage.`,
	RunE: runStackSetParent,
}

var stackUndoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Restore every piece branch to the snapshot taken by the last 'mp stack sync'",
	Long: `Reset each piece branch back to the commit recorded before the last stack
sync. Refuses to run if an affected worktree has uncommitted changes. Local
only: remote branches are untouched (force-push with lease afterwards if you
had pushed).`,
	RunE: runStackUndo,
}

var stackGraphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Reconstruct a repo's stacked-PR forest straight from the forge (no clone)",
	Long: `Build the forest of stacked PRs for a repository purely from its open PRs'
base->head edges — no local clone required. Auth comes from the ambient
GH_TOKEN/GITHUB_TOKEN (or GITLAB_TOKEN) environment, so a server can run this as
a specific user. Output is the same forest the hosted dashboard renders, because
both call the shared stackgraph builder.`,
	RunE: runStackGraph,
}

var (
	flagStackMain         string
	flagStackFromRemote   bool
	flagStackFromGitHub   bool
	flagStackApplyBases   bool
	flagStackStatusSchema bool

	flagStackStrategy   string
	flagStackSyncFrom   string
	flagStackPush       bool
	flagStackStackScope bool
	flagStackSyncApply  bool
	flagStackSyncDryRun bool
	flagStackSyncSchema bool

	flagStackName          string
	flagStackPrompt        string
	flagStackAppendSchema  bool
	flagStackPrependSchema bool

	flagStackContinueSchema bool

	flagStackSetParentPiece  string
	flagStackSetParentParent string
	flagStackSetParentSchema bool

	flagStackGraphRepo     string
	flagStackGraphBranch   string
	flagStackGraphProvider string
	flagStackGraphLimit    int
	flagStackGraphSchema   bool

	flagStackStatusJSON bool
	flagStackGraphJSON  bool
	flagStackSyncJSON   bool
)

func init() {
	stackStatusCmd.Flags().StringVar(&flagStackMain, "main", "main", "Main branch name")
	stackStatusCmd.Flags().BoolVar(&flagStackFromRemote, "from-remote", false, "Rebuild local lineage from open PR/MR bases")
	stackStatusCmd.Flags().BoolVar(&flagStackFromGitHub, "from-github", false, "Deprecated: use --from-remote")
	_ = stackStatusCmd.Flags().MarkDeprecated("from-github", "use --from-remote")
	_ = stackStatusCmd.Flags().MarkHidden("from-github")
	stackStatusCmd.Flags().BoolVar(&flagStackApplyBases, "apply-bases", false, "Edit PR/MR bases on the forge to match local lineage")
	stackStatusCmd.Flags().BoolVar(&flagStackStatusSchema, "schema", false, "Output JSON schema and exit")
	stackStatusCmd.Flags().BoolVar(&flagStackStatusJSON, "json", false, "Output JSON even on a terminal")

	stackSyncCmd.Flags().StringVar(&flagStackMain, "main", "main", "Main branch name")
	stackSyncCmd.Flags().StringVar(&flagStackSyncFrom, "from", "", "Upstream ref to sync main from, e.g. origin/main (prompts when omitted on a terminal; defaults to origin/<main>)")
	stackSyncCmd.Flags().StringVar(&flagStackStrategy, "strategy", "merge", "Sync strategy: merge (default) or rebase")
	stackSyncCmd.Flags().BoolVar(&flagStackPush, "push", false, "Push each branch after syncing")
	stackSyncCmd.Flags().BoolVar(&flagStackStackScope, "stack", false, "Limit to the current piece's stack (run from a piece worktree)")
	stackSyncCmd.Flags().BoolVar(&flagStackSyncApply, "apply", false, "Apply the sync (default is a dry-run preview)")
	stackSyncCmd.Flags().BoolVar(&flagStackSyncDryRun, "dry-run", false, "Preview which pieces would be synced without changing anything")
	stackSyncCmd.Flags().BoolVar(&flagStackSyncSchema, "schema", false, "Output JSON schema and exit")
	stackSyncCmd.Flags().BoolVar(&flagStackSyncJSON, "json", false, "Output JSON even on a terminal")

	stackAppendCmd.Flags().StringVar(&flagStackName, "name", "", "Piece name")
	stackAppendCmd.Flags().StringVar(&flagStackPrompt, "prompt", "", "Piece prompt (recorded in piece metadata; used to name the piece)")
	stackAppendCmd.Flags().BoolVar(&flagStackAppendSchema, "schema", false, "Output JSON schema and exit")

	stackPrependCmd.Flags().StringVar(&flagStackName, "name", "", "Piece name")
	stackPrependCmd.Flags().StringVar(&flagStackPrompt, "prompt", "", "Piece prompt (recorded in piece metadata; used to name the piece)")
	stackPrependCmd.Flags().BoolVar(&flagStackPrependSchema, "schema", false, "Output JSON schema and exit")

	stackContinueCmd.Flags().BoolVar(&flagStackContinueSchema, "schema", false, "Output JSON schema and exit")

	stackSetParentCmd.Flags().StringVar(&flagStackSetParentPiece, "piece", "", "Piece to re-parent (default: current piece)")
	stackSetParentCmd.Flags().StringVar(&flagStackSetParentParent, "parent", "", "New parent piece name, or \"main\"")
	stackSetParentCmd.Flags().BoolVar(&flagStackSetParentSchema, "schema", false, "Output JSON schema and exit")

	stackGraphCmd.Flags().StringVar(&flagStackGraphRepo, "repo", "", "Repository as owner/name (required)")
	stackGraphCmd.Flags().StringVar(&flagStackGraphBranch, "default-branch", "", "Trunk branch (auto-detected from the forge if omitted)")
	stackGraphCmd.Flags().StringVar(&flagStackGraphProvider, "provider", "github", "Forge provider: github (default) or gitlab")
	stackGraphCmd.Flags().IntVar(&flagStackGraphLimit, "limit", 200, "Max PRs to fetch")
	stackGraphCmd.Flags().BoolVar(&flagStackGraphSchema, "schema", false, "Output JSON schema and exit")
	stackGraphCmd.Flags().BoolVar(&flagStackGraphJSON, "json", false, "Output JSON even on a terminal")

	stackCmd.AddCommand(stackStatusCmd)
	stackCmd.AddCommand(stackSyncCmd)
	stackCmd.AddCommand(stackAppendCmd)
	stackCmd.AddCommand(stackPrependCmd)
	stackCmd.AddCommand(stackContinueCmd)
	stackCmd.AddCommand(stackSetParentCmd)
	stackCmd.AddCommand(stackUndoCmd)
	stackCmd.AddCommand(stackGraphCmd)
	rootCmd.AddCommand(stackCmd)
}

func newStackHandler() (*stackcmd.Handler, error) {
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	return stackcmd.NewHandler(deps), nil
}

func runStackStatus(cmd *cobra.Command, args []string) error {
	if flagStackStatusSchema {
		schema, err := stackcmd.StatusSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	var input stackcmd.StatusInput
	if cmd.Flags().Changed("main") || flagStackFromRemote || flagStackFromGitHub || flagStackApplyBases {
		input = stackcmd.StatusInput{MainBranch: flagStackMain, FromRemote: flagStackFromRemote, FromGitHub: flagStackFromGitHub, ApplyBases: flagStackApplyBases}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if input, err = stackcmd.ParseStatusJSON(data); err != nil {
			return err
		}
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := handler.Status(cmd.Context(), wd, input)
	if err != nil {
		return err
	}
	// Humans at a terminal get the tree; pipes/agents keep the JSON contract.
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagStackStatusJSON {
		fmt.Print(renderStackStatus(result))
		return nil
	}
	return cli.PrintJSON(result)
}

func runStackGraph(cmd *cobra.Command, args []string) error {
	if flagStackGraphSchema {
		schema, err := stackcmd.GraphSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	var input stackcmd.GraphInput
	if cmd.Flags().Changed("repo") || cmd.Flags().Changed("default-branch") ||
		cmd.Flags().Changed("provider") || cmd.Flags().Changed("limit") {
		input = stackcmd.GraphInput{
			Repo:          flagStackGraphRepo,
			DefaultBranch: flagStackGraphBranch,
			Provider:      flagStackGraphProvider,
			Limit:         flagStackGraphLimit,
		}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if input, err = stackcmd.ParseGraphJSON(data); err != nil {
			return err
		}
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := handler.Graph(cmd.Context(), input)
	if err != nil {
		return err
	}
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagStackGraphJSON {
		fmt.Print(renderStackGraph(result))
		return nil
	}
	return cli.PrintJSON(result)
}

func runStackSync(cmd *cobra.Command, args []string) error {
	if flagStackSyncSchema {
		schema, err := stackcmd.SyncSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	input, err := getSyncInput(cmd)
	if err != nil {
		return err
	}

	// Ask a human where to sync main from when they didn't say. Non-interactive
	// callers fall through and the handler defaults to origin/<main>.
	if input.From == "" && cli.IsTerminal() && !cli.HasStdinData() {
		mainName := input.MainBranch
		if mainName == "" {
			mainName = "main"
		}
		from, ok, err := textprompt.Run("Sync the stack from which ref?", "origin/"+mainName)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("cancelled")
		}
		input.From = from
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}

	// Sync is dry-run by default. Always preview first, then decide whether to
	// apply: --apply opts in, --dry-run stays a preview, an interactive terminal
	// is asked to confirm, and any other (non-interactive) caller previews.
	preview := input
	preview.DryRun = true
	preview.Apply = false
	previewResult, err := handler.Sync(cmd.Context(), wd, preview)
	if err != nil {
		return err
	}

	apply, err := resolveApply(input.Apply, input.DryRun, len(previewResult.Updated) > 0, func() (bool, error) {
		return confirmApply(
			"Sync the stack?",
			fmt.Sprintf("Would sync %d piece(s) via %s: %s",
				len(previewResult.Updated), previewResult.Strategy, strings.Join(previewResult.Updated, ", ")),
		)
	})
	if err != nil {
		return err
	}
	if !apply {
		// The dry-run lines already streamed to the terminal; JSON is for pipes.
		if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagStackSyncJSON {
			return nil
		}
		return cli.PrintJSON(previewResult)
	}

	input.DryRun = false
	input.Apply = true
	result, err := handler.Sync(cmd.Context(), wd, input)
	if err != nil {
		return err
	}
	// The ✓ summary already streamed via Output on a terminal.
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagStackSyncJSON {
		return nil
	}
	return cli.PrintJSON(result)
}

// getSyncInput resolves `mp stack sync` input from stdin JSON, then overlays any
// explicitly-set flags (flags win over stdin).
func getSyncInput(cmd *cobra.Command) (stackcmd.SyncInput, error) {
	var input stackcmd.SyncInput
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return input, fmt.Errorf("failed to read stdin: %w", err)
		}
		if input, err = stackcmd.ParseSyncJSON(data); err != nil {
			return input, err
		}
	}
	if cmd.Flags().Changed("main") {
		input.MainBranch = flagStackMain
	}
	if cmd.Flags().Changed("from") {
		input.From = flagStackSyncFrom
	}
	if cmd.Flags().Changed("strategy") {
		input.Strategy = flagStackStrategy
	}
	if cmd.Flags().Changed("push") {
		input.Push = flagStackPush
	}
	if cmd.Flags().Changed("stack") {
		input.Stack = flagStackStackScope
	}
	if cmd.Flags().Changed("apply") {
		input.Apply = flagStackSyncApply
	}
	if cmd.Flags().Changed("dry-run") {
		input.DryRun = flagStackSyncDryRun
	}
	return input, nil
}

func runStackAppend(cmd *cobra.Command, args []string) error {
	if flagStackAppendSchema {
		schema, err := stackcmd.AppendSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	input, err := getAppendInput(args)
	if err != nil {
		return err
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	info, err := handler.Append(cmd.Context(), wd, input)
	if err != nil {
		return err
	}
	return cli.PrintJSON(info)
}

func runStackPrepend(cmd *cobra.Command, args []string) error {
	if flagStackPrependSchema {
		schema, err := stackcmd.AppendSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	in, err := getAppendInput(args)
	if err != nil {
		return err
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	info, err := handler.Prepend(cmd.Context(), wd, stackcmd.PrependInput(in))
	if err != nil {
		return err
	}
	return cli.PrintJSON(info)
}

// getAppendInput resolves append/prepend input from positional arg, flags, or stdin JSON.
func getAppendInput(args []string) (stackcmd.AppendInput, error) {
	var input stackcmd.AppendInput
	switch {
	case len(args) > 0:
		input = stackcmd.AppendInput{Name: args[0], Prompt: flagStackPrompt}
	case flagStackName != "" || flagStackPrompt != "":
		input = stackcmd.AppendInput{Name: flagStackName, Prompt: flagStackPrompt}
	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return stackcmd.AppendInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		if input, err = stackcmd.ParseAppendJSON(data); err != nil {
			return stackcmd.AppendInput{}, err
		}
	}
	return input, nil
}

func runStackContinue(cmd *cobra.Command, args []string) error {
	if flagStackContinueSchema {
		fmt.Println("{}")
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := handler.Continue(cmd.Context(), wd)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}

func runStackSetParent(cmd *cobra.Command, args []string) error {
	if flagStackSetParentSchema {
		schema, err := stackcmd.SetParentSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	var input stackcmd.SetParentInput
	if flagStackSetParentPiece != "" || flagStackSetParentParent != "" {
		input = stackcmd.SetParentInput{Piece: flagStackSetParentPiece, Parent: flagStackSetParentParent}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if input, err = stackcmd.ParseSetParentJSON(data); err != nil {
			return err
		}
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := handler.SetParent(cmd.Context(), wd, input)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}

func runStackUndo(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := handler.Undo(cmd.Context(), wd)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}
