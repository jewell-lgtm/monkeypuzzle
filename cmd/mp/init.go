package mp

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	initTUI "github.com/jewell-lgtm/monkeypuzzle/internal/tui/init"
)

var (
	flagName          string
	flagIssueProvider string
	flagPRProvider    string
	flagYes           bool
	flagSchema        bool
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
	initCmd.Flags().StringVar(&flagIssueProvider, "issue-provider", "", "Issue provider (markdown)")
	initCmd.Flags().StringVar(&flagPRProvider, "pr-provider", "", "PR provider (github)")
	initCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "Overwrite existing config without prompting")
	initCmd.Flags().BoolVar(&flagSchema, "schema", false, "Output JSON schema with defaults and exit")
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
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := initcmd.NewHandler(deps)

	// Check for existing config
	if handler.ConfigExists() && !flagYes {
		if !isTerminal() {
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

	return handler.Run(input)
}

func getInput(workDir string) (initcmd.Input, error) {
	allFlagsProvided := flagName != "" && flagIssueProvider != "" && flagPRProvider != ""
	hasStdin := hasStdinData()

	var input initcmd.Input
	var err error

	switch {
	case allFlagsProvided:
		input = initcmd.Input{
			Name:          flagName,
			IssueProvider: flagIssueProvider,
			PRProvider:    flagPRProvider,
		}

	case hasStdin:
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return initcmd.Input{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = initcmd.ParseJSON(data)
		if err != nil {
			return initcmd.Input{}, err
		}

	case isTerminal():
		input, err = runInteractiveMode(workDir)
		if err != nil {
			return initcmd.Input{}, err
		}

	default:
		return initcmd.Input{}, fmt.Errorf("no input provided; use --schema to see expected format, or provide flags")
	}

	// Apply defaults
	input = initcmd.WithDefaults(input, workDir)

	// Validate
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

	// Get defaults from field definitions
	fields := initcmd.Fields()
	var issueProvider, prProvider string
	for _, f := range fields {
		switch f.Name {
		case "issue_provider":
			issueProvider = f.Default
		case "pr_provider":
			prProvider = f.Default
		}
	}

	return initcmd.Input{
		Name:          name,
		IssueProvider: issueProvider,
		PRProvider:    prProvider,
	}, nil
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func hasStdinData() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode()&os.ModeCharDevice) == 0 && fi.Size() > 0 || (fi.Mode()&os.ModeNamedPipe) != 0
}
