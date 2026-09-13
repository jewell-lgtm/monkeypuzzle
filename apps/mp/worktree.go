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
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	worktreecmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/worktree"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/chooser"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var worktreeCmd = &cobra.Command{
	Use:     "worktree",
	Aliases: []string{"worktrees"},
	Short:   "Inspect storage used by mp piece worktrees",
	Long: `List the worktrees behind mp pieces, including their size and lifecycle
signals. Unmanaged Git worktrees are shown only as adoption candidates. On a
terminal, the bare command opens a management picker. Without a TTY it is a
read-only JSON list.`,
	Args: cobra.NoArgs,
	RunE: runWorktree,
}

var worktreeListCmd = &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List worktrees", Args: cobra.NoArgs, RunE: runWorktreeList}
var worktreeShowCmd = &cobra.Command{Use: "show [path|branch|piece]", Aliases: []string{"status"}, Short: "Show one worktree", Args: cobra.MaximumNArgs(1), RunE: runWorktreeShow}
var worktreeDeleteCmd = &cobra.Command{Use: "delete [path|branch|piece]", Aliases: []string{"remove", "rm"}, Short: "Abandon an mp piece by its worktree", Args: cobra.MaximumNArgs(1), RunE: runWorktreeDelete}

var (
	flagWorktreeJSON         bool
	flagWorktreeSelector     string
	flagWorktreeForce        bool
	flagWorktreeDeleteBranch bool
	flagWorktreeSchema       bool
)

func init() {
	for _, cmd := range []*cobra.Command{worktreeCmd, worktreeListCmd, worktreeShowCmd} {
		cmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "Output JSON even on a terminal")
	}
	worktreeDeleteCmd.Flags().StringVar(&flagWorktreeSelector, "worktree", "", "Worktree path, branch, or piece (alternative to positional)")
	worktreeDeleteCmd.Flags().BoolVarP(&flagWorktreeForce, "force", "f", false, "Discard uncommitted changes")
	worktreeDeleteCmd.Flags().BoolVar(&flagWorktreeDeleteBranch, "delete-branch", false, "Also delete the checked-out branch")
	worktreeDeleteCmd.Flags().BoolVar(&flagWorktreeSchema, "schema", false, "Print an example input document and exit")
	worktreeDeleteCmd.Flags().BoolVar(&flagWorktreeJSON, "json", false, "Output JSON even on a terminal")
	worktreeCmd.AddCommand(worktreeListCmd, worktreeShowCmd, worktreeDeleteCmd)
	rootCmd.AddCommand(worktreeCmd)
	for _, cmd := range []*cobra.Command{worktreeShowCmd, worktreeDeleteCmd} {
		cmd.ValidArgsFunction = completeWorktrees
	}
}

func newWorktreeHandler() *worktreecmd.Handler {
	deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupNoopLoading())
	return worktreecmd.NewHandler(deps, newPieceHandler(deps))
}

func runWorktree(cmd *cobra.Command, args []string) error {
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagWorktreeJSON {
		return runWorktreeTUI(cmd)
	}
	return runWorktreeList(cmd, args)
}

func worktreeRows(cmd *cobra.Command) ([]worktreecmd.Info, string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	rows, err := newWorktreeHandler().List(cmd.Context(), wd)
	return rows, wd, err
}

func runWorktreeList(cmd *cobra.Command, _ []string) error {
	rows, _, err := worktreeRows(cmd)
	if err != nil {
		return err
	}
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagWorktreeJSON {
		return cli.PrintJSON(map[string]any{"worktrees": rows})
	}
	renderWorktreeTable(rows)
	return nil
}

func renderWorktreeTable(rows []worktreecmd.Info) {
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "\tKIND\tBRANCH\tSIZE\tSTATE\tPATH")
	for _, row := range rows {
		marker := " "
		if row.Current {
			marker = "*"
		}
		kind := "worktree"
		if row.Main {
			kind = "main"
		} else if row.Managed {
			kind = "piece " + row.Piece
		}
		var state []string
		if row.Locked {
			state = append(state, "locked")
		}
		if row.Prunable {
			state = append(state, "prunable")
		}
		if row.Lifecycle != "" {
			state = append(state, row.Lifecycle)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n", marker, kind, row.Branch, humanBytes(row.SizeBytes), strings.Join(state, ","), row.Path)
	}
	_ = w.Flush()
}

func runWorktreeShow(cmd *cobra.Command, args []string) error {
	selector := ""
	if len(args) == 1 {
		selector = args[0]
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	row, err := newWorktreeHandler().Show(cmd.Context(), wd, selector)
	if err != nil {
		return err
	}
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagWorktreeJSON {
		return cli.PrintJSON(row)
	}
	renderWorktreeTable([]worktreecmd.Info{row})
	return nil
}

func runWorktreeDelete(cmd *cobra.Command, args []string) error {
	if flagWorktreeSchema {
		return cli.PrintJSON(worktreecmd.DeleteInput{Selector: "piece, branch, or path"})
	}
	var input worktreecmd.DeleteInput
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	}
	if len(args) == 1 && cmd.Flags().Changed("worktree") {
		return fmt.Errorf("worktree given both positionally and with --worktree")
	}
	if len(args) == 1 {
		input.Selector = args[0]
	} else if cmd.Flags().Changed("worktree") {
		input.Selector = flagWorktreeSelector
	}
	if cmd.Flags().Changed("force") {
		input.Force = flagWorktreeForce
	}
	if cmd.Flags().Changed("delete-branch") {
		input.DeleteBranch = flagWorktreeDeleteBranch
	}
	if strings.TrimSpace(input.Selector) == "" {
		return fmt.Errorf("worktree selector is required")
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := newWorktreeHandler().Delete(cmd.Context(), wd, input)
	if err != nil {
		return err
	}
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagWorktreeJSON {
		fmt.Fprintf(os.Stderr, "%s Deleted worktree %s\n", cli.GlyphOK, result.Path)
		return nil
	}
	return cli.PrintJSON(result)
}

func runWorktreeTUI(cmd *cobra.Command) error {
	rows, _, err := worktreeRows(cmd)
	if err != nil {
		return err
	}
	var options []chooser.Option
	byPath := make(map[string]worktreecmd.Info, len(rows))
	for _, row := range rows {
		if row.Main {
			continue
		}
		label := row.Branch
		if row.Piece != "" {
			label = row.Piece + "  (" + row.Branch + ")"
		} else if label == "" {
			label = row.Path
		}
		options = append(options, chooser.Option{Label: label, Desc: row.Path, Value: row.Path})
		byPath[row.Path] = row
	}
	if len(options) == 0 {
		fmt.Fprintln(os.Stderr, "No linked worktrees to manage.")
		return nil
	}
	path, ok, err := chooser.Run("Piece storage", []string{"Select a worktree to inspect or cull."}, options)
	if err != nil || !ok {
		return err
	}
	target := byPath[path]
	action, ok, err := chooser.Run("Manage "+target.Branch, []string{target.Path}, worktreeActionOptions(target))
	if err != nil || !ok {
		return err
	}
	if action == "show" {
		renderWorktreeTable([]worktreecmd.Info{target})
		return nil
	}
	if action == "adopt" {
		deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupCLILoading(os.Stderr))
		_, err := newPieceHandler(deps).AdoptPiece(cmd.Context(), piececmd.AdoptPieceInput{Branch: target.Branch})
		return err
	}
	if action == "done" {
		deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupCLILoading(os.Stderr))
		_, err := newPieceHandler(deps).DonePiece(cmd.Context(), target.Path, piececmd.DoneInput{Main: "main", MainBranch: "main"})
		return err
	}
	input := worktreecmd.DeleteInput{Selector: path, Force: action == "force", DeleteBranch: action == "delete-branch"}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := newWorktreeHandler().Delete(cmd.Context(), wd, input)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "%s Deleted worktree %s\n", cli.GlyphOK, result.Path)
	return nil
}

func worktreeActionOptions(target worktreecmd.Info) []chooser.Option {
	options := []chooser.Option{{Label: "Inspect only", Desc: "Show details and make no changes", Value: "show"}}
	// The process cannot safely remove the checkout it is executing from. Do not
	// offer actions that the handler would reject anyway.
	if target.Current || target.Main {
		return options
	}
	if !target.Managed {
		if target.Branch != "" && !target.Prunable {
			return append(options, chooser.Option{Label: "Adopt as an mp piece", Desc: "Move this checkout into mp and add lifecycle metadata", Value: "adopt"})
		}
		return options
	}
	if target.Lifecycle == "merged" {
		return append(options,
			chooser.Option{Label: "Finish merged piece", Desc: "Run the mp piece-done lifecycle and reclaim its worktree", Value: "done"},
			chooser.Option{Label: "Abandon piece and keep branch", Desc: "Run the mp piece-abandon lifecycle", Value: "delete"},
		)
	}
	return append(options,
		chooser.Option{Label: "Abandon piece and keep branch", Desc: "Remove it from the inbox and reclaim its worktree", Value: "delete"},
		chooser.Option{Label: "Abandon piece and branch", Desc: "Remove it from the inbox and apply branch safety checks", Value: "delete-branch"},
		chooser.Option{Label: "Force-abandon piece", Desc: "Discard uncommitted changes; keep branch", Value: "force"},
	)
}

func humanBytes(n int64) string {
	const unit = int64(1024)
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := unit, 0
	for value := n / unit; value >= unit && exp < 4; value /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func completeWorktrees(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	rows, _, err := worktreeRows(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var values []string
	for _, row := range rows {
		for _, value := range []string{row.Piece, row.Branch, row.Path} {
			if value != "" && strings.HasPrefix(value, toComplete) {
				values = append(values, value)
			}
		}
	}
	return values, cobra.ShellCompDirectiveNoFileComp
}
