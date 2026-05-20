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
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	issueTUI "github.com/jewell-lgtm/monkeypuzzle/internal/tui/issue"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/issuepicker"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagIssueTitle        string
	flagIssueDescription  string
	flagIssueSchema       bool
	flagIssueListStatus   []string
	flagIssueListSchema   bool
	flagIssueListAll      bool
	flagIssueSearchQuery  string
	flagIssueSearchStatus []string
	flagIssueSearchSchema bool
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

var issueSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search issues interactively",
	Long: `Search issues with fuzzy matching and live results.

Modes:
  Interactive (default): TUI with live search
  Flags/stdin:          Direct query
  --schema:             Output expected JSON format

Examples:
  mp issue search                      # Interactive search
  mp issue search --query "auth"       # Direct search
  mp issue search --status in-progress # Filter by status
  echo '{"query":"auth"}' | mp issue search  # Stdin JSON`,
	RunE: runIssueSearch,
}

func init() {
	issueCreateCmd.Flags().StringVar(&flagIssueTitle, "title", "", "Issue title")
	issueCreateCmd.Flags().StringVar(&flagIssueDescription, "description", "", "Issue description")
	issueCreateCmd.Flags().BoolVar(&flagIssueSchema, "schema", false, "Output JSON schema with defaults and exit")

	issueListCmd.Flags().StringSliceVar(&flagIssueListStatus, "status", nil, "Filter by status (todo, in-progress, done)")
	issueListCmd.Flags().BoolVar(&flagIssueListSchema, "schema", false, "Output JSON schema and exit")
	issueListCmd.Flags().BoolVar(&flagIssueListAll, "all", false, "List issues across all registered projects")

	issueSearchCmd.Flags().StringVar(&flagIssueSearchQuery, "query", "", "Search query (fuzzy match)")
	issueSearchCmd.Flags().StringSliceVar(&flagIssueSearchStatus, "status", nil, "Filter by status (todo, in-progress, done)")
	issueSearchCmd.Flags().BoolVar(&flagIssueSearchSchema, "schema", false, "Output JSON schema and exit")

	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueSearchCmd)
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
		adapters.SetupCLILoading(os.Stderr),
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

	input, err := getIssueListInput()
	if err != nil {
		return err
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)

	if flagIssueListAll {
		return runIssueListAll(deps, input.Status)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	handler := issue.NewHandler(deps, wd)

	issues, err := handler.ListIssues(input.Status)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	return cli.PrintJSON(issues)
}

// projectIssues is the per-project entry in `mp issue list --all` JSON output.
type projectIssues struct {
	Name   string                `json:"name"`
	Path   string                `json:"path"`
	Issues []issue.IssueListItem `json:"issues"`
	Error  string                `json:"error,omitempty"`
}

func runIssueListAll(deps core.Deps, statusFilter []string) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}

	results := make([]projectIssues, 0, len(reg.Projects))
	for _, p := range reg.Projects {
		entry := projectIssues{Name: p.Name, Path: p.Path}
		issues, err := issue.NewHandler(deps, p.Path).ListIssues(statusFilter)
		if err != nil {
			entry.Error = err.Error()
		} else {
			entry.Issues = issues
		}
		results = append(results, entry)
	}

	if !cli.IsStdoutTerminal() {
		return cli.PrintJSON(map[string]any{"projects": results})
	}

	if len(results) == 0 {
		fmt.Println("No registered projects. Run `mp init` in a repo, or `mp project add <path>`.")
		return nil
	}
	for i, entry := range results {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("# %s (%s)\n", entry.Name, entry.Path)
		if entry.Error != "" {
			fmt.Printf("  error: %s\n", entry.Error)
			continue
		}
		if len(entry.Issues) == 0 {
			fmt.Println("  (no issues)")
			continue
		}
		for _, it := range entry.Issues {
			num := it.Number
			if num != "" {
				num += " "
			}
			fmt.Printf("  [%s] %s%s\n", it.Status, num, it.Title)
		}
	}
	return nil
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

func runIssueSearch(cmd *cobra.Command, args []string) error {
	// --schema: output template and exit
	if flagIssueSearchSchema {
		schema, err := issue.SearchSchema()
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

	input, interactive, err := getIssueSearchInput()
	if err != nil {
		return err
	}

	// TUI handles its own loading state; CLI mode uses spinner
	var loading *core.LoadingSignal
	if !interactive {
		loading = adapters.SetupCLILoading(os.Stderr)
	}
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		loading,
	)
	handler := issue.NewHandler(deps, wd)

	// Interactive mode: show TUI picker
	if interactive {
		// First load initial issues
		initialIssues, err := handler.SearchIssues(issue.SearchInput{Limit: 100})
		if err != nil {
			return err
		}

		// Create search function for async fetches
		searchFn := func(query string) tea.Cmd {
			return func() tea.Msg {
				results, err := handler.SearchIssues(issue.SearchInput{
					Query: query,
					Limit: 100,
				})
				return issuepicker.IssuesLoadedMsg{
					Query:  query,
					Issues: results,
					Err:    err,
				}
			}
		}

		p := tea.NewProgram(issuepicker.NewWithSearch(initialIssues, searchFn))
		m, err := p.Run()
		if err != nil {
			return err
		}

		model := m.(issuepicker.Model)
		if model.Cancelled {
			return fmt.Errorf("cancelled")
		}

		selected, ok := model.SelectedIssue()
		if !ok {
			return fmt.Errorf("no issue selected")
		}

		return cli.PrintJSON(selected)
	}

	// Non-interactive: direct search
	issues, err := handler.SearchIssues(input)
	if err != nil {
		return err
	}

	return cli.PrintJSON(issues)
}

func getIssueSearchInput() (issue.SearchInput, bool, error) {
	hasFlagsOrStdin := flagIssueSearchQuery != "" || len(flagIssueSearchStatus) > 0 || cli.HasStdinData()

	// Interactive mode if TTY and no flags/stdin
	if cli.IsTerminal() && !hasFlagsOrStdin {
		return issue.SearchInput{}, true, nil
	}

	var input issue.SearchInput

	switch {
	case flagIssueSearchQuery != "" || len(flagIssueSearchStatus) > 0:
		input = issue.SearchInput{
			Query:  flagIssueSearchQuery,
			Status: flagIssueSearchStatus,
		}

	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return issue.SearchInput{}, false, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = issue.ParseSearchJSON(data)
		if err != nil {
			return issue.SearchInput{}, false, err
		}

	default:
		// No input - return all
		input = issue.SearchInput{}
	}

	if err := issue.ValidateSearchInput(input); err != nil {
		return issue.SearchInput{}, false, err
	}

	return input, false, nil
}
