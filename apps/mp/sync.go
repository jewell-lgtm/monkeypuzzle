package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var pieceSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync the current piece with its parent",
	Long: `Merge the current piece's parent into the piece. The parent comes from piece
metadata (another piece, or main for root pieces).

Defaults to origin's version of the parent: origin/<parent> is fetched and
merged. The local parent branch is used only when origin doesn't have it (or no
origin is configured), or when --local is passed. --from overrides the ref
entirely (e.g. upstream/main).

To sync a whole stack of pieces, use 'mp stack sync' instead.`,
	Args: cobra.NoArgs,
	RunE: runPieceSync,
}

var (
	flagSyncFrom   string
	flagSyncLocal  bool
	flagSyncSchema bool
)

func init() {
	pieceSyncCmd.Flags().StringVar(&flagMainBranch, "main", "main", "Trunk branch name, used when the piece's parent is main")
	pieceSyncCmd.Flags().StringVar(&flagMainBranchLegacy, "main-branch", "", "Deprecated alias for --main")
	_ = pieceSyncCmd.Flags().MarkDeprecated("main-branch", "use --main")
	pieceSyncCmd.Flags().StringVar(&flagSyncFrom, "from", "", "Explicit ref to sync from (e.g. origin/parent-branch)")
	pieceSyncCmd.Flags().BoolVar(&flagSyncLocal, "local", false, "Sync from the local parent branch instead of origin's version")
	pieceSyncCmd.Flags().BoolVar(&flagSyncSchema, "schema", false, "Output JSON schema and exit")
	rootCmd.AddCommand(pieceSyncCmd)

	_ = pieceSyncCmd.RegisterFlagCompletionFunc("main", completeGitBranches)
}

func runPieceSync(cmd *cobra.Command, args []string) error {
	// --schema mode
	if flagSyncSchema {
		schema, err := piececmd.SyncPieceSchema()
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
	handler := newPieceHandler(deps)

	input, err := getPieceSyncInput(cmd)
	if err != nil {
		return err
	}

	result, err := handler.SyncPiece(ctx, wd, input)
	if err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

// getPieceSyncInput resolves `mp sync` input from stdin JSON, then overlays any
// explicitly-set flags (flags win over stdin). The trunk flag (--main, or the
// deprecated --main-branch) defaults to "main", so it is gated via
// mainBranchFromFlags to avoid clobbering a stdin value.
func getPieceSyncInput(cmd *cobra.Command) (piececmd.SyncInput, error) {
	var input piececmd.SyncInput

	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.SyncInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseSyncJSON(data)
		if err != nil {
			return piececmd.SyncInput{}, err
		}
	}

	if v, ok := mainBranchFromFlags(cmd); ok {
		input.Main = v
		input.MainBranch = v
	}
	if cmd.Flags().Changed("from") {
		input.From = flagSyncFrom
	}
	if cmd.Flags().Changed("local") {
		input.Local = flagSyncLocal
	}

	return piececmd.WithSyncDefaults(input), nil
}
