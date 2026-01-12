package mp

import (
	"fmt"
	"io"
	"net/http"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	issueTUI "github.com/jewell-lgtm/monkeypuzzle/internal/tui/issue"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagIssueTitle       string
	flagIssueDescription string
	flagIssueSchema      bool
	flagIssueListStatus  []string
	flagIssueListSchema  bool
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Manage issues",
	Long:  `Create and manage issues.`,
}

var issueCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new issue",
	Long: `Create a new markdown issue file.

Modes:
  Interactive (default): TUI wizard for humans
  Stdin JSON:            Pipe JSON to stdin
  All flags provided:    Direct mode, no prompts
  --schema:              Output expected JSON format

Examples:
  mp issue create                              # Interactive wizard
  mp issue create --title "Add feature X"     # Direct mode
  mp issue create --schema | jq '.title = "foo"' | mp issue create  # Pipe JSON`,
	RunE: runIssueCreate,
}

var issueListCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues",
	Long: `List issues from the configured issues directory.

Modes:
  Flags/stdin:  Filter by status
  --schema:     Output expected JSON format

Examples:
  mp issue list                        # List all issues
  mp issue list --status todo          # Filter by status
  mp issue list --status todo,in-progress  # Multiple statuses
  echo '{"status":["todo"]}' | mp issue list  # Stdin JSON`,
	RunE: runIssueList,
}

func init() {
	issueCreateCmd.Flags().StringVar(&flagIssueTitle, "title", "", "Issue title")
	issueCreateCmd.Flags().StringVar(&flagIssueDescription, "description", "", "Issue description")
	issueCreateCmd.Flags().BoolVar(&flagIssueSchema, "schema", false, "Output JSON schema with defaults and exit")

	issueListCmd.Flags().StringSliceVar(&flagIssueListStatus, "status", nil, "Filter by status (todo, in-progress, done)")
	issueListCmd.Flags().BoolVar(&flagIssueListSchema, "schema", false, "Output JSON schema and exit")

	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueListCmd)
	rootCmd.AddCommand(issueCmd)
}

func runIssueCreate(cmd *cobra.Command, args []string) error {
	// --schema: output template and exit
	if flagIssueSchema {
		schema, err := issue.Schema()
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

	// Create dependencies
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
	)
	handler := issue.NewHandler(deps, wd)

	// Get input based on mode
	input, err := getIssueInput()
	if err != nil {
		return err
	}

	result, err := handler.Run(input)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	return cli.PrintJSON(result)
}

func getIssueInput() (issue.Input, error) {
	allFlagsProvided := flagIssueTitle != ""

	var input issue.Input
	var err error

	switch {
	case allFlagsProvided:
		input = issue.Input{
			Title:       flagIssueTitle,
			Description: flagIssueDescription,
		}

	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return issue.Input{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = issue.ParseJSON(data)
		if err != nil {
			return issue.Input{}, err
		}

	case cli.IsTerminal():
		input, err = runIssueInteractiveMode()
		if err != nil {
			return issue.Input{}, err
		}

	default:
		return issue.Input{}, fmt.Errorf("no input provided; use --schema to see expected format, or provide --title flag")
	}

	// Apply defaults and validate inside input layer
	input = issue.WithDefaults(input)
	if err := issue.Validate(input); err != nil {
		return issue.Input{}, err
	}

	return input, nil
}

func runIssueInteractiveMode() (issue.Input, error) {
	p := tea.NewProgram(issueTUI.New())
	m, err := p.Run()
	if err != nil {
		return issue.Input{}, err
	}

	finalModel := m.(issueTUI.Model)
	if finalModel.Cancelled {
		return issue.Input{}, fmt.Errorf("cancelled")
	}

	return issue.Input{
		Title:       finalModel.Title.Value(),
		Description: finalModel.Description.Value(),
	}, nil
}

func runIssueList(cmd *cobra.Command, args []string) error {
	// --schema: output template and exit
	if flagIssueListSchema {
		schema, err := issue.ListSchema()
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

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
	)
	handler := issue.NewHandler(deps, wd)

	input, err := getIssueListInput()
	if err != nil {
		return err
	}

	issues, err := handler.ListIssues(input.Status)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	return cli.PrintJSON(issues)
}

func getIssueListInput() (issue.ListInput, error) {
	var input issue.ListInput

	switch {
	case len(flagIssueListStatus) > 0:
		input = issue.ListInput{Status: flagIssueListStatus}

	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return issue.ListInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = issue.ParseListJSON(data)
		if err != nil {
			return issue.ListInput{}, err
		}

	default:
		// No filter - list all
		input = issue.ListInput{}
	}

	if err := issue.ValidateListInput(input); err != nil {
		return issue.ListInput{}, err
	}

	return input, nil
}
