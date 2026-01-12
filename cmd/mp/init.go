package mp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	initTUI "github.com/jewell-lgtm/monkeypuzzle/internal/tui/init"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagName           string
	flagIssueProvider  string
	flagPRProvider     string
	flagLinearAPIKey   string
	flagLinearTeam     string
	flagYes            bool
	flagSchema         bool
	flagInitGitignore  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize monkeypuzzle in current directory",
	Long: `Initialize monkeypuzzle in current directory.

Modes:
  Interactive (default): TUI wizard for humans
  Stdin JSON:            Pipe JSON config to stdin
  All flags provided:    Direct mode, no prompts
  --schema:              Output expected JSON format

Examples:
  mp init                                    # Interactive wizard
  mp init --schema | jq '.name = "foo"' | mp init  # Pipe JSON
  mp init --name foo --issue-provider markdown --pr-provider github`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	initCmd.Flags().StringVar(&flagName, "name", "", "Project name")
	initCmd.Flags().StringVar(&flagIssueProvider, "issue-provider", "", "Issue provider (markdown, linear)")
	initCmd.Flags().StringVar(&flagPRProvider, "pr-provider", "", "PR provider (github)")
	initCmd.Flags().StringVar(&flagLinearAPIKey, "linear-api-key", "", "Linear API key (or use LINEAR_API_KEY env var)")
	initCmd.Flags().StringVar(&flagLinearTeam, "linear-team", "", "Linear team key (required for linear provider)")
	initCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "Overwrite existing config without prompting")
	initCmd.Flags().BoolVar(&flagSchema, "schema", false, "Output JSON schema with defaults and exit")
	initCmd.Flags().BoolVar(&flagInitGitignore, "gitignore", false, "Regenerate .monkeypuzzle/.gitignore only")

	// Register completion functions (errors ignored - completion is optional)
	_ = initCmd.RegisterFlagCompletionFunc("issue-provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"markdown", "linear"}, cobra.ShellCompDirectiveNoFileComp
	})
	_ = initCmd.RegisterFlagCompletionFunc("pr-provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"github"}, cobra.ShellCompDirectiveNoFileComp
	})
}

func runInit(cmd *cobra.Command, args []string) error {
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// --schema: output template and exit
	if flagSchema {
		schema, err := initcmd.Schema(wd)
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	// Create dependencies
	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
	)
	handler := initcmd.NewHandler(deps)

	// --gitignore: regenerate gitignore only
	if flagInitGitignore {
		if err := handler.EnsureGitignore(); err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "Regenerated .monkeypuzzle/.gitignore")
		return nil
	}

	// Check for existing config
	if handler.ConfigExists() && !flagYes {
		if !cli.IsTerminal() {
			return fmt.Errorf("config already exists, use --yes to overwrite")
		}
		fmt.Print("Config already exists. Overwrite? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		answer, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read input: %w", err)
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Println("Cancelled.")
			return nil
		}
	}

	// Get input based on mode
	input, err := getInput(wd)
	if err != nil {
		return err
	}

	cfg, err := handler.Run(input, wd)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func getInput(workDir string) (initcmd.Input, error) {
	allFlagsProvided := flagName != "" && flagIssueProvider != "" && flagPRProvider != ""

	var input initcmd.Input
	var err error

	switch {
	case allFlagsProvided:
		input = initcmd.Input{
			Name:          flagName,
			IssueProvider: flagIssueProvider,
			PRProvider:    flagPRProvider,
		}
		// Build issue config from flags
		if flagIssueProvider == "linear" {
			input.IssueConfig = make(map[string]string)
			if flagLinearTeam != "" {
				input.IssueConfig["team"] = flagLinearTeam
			}
			if flagLinearAPIKey != "" {
				input.IssueConfig["api_key"] = flagLinearAPIKey
			}
		}

	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return initcmd.Input{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = initcmd.ParseJSON(data)
		if err != nil {
			return initcmd.Input{}, err
		}

	case cli.IsTerminal():
		input, err = runInteractiveMode(workDir)
		if err != nil {
			return initcmd.Input{}, err
		}

	default:
		return initcmd.Input{}, fmt.Errorf("no input provided; use --schema to see expected format, or provide flags")
	}

	// Apply defaults and validate inside input layer
	input = initcmd.WithDefaults(input, workDir)
	if err := initcmd.Validate(input); err != nil {
		return initcmd.Input{}, err
	}

	return input, nil
}

func runInteractiveMode(workDir string) (initcmd.Input, error) {
	p := tea.NewProgram(initTUI.New())
	m, err := p.Run()
	if err != nil {
		return initcmd.Input{}, err
	}

	finalModel := m.(initTUI.Model)
	if finalModel.Cancelled {
		return initcmd.Input{}, fmt.Errorf("cancelled")
	}

	// Extract input from TUI model
	name := finalModel.ProjectName.Value()
	if name == "" {
		name = finalModel.ProjectName.Placeholder
	}

	// Get issue provider from selection
	issueProvider := initTUI.IssueProviders[finalModel.IssueMethod]

	// Get PR provider default
	fields := initcmd.Fields()
	var prProvider string
	for _, f := range fields {
		if f.Name == "pr_provider" {
			prProvider = f.Default
			break
		}
	}

	input := initcmd.Input{
		Name:          name,
		IssueProvider: issueProvider,
		PRProvider:    prProvider,
	}

	// Build issue config for linear provider
	if issueProvider == "linear" {
		input.IssueConfig = make(map[string]string)
		if team := finalModel.LinearTeam.Value(); team != "" {
			input.IssueConfig["team"] = team
		}
		if apiKey := finalModel.LinearAPIKey.Value(); apiKey != "" {
			input.IssueConfig["api_key"] = apiKey
		}
	}

	return input, nil
}

