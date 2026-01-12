package mp

import (
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	prcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var prCmd = &cobra.Command{
	Use:   "pr",
	Short: "Manage pull requests",
	Long:  `Commands for managing pull requests for pieces.`,
}

var prCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a pull request for the current piece",
	Long: `Create a GitHub pull request for the current piece worktree.
Pushes the branch to origin and creates a PR using the gh CLI.

If the piece was created from an issue, the issue title is used as the default PR title.`,
	RunE: runPRCreate,
}

var (
	flagPRTitle  string
	flagPRBody   string
	flagPRBase   string
	flagPRSchema bool
)

func init() {
	prCreateCmd.Flags().StringVar(&flagPRTitle, "title", "", "PR title (default: issue title or piece name)")
	prCreateCmd.Flags().StringVar(&flagPRBody, "body", "", "PR description")
	prCreateCmd.Flags().StringVar(&flagPRBase, "base", "", "Base branch to merge into (default: auto-detect from parent)")
	prCreateCmd.Flags().BoolVar(&flagPRSchema, "schema", false, "Output JSON schema and exit")
	prCmd.AddCommand(prCreateCmd)
	pieceCmd.AddCommand(prCmd)
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

	// Output JSON to stdout
	return cli.PrintJSON(result)
}

func getPRInput() (prcmd.Input, error) {
	var input prcmd.Input

	// Flags always take priority (base not included since it auto-detects)
	if flagPRTitle != "" || flagPRBody != "" {
		input = prcmd.Input{
			Title: flagPRTitle,
			Body:  flagPRBody,
			Base:  flagPRBase,
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
