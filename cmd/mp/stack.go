package mp

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	stackcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var stackCmd = &cobra.Command{
	Use:   "stack",
	Short: "Manage stacks of pieces (git-town-style)",
	Long: `Whole-stack operations over pieces: sync a stack against main and itself,
inspect the tree against the GitHub PR list, and append/prepend pieces.

All operations are non-interactive: anything risky aborts cleanly and prints
plain-English next steps (e.g. which PR base to change on GitHub).`,
}

var stackStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the stack tree, PR state, and drift vs the GitHub PR list",
	RunE:  runStackStatus,
}

var stackSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Propagate main and each parent down through the stack",
	RunE:  runStackSync,
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

var (
	flagStackMain         string
	flagStackFromGitHub   bool
	flagStackApplyBases   bool
	flagStackStatusSchema bool

	flagStackStrategy   string
	flagStackPush       bool
	flagStackStackScope bool
	flagStackSyncSchema bool

	flagStackName          string
	flagStackPrompt        string
	flagStackAppendSchema  bool
	flagStackPrependSchema bool

	flagStackContinueSchema bool
)

func init() {
	stackStatusCmd.Flags().StringVar(&flagStackMain, "main", "main", "Main branch name")
	stackStatusCmd.Flags().BoolVar(&flagStackFromGitHub, "from-github", false, "Rebuild local lineage from open PR bases")
	stackStatusCmd.Flags().BoolVar(&flagStackApplyBases, "apply-bases", false, "Edit PR bases on GitHub to match local lineage")
	stackStatusCmd.Flags().BoolVar(&flagStackStatusSchema, "schema", false, "Output JSON schema and exit")

	stackSyncCmd.Flags().StringVar(&flagStackMain, "main", "main", "Main branch name")
	stackSyncCmd.Flags().StringVar(&flagStackStrategy, "strategy", "merge", "Sync strategy: merge (default) or rebase")
	stackSyncCmd.Flags().BoolVar(&flagStackPush, "push", false, "Push each branch after syncing")
	stackSyncCmd.Flags().BoolVar(&flagStackStackScope, "stack", false, "Limit to the current piece's stack (run from a piece worktree)")
	stackSyncCmd.Flags().BoolVar(&flagStackSyncSchema, "schema", false, "Output JSON schema and exit")

	stackAppendCmd.Flags().StringVar(&flagStackName, "name", "", "Piece name")
	stackAppendCmd.Flags().StringVar(&flagStackPrompt, "prompt", "", "Piece prompt (recorded in piece metadata; used to name the piece)")
	stackAppendCmd.Flags().BoolVar(&flagStackAppendSchema, "schema", false, "Output JSON schema and exit")

	stackPrependCmd.Flags().StringVar(&flagStackName, "name", "", "Piece name")
	stackPrependCmd.Flags().StringVar(&flagStackPrompt, "prompt", "", "Piece prompt (recorded in piece metadata; used to name the piece)")
	stackPrependCmd.Flags().BoolVar(&flagStackPrependSchema, "schema", false, "Output JSON schema and exit")

	stackContinueCmd.Flags().BoolVar(&flagStackContinueSchema, "schema", false, "Output JSON schema and exit")

	stackCmd.AddCommand(stackStatusCmd)
	stackCmd.AddCommand(stackSyncCmd)
	stackCmd.AddCommand(stackAppendCmd)
	stackCmd.AddCommand(stackPrependCmd)
	stackCmd.AddCommand(stackContinueCmd)
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
	if cmd.Flags().Changed("main") || flagStackFromGitHub || flagStackApplyBases {
		input = stackcmd.StatusInput{MainBranch: flagStackMain, FromGitHub: flagStackFromGitHub, ApplyBases: flagStackApplyBases}
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

	var input stackcmd.SyncInput
	if cmd.Flags().Changed("main") || cmd.Flags().Changed("strategy") || flagStackPush || flagStackStackScope {
		input = stackcmd.SyncInput{MainBranch: flagStackMain, Strategy: flagStackStrategy, Push: flagStackPush, Stack: flagStackStackScope}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if input, err = stackcmd.ParseSyncJSON(data); err != nil {
			return err
		}
	}

	handler, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := handler.Sync(cmd.Context(), wd, input)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
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
