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
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/chooser"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagFlattenForce          bool
	flagFlattenDeleteBranches bool
	flagFlattenDryRun         bool
	flagFlattenYes            bool
	flagFlattenSchema         bool
)

var flattenCmd = &cobra.Command{
	Use:   "flatten",
	Short: "Remove all piece worktrees",
	Long: `Remove every piece worktree for the current repository, returning it to a flat
main-only state. Each piece's tmux session is killed and its worktree removed.

Unlike "mp cleanup" (which only removes merged pieces), flatten removes ALL
pieces regardless of merge status. Branches are kept by default; pass
--delete-branches to remove them too. By default worktrees with uncommitted
changes are left in place — pass --force to discard those changes.

Modes:
  Flag:       mp flatten --force
  Stdin JSON: echo '{"force":true}' | mp flatten
  --schema:   Output expected JSON format

In an interactive terminal you are asked to confirm before anything is removed;
pass --yes (or --force) to skip the prompt.`,
	RunE: runFlatten,
}

func init() {
	flattenCmd.Flags().BoolVar(&flagFlattenForce, "force", false, "Force removal even with uncommitted changes")
	flattenCmd.Flags().BoolVar(&flagFlattenDeleteBranches, "delete-branches", false, "Also delete each piece's git branch")
	flattenCmd.Flags().BoolVar(&flagFlattenDryRun, "dry-run", false, "Show what would be removed without making changes")
	flattenCmd.Flags().BoolVar(&flagFlattenYes, "yes", false, "Skip the interactive confirmation prompt")
	flattenCmd.Flags().BoolVar(&flagFlattenSchema, "schema", false, "Output JSON schema and exit")
	rootCmd.AddCommand(flattenCmd)
}

func runFlatten(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagFlattenSchema {
		schema, err := piececmd.FlattenSchema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	ctx := cmd.Context()
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := newPieceHandler(deps)

	input, err := getFlattenInput()
	if err != nil {
		return err
	}

	// List pieces up front so we can short-circuit when empty and confirm
	// before removing anything.
	pieces, err := handler.ListPieces(ctx, "")
	if err != nil {
		return err
	}
	if len(pieces) == 0 {
		fmt.Fprintln(os.Stderr, "No pieces to flatten.")
		return cli.PrintJSON(piececmd.FlattenResult{Removed: []piececmd.FlattenItem{}})
	}

	// Confirm this destructive op interactively, unless the caller opted out via
	// --yes/--force/--dry-run or is running non-interactively (flags or stdin).
	if !input.DryRun && !flagFlattenYes && !input.Force && cli.IsTerminal() && !cli.HasStdinData() {
		names := make([]string, len(pieces))
		for i, p := range pieces {
			names[i] = p.Name
		}
		choice, ok, cerr := chooser.Run(
			fmt.Sprintf("Remove all %d piece worktree(s)?", len(pieces)),
			[]string{"Pieces: " + strings.Join(names, ", ")},
			[]chooser.Option{
				{Label: "Remove all pieces", Desc: "kills sessions and removes worktrees", Value: "yes"},
				{Label: "Cancel", Desc: "do nothing", Value: ""},
			},
		)
		if cerr != nil {
			return cerr
		}
		if !ok || choice == "" {
			fmt.Fprintln(os.Stderr, "Cancelled.")
			return nil
		}
	}

	opts := piececmd.FlattenOptions(input)
	result, err := handler.FlattenPieces(ctx, "", opts)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}

func getFlattenInput() (piececmd.FlattenInput, error) {
	var input piececmd.FlattenInput

	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return input, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseFlattenJSON(data)
		if err != nil {
			return input, err
		}
	}

	// Flags override stdin.
	if flagFlattenForce {
		input.Force = true
	}
	if flagFlattenDeleteBranches {
		input.DeleteBranches = true
	}
	if flagFlattenDryRun {
		input.DryRun = true
	}

	return input, nil
}
