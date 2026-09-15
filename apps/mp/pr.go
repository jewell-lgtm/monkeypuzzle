package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	prcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Manage pull requests",
	Long:  `Inspect PR/MR associations recorded on mp branch layers, or create and advance them.`,
	Args:  cobra.NoArgs,
	RunE:  runPRList,
}

var prListCmd = &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List PRs recorded on managed branches", Args: cobra.NoArgs, RunE: runPRList}
var prShowCmd = &cobra.Command{Use: "show [number|branch]", Aliases: []string{"status"}, Short: "Show a recorded PR", Args: cobra.MaximumNArgs(1), RunE: runPRShow}

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a PR/MR for the current piece",
	Long: `Create a pull or merge request for the current piece worktree. Pushes the
branch to origin and uses the provider configured by mp init (GitHub or GitLab).

When no title is provided, the piece name is used.`,
	Args: cobra.NoArgs,
	RunE: runPRCreate,
}

var prReadyCmd = &cobra.Command{
	Use:   "ready",
	Short: "Flip the current piece's draft PR/MR to ready-for-review",
	Long: `Flip the draft PR/MR for the current piece worktree to ready-for-review.
Reads the PR number from .monkeypuzzle/pr-metadata.json. Fires the
before-pr-ready and after-pr-ready hooks around the provider call.`,
	Args: cobra.NoArgs,
	RunE: runPRReady,
}

var (
	flagPRTitle       string
	flagPRBody        string
	flagPRBase        string
	flagPRDraft       bool
	flagPRSchema      bool
	flagPRJSON        bool
	flagPRReadySchema bool
	flagPRReadyJSON   bool
	flagPRListJSON    bool
)

func init() {
	prCreateCmd.Flags().StringVar(&flagPRTitle, "title", "", "PR/MR title (default: piece name)")
	prCreateCmd.Flags().StringVar(&flagPRBody, "body", "", "PR/MR description")
	prCreateCmd.Flags().StringVar(&flagPRBase, "base", "", "Base branch to merge into (default: auto-detect from parent)")
	prCreateCmd.Flags().BoolVar(&flagPRDraft, "draft", false, "Open the PR/MR as a draft")
	prCreateCmd.Flags().BoolVar(&flagPRSchema, "schema", false, "Print an example input document and exit")
	prCreateCmd.Flags().BoolVar(&flagPRJSON, "json", false, "Output JSON even on a terminal")
	prReadyCmd.Flags().BoolVar(&flagPRReadySchema, "schema", false, "Print an example input document and exit")
	prReadyCmd.Flags().BoolVar(&flagPRReadyJSON, "json", false, "Output JSON even on a terminal")
	for _, cmd := range []*cobra.Command{prCmd, prListCmd, prShowCmd} {
		cmd.Flags().BoolVar(&flagPRListJSON, "json", false, "Output JSON even on a terminal")
	}
	prCmd.AddCommand(prListCmd, prShowCmd, prCreateCmd)
	prCmd.AddCommand(prReadyCmd)
	rootCmd.AddCommand(prCmd)
}

func newPRHandler() *prcmd.Handler {
	deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupNoopLoading())
	return prcmd.NewHandler(deps)
}

func runPRList(cmd *cobra.Command, _ []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	rows, err := newPRHandler().List(cmd.Context(), wd)
	if err != nil {
		return err
	}
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagPRListJSON {
		return cli.PrintJSON(map[string]any{"prs": rows})
	}
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "PR\tBRANCH\tBASE\tPIECE\tSTATUS")
	for _, row := range rows {
		_, _ = fmt.Fprintf(w, "#%d\t%s\t%s\t%s\t%s\n", row.Number, row.Branch, row.Base, row.Piece, row.Status)
	}
	return w.Flush()
}

func runPRShow(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	selector := ""
	if len(args) == 1 {
		selector = args[0]
	}
	row, err := newPRHandler().Show(cmd.Context(), wd, selector)
	if err != nil {
		return err
	}
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagPRListJSON {
		return cli.PrintJSON(row)
	}
	_, err = fmt.Fprintf(os.Stderr, "#%d %s → %s (%s)\n%s\n", row.Number, row.Branch, row.Base, row.Status, row.URL)
	return err
}

func runPRCreate(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagPRSchema {
		schema, err := prcmd.Schema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := prcmd.NewHandler(deps)

	// Get validated input
	input, err := getPRInput()
	if err != nil {
		return err
	}

	result, err := handler.CreatePR(ctx, wd, input)
	if err != nil {
		return err
	}

	cli.Hint("mp pr ready (when it's ready for review)")
	return emitResult(result, flagPRJSON)
}

func runPRReady(cmd *cobra.Command, args []string) error {
	// --schema mode: ready takes no input fields, but the quad (flags / stdin /
	// --schema / interactive) holds for every command so agents can introspect
	// them uniformly.
	if flagPRReadySchema {
		fmt.Println("{}")
		return nil
	}
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if len(strings.TrimSpace(string(data))) > 0 {
			var empty map[string]any
			if err := json.Unmarshal(data, &empty); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
		}
	}

	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := prcmd.NewHandler(deps)
	if err := handler.MarkReady(ctx, wd); err != nil {
		return err
	}
	cli.Hint("mp merge — or merge on the forge, then mp done")
	return emitResult(map[string]any{"status": "ready"}, flagPRReadyJSON)
}

func getPRInput() (prcmd.Input, error) {
	var input prcmd.Input

	// Flags always take priority (base not included since it auto-detects)
	if flagPRTitle != "" || flagPRBody != "" || flagPRDraft {
		input = prcmd.Input{
			Title: flagPRTitle,
			Body:  flagPRBody,
			Base:  flagPRBase,
			Draft: flagPRDraft,
		}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return prcmd.Input{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = prcmd.ParseJSON(data)
		if err != nil {
			return prcmd.Input{}, err
		}
	}
	// Note: PR create doesn't need TUI - all fields are optional and have sensible defaults

	// Apply defaults and validate
	input = prcmd.WithDefaults(input)
	if err := prcmd.Validate(input); err != nil {
		return prcmd.Input{}, err
	}

	return input, nil
}
