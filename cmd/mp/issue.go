package mp

import (
	"context"
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
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/importwizard"
	issueTUI "github.com/jewell-lgtm/monkeypuzzle/internal/tui/issue"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/issuepicker"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagIssueTitle        string
	flagIssueDescription  string
	flagIssueSchema       bool
	flagIssueListSchema   bool
	flagIssueListAll      bool
	flagIssueSearchQuery  string
	flagIssueSearchSchema bool
	flagIssueImportFrom   string
	flagIssueImportID     string
	flagIssueImportQuery  string
	flagIssueImportSchema bool
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
  --all:        List issues across all registered projects
  --schema:     Output expected JSON format

Examples:
  mp issue list                        # List all issues
  mp issue list --all                  # List across all projects`,
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
  echo '{"query":"auth"}' | mp issue search  # Stdin JSON`,
	RunE: runIssueSearch,
}

var issueImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a remote tracker issue as a local markdown issue",
	Long: `Fetch an issue from a configured import source (linear, plane) and write
it as a local markdown issue. Trackers are read-only one-shot import sources;
the local store is always markdown.

Modes:
  Interactive (default): pick source (if >1), search, pick issue
  Flags:                 --from <source> --id <id>  OR  --from <source> --query <q>
  Stdin JSON:            echo '{"from":"linear","id":"ABC-123"}' | mp issue import
  --schema:              Output expected JSON format

Non-interactive invocations fail loudly on ambiguity: no source with multiple
configured, or a --query matching multiple remote issues with no --id.

Examples:
  mp issue import --from linear --id ENG-123
  mp issue import --from plane --query "auth"
  echo '{"from":"linear","query":"login"}' | mp issue import`,
	RunE: runIssueImport,
}

func init() {
	issueCreateCmd.Flags().StringVar(&flagIssueTitle, "title", "", "Issue title")
	issueCreateCmd.Flags().StringVar(&flagIssueDescription, "description", "", "Issue description")
	issueCreateCmd.Flags().BoolVar(&flagIssueSchema, "schema", false, "Output JSON schema with defaults and exit")

	issueListCmd.Flags().BoolVar(&flagIssueListSchema, "schema", false, "Output JSON schema and exit")
	issueListCmd.Flags().BoolVar(&flagIssueListAll, "all", false, "List issues across all registered projects")

	issueSearchCmd.Flags().StringVar(&flagIssueSearchQuery, "query", "", "Search query (fuzzy match)")
	issueSearchCmd.Flags().BoolVar(&flagIssueSearchSchema, "schema", false, "Output JSON schema and exit")

	issueImportCmd.Flags().StringVar(&flagIssueImportFrom, "from", "", "Import source (linear, plane)")
	issueImportCmd.Flags().StringVar(&flagIssueImportID, "id", "", "Remote issue identifier (unique match)")
	issueImportCmd.Flags().StringVar(&flagIssueImportQuery, "query", "", "Text search to resolve a remote issue")
	issueImportCmd.Flags().BoolVar(&flagIssueImportSchema, "schema", false, "Output JSON schema and exit")

	issueCmd.AddCommand(issueCreateCmd)
	issueCmd.AddCommand(issueListCmd)
	issueCmd.AddCommand(issueSearchCmd)
	issueCmd.AddCommand(issueImportCmd)
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

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)

	if flagIssueListAll {
		return runIssueListAll(deps)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	handler := issue.NewHandler(deps, wd)

	issues, err := handler.ListIssues()
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

func runIssueListAll(deps core.Deps) error {
	reg, err := registry.Load()
	if err != nil {
		return err
	}

	results := make([]projectIssues, 0, len(reg.Projects))
	for _, p := range reg.Projects {
		entry := projectIssues{Name: p.Name, Path: p.Path}
		issues, err := issue.NewHandler(deps, p.Path).ListIssues()
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
			fmt.Printf("  %s%s\n", num, it.Title)
		}
	}
	return nil
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
	hasFlagsOrStdin := flagIssueSearchQuery != "" || cli.HasStdinData()

	// Interactive mode if TTY and no flags/stdin
	if cli.IsTerminal() && !hasFlagsOrStdin {
		return issue.SearchInput{}, true, nil
	}

	var input issue.SearchInput

	switch {
	case flagIssueSearchQuery != "":
		input = issue.SearchInput{
			Query: flagIssueSearchQuery,
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

func runIssueImport(cmd *cobra.Command, args []string) error {
	if flagIssueImportSchema {
		schema, err := issue.ImportSchema()
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
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := issue.NewHandler(deps, wd)
	ctx := context.Background()

	// Interactive mode: TTY with no flags/stdin selectors.
	hasSelectors := flagIssueImportFrom != "" || flagIssueImportID != "" || flagIssueImportQuery != ""
	if cli.IsTerminal() && !hasSelectors && !cli.HasStdinData() {
		return runIssueImportInteractive(ctx, handler)
	}

	in, err := getIssueImportInput()
	if err != nil {
		return err
	}

	result, err := handler.Import(ctx, in)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}

func getIssueImportInput() (issue.ImportInput, error) {
	switch {
	case flagIssueImportFrom != "" || flagIssueImportID != "" || flagIssueImportQuery != "":
		return issue.ImportInput{
			From:  flagIssueImportFrom,
			ID:    flagIssueImportID,
			Query: flagIssueImportQuery,
		}, nil

	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return issue.ImportInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		return issue.ParseImportJSON(data)

	default:
		return issue.ImportInput{}, fmt.Errorf("no import source/selector provided; use --from with --id or --query, pipe JSON, or run in a terminal (see --schema)")
	}
}

func runIssueImportInteractive(ctx context.Context, handler *issue.Handler) error {
	// Step 1: resolve the import source (pick if >1 configured).
	sources, err := handler.ConfiguredImportSources()
	if err != nil {
		return err
	}
	if len(sources) == 0 {
		return fmt.Errorf("no import source configured; run `mp init` and configure linear or plane")
	}

	src := sources[0]
	if len(sources) > 1 {
		labels := make([]string, len(sources))
		for i, s := range sources {
			labels[i] = s.Name
		}
		picker := importwizard.NewListPicker("Choose import source", labels)
		m, perr := tea.NewProgram(picker).Run()
		if perr != nil {
			return perr
		}
		final := m.(importwizard.ListPicker)
		if final.Cancelled {
			return fmt.Errorf("cancelled")
		}
		src = sources[final.Chosen]
	}

	// Step 2: prompt for a search query.
	qp := importwizard.NewQueryPrompt("Import from " + src.Name)
	qm, err := tea.NewProgram(qp).Run()
	if err != nil {
		return err
	}
	finalQP := qm.(importwizard.QueryPrompt)
	if finalQP.Cancelled {
		return fmt.Errorf("cancelled")
	}

	// Step 3: search and pick a remote issue.
	results, err := handler.SearchRemote(ctx, src, finalQP.Query, issue.DefaultSearchLimit)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("no %s issues matched query %q", src.Name, finalQP.Query)
	}

	labels := make([]string, len(results))
	for i, r := range results {
		if r.Title != "" {
			labels[i] = fmt.Sprintf("%s — %s", r.ID, r.Title)
		} else {
			labels[i] = r.ID
		}
	}
	picker := importwizard.NewListPicker("Choose issue to import", labels)
	pm, err := tea.NewProgram(picker).Run()
	if err != nil {
		return err
	}
	finalPicker := pm.(importwizard.ListPicker)
	if finalPicker.Cancelled {
		return fmt.Errorf("cancelled")
	}

	result, err := handler.WriteImported(results[finalPicker.Chosen])
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}
