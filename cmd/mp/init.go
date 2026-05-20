package mp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
	initTUI "github.com/jewell-lgtm/monkeypuzzle/internal/tui/init"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagName           string
	flagIssueProvider  string
	flagPRProvider     string
	flagLinearAPIKey   string
	flagLinearTeam     string
	flagPlaneAPIKey    string
	flagPlaneWorkspace string
	flagPlaneProject   string
	flagPlaneBaseURL   string
	flagInitDir        string
	flagYes            bool
	flagSchema         bool
	flagInitGitignore  bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize or refresh monkeypuzzle in current directory",
	Long: `Initialize or refresh monkeypuzzle in the current directory.

Re-running ` + "`mp init`" + ` in an already-configured repo is idempotent: it refreshes
.monkeypuzzle/.gitignore and the Claude skill but leaves monkeypuzzle.json
alone. ` + "`mp reinit`" + ` is an alias for the same operation. To reconfigure providers,
pass flags / stdin JSON / --yes — explicit input signals an overwrite.

Modes:
  Interactive (default): TUI wizard for humans (first-time only)
  Stdin JSON:            Pipe JSON config to stdin (overwrites)
  All flags provided:    Direct mode, no prompts (overwrites)
  --schema:              Output expected JSON format

Examples:
  mp init                                    # First time: wizard. Already set up: refresh.
  mp reinit                                  # Synonym for the refresh case.
  mp init --schema | jq '.name = "foo"' | mp init  # Pipe JSON (reconfigure)
  mp init --name foo --issue-provider markdown --pr-provider github  # Reconfigure`,
	RunE: runInit,
}

// reinitCmd is a pure synonym for init — same flags, same handler. Lets users
// type the verb that matches their intent ("refresh this repo's mp setup").
var reinitCmd = &cobra.Command{
	Use:   "reinit",
	Short: "Alias for `mp init` — refresh scaffolding in an existing repo",
	Long:  initCmd.Long,
	RunE:  runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(reinitCmd)
	// reinit shares init's flags so behaviour stays identical.
	reinitCmd.Flags().AddFlagSet(initCmd.Flags())
	initCmd.Flags().StringVar(&flagName, "name", "", "Project name")
	initCmd.Flags().StringVar(&flagIssueProvider, "issue-provider", "", "Issue provider (markdown, linear, plane)")
	initCmd.Flags().StringVar(&flagPRProvider, "pr-provider", "", "PR provider (github)")
	initCmd.Flags().StringVar(&flagLinearAPIKey, "linear-api-key", "", "Linear API key (or use LINEAR_API_KEY env var)")
	initCmd.Flags().StringVar(&flagLinearTeam, "linear-team", "", "Linear team key (required for linear provider)")
	initCmd.Flags().StringVar(&flagPlaneAPIKey, "plane-api-key", "", "Plane API key (or use PLANE_API_KEY env var)")
	initCmd.Flags().StringVar(&flagPlaneWorkspace, "plane-workspace", "", "Plane workspace slug (required for plane provider)")
	initCmd.Flags().StringVar(&flagPlaneProject, "plane-project", "", "Plane project ID (required for plane provider)")
	initCmd.Flags().StringVar(&flagPlaneBaseURL, "plane-base-url", "", "Plane API base URL (defaults to https://api.plane.so; set for self-hosted)")
	initCmd.Flags().StringVar(&flagInitDir, "dir", "", "Directory (relative to the repo root) for monkeypuzzle state (default .monkeypuzzle); the mapping is recorded in ~/.config/monkeypuzzle/project-dirs.json")
	initCmd.Flags().BoolVarP(&flagYes, "yes", "y", false, "Overwrite existing config without prompting")
	initCmd.Flags().BoolVar(&flagSchema, "schema", false, "Output JSON schema with defaults and exit")
	initCmd.Flags().BoolVar(&flagInitGitignore, "gitignore", false, "Regenerate .monkeypuzzle/.gitignore only")

	// Register completion functions (errors ignored - completion is optional)
	_ = initCmd.RegisterFlagCompletionFunc("issue-provider", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"markdown", "linear", "plane"}, cobra.ShellCompDirectiveNoFileComp
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
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := initcmd.NewHandler(deps)

	// Resolve the monkeypuzzle dir for this repo: explicit --dir wins, else any
	// recorded relocation, else the default.
	mpDir := flagInitDir
	if mpDir == "" {
		if root, rerr := registry.ResolveRepoRoot(wd); rerr == nil {
			mpDir = projectdir.RelDir(root)
		} else {
			mpDir = projectdir.DefaultDirName
		}
	}

	// --gitignore: regenerate gitignore only
	if flagInitGitignore {
		if err := handler.EnsureGitignore(mpDir); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Regenerated %s\n", filepath.Join(mpDir, ".gitignore"))
		return nil
	}

	// Existing repo: bare `mp init` / `mp reinit` is a refresh (regenerate
	// gitignore + Claude skill, keep monkeypuzzle.json). Explicit reconfigure
	// intent is signalled by --yes, any provider flag, or piped JSON.
	if handler.ConfigExists(mpDir) {
		explicitReconfigure := flagYes ||
			flagName != "" || flagIssueProvider != "" || flagPRProvider != "" ||
			cli.HasStdinData()

		if !explicitReconfigure {
			cfg, err := handler.Refresh(wd, mpDir)
			if err != nil {
				return err
			}
			jsonData, err := json.MarshalIndent(cfg, "", "  ")
			if err != nil {
				return fmt.Errorf("failed to marshal config: %w", err)
			}
			fmt.Println(string(jsonData))
			return nil
		}
	}

	// Get input based on mode
	input, err := getInput(wd, mpDir)
	if err != nil {
		return err
	}

	cfg, err := handler.Run(input, wd)
	if err != nil {
		return err
	}

	// Record the repo -> monkeypuzzle-dir mapping when relocated (non-default).
	if input.Dir != projectdir.DefaultDirName {
		if root, rerr := registry.ResolveRepoRoot(wd); rerr != nil {
			fmt.Fprintf(os.Stderr, "warning: not in a git repository; monkeypuzzle dir mapping not saved: %v\n", rerr)
		} else if serr := projectdir.Set(root, input.Dir); serr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not save monkeypuzzle dir mapping: %v\n", serr)
		}
	}

	// Register this repo in the global project registry (non-fatal on failure,
	// e.g. when not inside a git repository).
	if p, _, regErr := projectcmd.Add(wd); regErr != nil {
		fmt.Fprintf(os.Stderr, "warning: could not register project: %v\n", regErr)
	} else {
		fmt.Fprintf(os.Stderr, "Registered project %s\n", p.Name)
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func getInput(workDir, mpDir string) (initcmd.Input, error) {
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
		if flagIssueProvider == "plane" {
			input.IssueConfig = make(map[string]string)
			if flagPlaneWorkspace != "" {
				input.IssueConfig["workspace"] = flagPlaneWorkspace
			}
			if flagPlaneProject != "" {
				input.IssueConfig["project"] = flagPlaneProject
			}
			if flagPlaneAPIKey != "" {
				input.IssueConfig["api_key"] = flagPlaneAPIKey
			}
			if flagPlaneBaseURL != "" {
				input.IssueConfig["base_url"] = flagPlaneBaseURL
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

	// Resolve the monkeypuzzle dir: explicit --dir flag wins, then any value
	// supplied via stdin JSON, then the resolved default (recorded relocation
	// or ".monkeypuzzle").
	if flagInitDir != "" {
		input.Dir = flagInitDir
	} else if input.Dir == "" {
		input.Dir = mpDir
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
