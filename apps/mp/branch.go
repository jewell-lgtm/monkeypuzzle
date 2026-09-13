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
	branchcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/branch"
	stackcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/stack"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var branchCmd = &cobra.Command{
	Use: "branch", Aliases: []string{"branches"}, Short: "Inspect and manage mp branch layers",
	Long: `A branch is a layer recorded in an mp piece's stack. Raw Git refs are not
mp atoms: adopt one as a piece before managing it. With no subcommand, lists
all managed branch layers in the current project.`, Args: cobra.NoArgs, RunE: runBranchList,
}
var branchListCmd = &cobra.Command{Use: "list", Aliases: []string{"ls"}, Short: "List managed branch layers", Args: cobra.NoArgs, RunE: runBranchList}
var branchShowCmd = &cobra.Command{Use: "show [branch]", Aliases: []string{"status"}, Short: "Show a managed branch layer", Args: cobra.MaximumNArgs(1), RunE: runBranchShow}
var branchCreateCmd = &cobra.Command{Use: "create [name]", Aliases: []string{"new"}, Short: "Append a branch layer to the current piece", Args: cobra.MaximumNArgs(1), RunE: runBranchCreate}
var branchDeleteCmd = &cobra.Command{Use: "delete [name]", Aliases: []string{"remove", "rm"}, Short: "Remove the current managed stack tip", Args: cobra.MaximumNArgs(1), RunE: runBranchDelete}

var (
	flagBranchJSON, flagBranchSchema, flagBranchForce bool
	flagBranchName, flagBranchPrompt                  string
)

func init() {
	for _, cmd := range []*cobra.Command{branchCmd, branchListCmd, branchShowCmd} {
		cmd.Flags().BoolVar(&flagBranchJSON, "json", false, "Output JSON even on a terminal")
	}
	branchCreateCmd.Flags().StringVar(&flagBranchName, "name", "", "Branch name (alternative to the positional)")
	branchCreateCmd.Flags().StringVar(&flagBranchPrompt, "prompt", "", "Prompt used to derive the branch name")
	branchCreateCmd.Flags().BoolVar(&flagBranchSchema, "schema", false, "Print an example input document and exit")
	branchCreateCmd.Flags().BoolVar(&flagBranchJSON, "json", false, "Output JSON even on a terminal")
	branchDeleteCmd.Flags().StringVar(&flagBranchName, "name", "", "Branch name (alternative to the positional; defaults to current)")
	branchDeleteCmd.Flags().BoolVarP(&flagBranchForce, "force", "f", false, "Remove a tip that has a recorded PR")
	branchDeleteCmd.Flags().BoolVar(&flagBranchSchema, "schema", false, "Print an example input document and exit")
	branchDeleteCmd.Flags().BoolVar(&flagBranchJSON, "json", false, "Output JSON even on a terminal")
	branchCmd.AddCommand(branchListCmd, branchShowCmd, branchCreateCmd, branchDeleteCmd)
	rootCmd.AddCommand(branchCmd)
	branchShowCmd.ValidArgsFunction = completeManagedBranches
	branchDeleteCmd.ValidArgsFunction = completeManagedBranches
}

func branchDeps() core.Deps {
	return core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupNoopLoading())
}

func runBranchList(cmd *cobra.Command, _ []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	rows, err := branchcmd.NewHandler(branchDeps()).List(cmd.Context(), wd)
	if err != nil {
		return err
	}
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagBranchJSON {
		return cli.PrintJSON(map[string]any{"branches": rows})
	}
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "\tBRANCH\tBASE\tPIECE\tPR")
	for _, row := range rows {
		marker := " "
		if row.Current {
			marker = "*"
		}
		pr := ""
		if row.PRNumber != 0 {
			pr = fmt.Sprintf("#%d", row.PRNumber)
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", marker, row.Name, row.Base, row.Piece, pr)
	}
	return w.Flush()
}

func runBranchShow(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	name := ""
	if len(args) == 1 {
		name = args[0]
	}
	row, err := branchcmd.NewHandler(branchDeps()).Show(cmd.Context(), wd, name)
	if err != nil {
		return err
	}
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagBranchJSON {
		return cli.PrintJSON(row)
	}
	_, err = fmt.Fprintf(os.Stderr, "%s\nbase: %s\npiece: %s\nworktree: %s\n", row.Name, row.Base, row.Piece, row.Worktree)
	return err
}

func branchInput(cmd *cobra.Command, args []string, in any) error {
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, in); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	}
	if len(args) == 1 && cmd.Flags().Changed("name") {
		return fmt.Errorf("branch given both positionally and with --name")
	}
	return nil
}

func runBranchCreate(cmd *cobra.Command, args []string) error {
	if flagBranchSchema {
		data, err := stackcmd.AppendSchema()
		if err == nil {
			fmt.Println(string(data))
		}
		return err
	}
	in := stackcmd.AppendInput{}
	if err := branchInput(cmd, args, &in); err != nil {
		return err
	}
	if len(args) == 1 {
		in.Name = args[0]
	} else if cmd.Flags().Changed("name") {
		in.Name = flagBranchName
	}
	if cmd.Flags().Changed("prompt") {
		in.Prompt = flagBranchPrompt
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	h, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := h.Append(cmd.Context(), wd, in)
	if err != nil {
		return err
	}
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagBranchJSON {
		return nil
	}
	return cli.PrintJSON(result)
}

func runBranchDelete(cmd *cobra.Command, args []string) error {
	if flagBranchSchema {
		data, err := stackcmd.RemoveSchema()
		if err == nil {
			fmt.Println(string(data))
		}
		return err
	}
	in := stackcmd.RemoveInput{}
	if err := branchInput(cmd, args, &in); err != nil {
		return err
	}
	if len(args) == 1 {
		in.Name = args[0]
	} else if cmd.Flags().Changed("name") {
		in.Name = flagBranchName
	}
	if cmd.Flags().Changed("force") {
		in.Force = flagBranchForce
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	h, err := newStackHandler()
	if err != nil {
		return err
	}
	result, err := h.Remove(cmd.Context(), wd, in)
	if err != nil {
		return err
	}
	if cli.IsTerminal() && cli.IsStdoutTerminal() && !flagBranchJSON {
		return nil
	}
	return cli.PrintJSON(result)
}

func completeManagedBranches(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	rows, err := branchcmd.NewHandler(branchDeps()).List(cmd.Context(), wd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	values := make([]string, 0, len(rows))
	for _, row := range rows {
		if strings.HasPrefix(row.Name, toComplete) {
			values = append(values, row.Name)
		}
	}
	return values, cobra.ShellCompDirectiveNoFileComp
}
