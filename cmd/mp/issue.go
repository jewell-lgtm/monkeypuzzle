package mp

import (
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	issueTUI "github.com/jewell-lgtm/monkeypuzzle/internal/tui/issue"
)

var (
	flagIssueTitle       string
	flagIssueDescription string
	flagIssueSchema      bool
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

func init() {
	issueCreateCmd.Flags().StringVar(&flagIssueTitle, "title", "", "Issue title")
	issueCreateCmd.Flags().StringVar(&flagIssueDescription, "description", "", "Issue description")
	issueCreateCmd.Flags().BoolVar(&flagIssueSchema, "schema", false, "Output JSON schema with defaults and exit")
	issueCmd.AddCommand(issueCreateCmd)
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
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := issue.NewHandler(deps, wd)

	// Get input based on mode
	input, err := getIssueInput()
	if err != nil {
		return err
	}

	_, err = handler.Run(input)
	return err
}

func getIssueInput() (issue.Input, error) {
	allFlagsProvided := flagIssueTitle != ""
	hasStdin := hasStdinData()

	var input issue.Input
	var err error

	switch {
	case allFlagsProvided:
		input = issue.Input{
			Title:       flagIssueTitle,
			Description: flagIssueDescription,
		}

	case hasStdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return issue.Input{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = issue.ParseJSON(data)
		if err != nil {
			return issue.Input{}, err
		}

	case isTerminal():
		input, err = runIssueInteractiveMode()
		if err != nil {
			return issue.Input{}, err
		}

	default:
		return issue.Input{}, fmt.Errorf("no input provided; use --schema to see expected format, or provide --title flag")
	}

	// Apply defaults
	input = issue.WithDefaults(input)

	// Validate
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
