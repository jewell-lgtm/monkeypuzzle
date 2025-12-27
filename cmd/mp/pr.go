package mp

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	prcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/pr"
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
	flagPRTitle string
	flagPRBody  string
	flagPRBase  string
)

func init() {
	prCreateCmd.Flags().StringVar(&flagPRTitle, "title", "", "PR title (default: issue title or piece name)")
	prCreateCmd.Flags().StringVar(&flagPRBody, "body", "", "PR description")
	prCreateCmd.Flags().StringVar(&flagPRBase, "base", "main", "Base branch to merge into")
	prCmd.AddCommand(prCreateCmd)
	pieceCmd.AddCommand(prCmd)
}

func runPRCreate(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := prcmd.NewHandler(deps)

	input := prcmd.Input{
		Title: flagPRTitle,
		Body:  flagPRBody,
		Base:  flagPRBase,
	}

	result, err := handler.CreatePR(wd, input)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}
