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
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagFlattenForce          bool
	flagFlattenDeleteBranches bool
	flagFlattenDryRun         bool
	flagFlattenApply          bool
	flagFlattenYes            bool
	flagFlattenSchema         bool
)

var flattenCmd = &cobra.Command{
	Use:   "flatten",
	Short: "Remove all piece worktrees",
	Long: `Remove every piece worktree for the current repository, returning it to a flat
main-only state. Each piece's multiplexer session is killed and its worktree removed.

Unlike "mp cleanup" (which only removes merged pieces), flatten removes ALL
pieces regardless of merge status. Branches are kept by default; pass
--delete-branches to remove them too. By default worktrees with uncommitted
changes are left in place — pass --force to discard those changes.

Dry-run by default: it reports what would be removed and changes nothing. Pass
--apply to actually flatten. In an interactive terminal you are shown the
preview and asked to confirm (--yes/-y skips the prompt and applies);
non-interactive callers (flags/stdin JSON) preview unless --apply (or
"apply": true) is given.

Modes:
  Flag:       mp flatten --apply
  Stdin JSON: echo '{"apply":true}' | mp flatten
  --schema:   Output expected JSON format`,
	Args: cobra.NoArgs,
	RunE: runFlatten,
}

func init() {
	flattenCmd.Flags().BoolVar(&flagFlattenForce, "force", false, "Force removal even with uncommitted changes")
	flattenCmd.Flags().BoolVar(&flagFlattenDeleteBranches, "delete-branches", false, "Also delete each piece's git branch")
	flattenCmd.Flags().BoolVar(&flagFlattenDryRun, "dry-run", false, "Show what would be removed without making changes")
	flattenCmd.Flags().BoolVar(&flagFlattenApply, "apply", false, "Apply the flatten (default is a dry-run preview)")
	flattenCmd.Flags().BoolVarP(&flagFlattenYes, "yes", "y", false, "Skip the interactive confirmation prompt (implies --apply)")
	flattenCmd.Flags().BoolVar(&flagFlattenSchema, "schema", false, "Print an example input document and exit")
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

	// List pieces up front so we can short-circuit when empty.
	pieces, err := handler.ListPieces(ctx, "")
	if err != nil {
		return err
	}
	if len(pieces) == 0 {
		fmt.Fprintln(os.Stderr, "No pieces to flatten.")
		return cli.PrintJSON(piececmd.FlattenResult{Removed: []piececmd.FlattenItem{}})
	}

	// Flatten is dry-run by default, like the other sweep ops (cleanup, stack
	// sync): --apply (or --yes on a terminal) opts in, --dry-run stays a
	// preview, an interactive terminal is asked to confirm after seeing the
	// preview, and any other (non-interactive) caller previews.
	opts := piececmd.FlattenOptions{
		Force:          input.Force,
		DeleteBranches: input.DeleteBranches,
	}
	names := make([]string, len(pieces))
	for i, p := range pieces {
		names[i] = p.Name
	}
	apply, err := resolveApply(input.Apply || flagFlattenYes, input.DryRun, len(pieces) > 0, func() (bool, error) {
		return confirmApply(
			fmt.Sprintf("Remove all %d piece worktree(s)?", len(pieces)),
			"Pieces: "+strings.Join(names, ", "),
		)
	})
	if err != nil {
		return err
	}
	opts.DryRun = !apply
	result, err := handler.FlattenPieces(ctx, "", opts)
	if err != nil {
		return err
	}
	if !apply {
		fmt.Fprintln(os.Stderr, "Dry run: nothing removed. Pass --apply to flatten.")
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
	if flagFlattenApply {
		input.Apply = true
	}

	return input, nil
}
