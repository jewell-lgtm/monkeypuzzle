package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

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

When no title is provided, the piece name is used as the default PR title.`,
	Args: cobra.NoArgs,
	RunE: runPRCreate,
}

var prReadyCmd = &cobra.Command{
	Use:   "ready",
	Short: "Flip the current piece's draft PR/MR to ready-for-review",
	Long: `Flip the draft PR/MR for the current piece worktree to ready-for-review.
Reads the PR number from .monkeypuzzle/pr-metadata.json. Fires the
before-pr-ready and after-pr-ready hooks around the provider call.`,
	Args: cobra.NoArgs,
	RunE: runPRReady,
}

var (
	flagPRTitle       string
	flagPRBody        string
	flagPRBase        string
	flagPRDraft       bool
	flagPRSchema      bool
	flagPRReadySchema bool
	flagPRReadyJSON   bool
)

func init() {
	prCreateCmd.Flags().StringVar(&flagPRTitle, "title", "", "PR title (default: piece name)")
	prCreateCmd.Flags().StringVar(&flagPRBody, "body", "", "PR description")
	prCreateCmd.Flags().StringVar(&flagPRBase, "base", "", "Base branch to merge into (default: auto-detect from parent)")
	prCreateCmd.Flags().BoolVar(&flagPRDraft, "draft", false, "Open the PR/MR as a draft")
	prCreateCmd.Flags().BoolVar(&flagPRSchema, "schema", false, "Output JSON schema and exit")
	prReadyCmd.Flags().BoolVar(&flagPRReadySchema, "schema", false, "Output JSON schema and exit")
	prReadyCmd.Flags().BoolVar(&flagPRReadyJSON, "json", false, "Output JSON even on a terminal")
	prCmd.AddCommand(prCreateCmd)
	prCmd.AddCommand(prReadyCmd)
	rootCmd.AddCommand(prCmd)
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
		adapters.SetupCLILoading(os.Stderr),
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

func runPRReady(cmd *cobra.Command, args []string) error {
	// --schema mode: ready takes no input fields, but the quad (flags / stdin /
	// --schema / interactive) holds for every command so agents can introspect
	// them uniformly.
	if flagPRReadySchema {
		fmt.Println("{}")
		return nil
	}
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		if len(strings.TrimSpace(string(data))) > 0 {
			var empty map[string]any
			if err := json.Unmarshal(data, &empty); err != nil {
				return fmt.Errorf("invalid JSON: %w", err)
			}
		}
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
	handler := prcmd.NewHandler(deps)
	if err := handler.MarkReady(ctx, wd); err != nil {
		return err
	}
	if flagPRReadyJSON || !cli.IsStdoutTerminal() {
		return cli.PrintJSON(map[string]any{"status": "ready"})
	}
	return nil
}

func getPRInput() (prcmd.Input, error) {
	var input prcmd.Input

	// Flags always take priority (base not included since it auto-detects)
	if flagPRTitle != "" || flagPRBody != "" || flagPRDraft {
		input = prcmd.Input{
			Title: flagPRTitle,
			Body:  flagPRBody,
			Base:  flagPRBase,
			Draft: flagPRDraft,
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
