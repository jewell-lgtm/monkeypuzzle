package mp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core/issue"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/issuepicker"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/pieceswitch"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var pieceCmd = &cobra.Command{
	Use:   "piece",
	Short: "Manage puzzle pieces",
	Long:  `Show piece status or create new pieces.`,
	RunE:  runPieceStatus,
}

var pieceNewCmd = &cobra.Command{
	Use:   "new",
	Short: "Create a new puzzle piece",
	Long: `Create a new puzzle piece by initializing a git worktree and opening a tmux session.
The worktree will be created in the platform-appropriate data directory (e.g., ~/Library/Application Support/monkeypuzzle/pieces on macOS, ~/.local/share/monkeypuzzle/pieces on Linux).`,
	RunE: runPieceNew,
}

var pieceUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update piece with latest from main branch",
	Long:  `Merges the main branch into the current piece's history. Must be run from within a piece worktree.`,
	RunE:  runPieceUpdate,
}

var pieceMergeCmd = &cobra.Command{
	Use:   "merge",
	Short: "Merge piece back into main branch",
	Long:  `Merges the piece branch back into main. Fails if main has commits not in the piece worktree. Must be run from within a piece worktree.`,
	RunE:  runPieceMerge,
}

var pieceCleanupCmd = &cobra.Command{
	Use:   "cleanup",
	Short: "Cleanup merged pieces",
	Long:  `Finds and removes pieces whose branches have been merged. Removes worktrees, kills tmux sessions, and updates issue status to done.`,
	RunE:  runPieceCleanup,
}

var pieceSwitchCmd = &cobra.Command{
	Use:   "switch",
	Short: "Switch to an existing piece",
	Long: `Switch to an existing piece worktree.
Tries to attach to the tmux session first, falls back to printing the path.
Use with: cd $(mp piece switch --name foo)`,
	RunE: runPieceSwitch,
}

var pieceAbandonCmd = &cobra.Command{
	Use:   "abandon",
	Short: "Abandon an unmerged piece",
	Long: `Remove a piece worktree, kill its tmux session, and optionally delete the branch.
Use --force to discard uncommitted changes.
Use --delete-branch to also remove the git branch.`,
	RunE: runPieceAbandon,
}

var flagMainBranch string
var flagSwitchName string
var flagPieceName string
var flagIssuePath string
var flagSkipSwitch bool
var flagDryRun bool
var flagForce bool
var flagAbandonName string
var flagDeleteBranch bool
var flagOverwriteSession bool
var flagPieceNewSchema bool

func init() {
	pieceNewCmd.Flags().StringVar(&flagPieceName, "name", "", "Optional piece name (default: auto-generated)")
	pieceNewCmd.Flags().StringVar(&flagIssuePath, "issue", "", "Create piece from issue file (e.g., issues/foo.md)")
	pieceNewCmd.Flags().BoolVar(&flagSkipSwitch, "skip-switch", false, "Don't switch to the new piece after creation")
	pieceNewCmd.Flags().BoolVar(&flagOverwriteSession, "overwrite-session", false, "Replace existing main repo tmux session")
	pieceNewCmd.Flags().BoolVar(&flagPieceNewSchema, "schema", false, "Output JSON schema and exit")
	pieceUpdateCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge (default: main)")
	pieceMergeCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge into (default: main)")
	pieceCleanupCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to check for merged status (default: main)")
	pieceCleanupCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be cleaned without making changes")
	pieceCleanupCmd.Flags().BoolVar(&flagForce, "force", false, "Skip confirmation prompts")
	pieceSwitchCmd.Flags().StringVar(&flagSwitchName, "name", "", "Piece name to switch to")
	pieceAbandonCmd.Flags().StringVar(&flagAbandonName, "name", "", "Piece name to abandon")
	pieceAbandonCmd.Flags().BoolVar(&flagForce, "force", false, "Force removal even with uncommitted changes")
	pieceAbandonCmd.Flags().BoolVar(&flagDeleteBranch, "delete-branch", false, "Also delete the git branch")
	pieceCmd.AddCommand(pieceNewCmd)
	pieceCmd.AddCommand(pieceUpdateCmd)
	pieceCmd.AddCommand(pieceMergeCmd)
	pieceCmd.AddCommand(pieceCleanupCmd)
	pieceCmd.AddCommand(pieceSwitchCmd)
	pieceCmd.AddCommand(pieceAbandonCmd)
	rootCmd.AddCommand(pieceCmd)

	// Register completion functions
	pieceSwitchCmd.RegisterFlagCompletionFunc("name", completePieceNames)
	pieceAbandonCmd.RegisterFlagCompletionFunc("name", completePieceNames)
	pieceNewCmd.RegisterFlagCompletionFunc("issue", completeIssueFiles)
	pieceUpdateCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
	pieceMergeCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
	pieceCleanupCmd.RegisterFlagCompletionFunc("main-branch", completeGitBranches)
}

func completePieceNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(io.Discard),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	pieces, err := handler.ListPieces(cmd.Context())
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var names []string
	for _, p := range pieces {
		if strings.HasPrefix(p.Name, toComplete) {
			names = append(names, p.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func completeIssueFiles(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Allow file completion, filtered to issues/ directory by default
	return nil, cobra.ShellCompDirectiveDefault
}

func completeGitBranches(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	exec := adapters.NewOSExec()
	out, err := exec.Run(cmd.Context(), "git", "branch", "--format=%(refname:short)")
	if err != nil {
		return []string{"main", "master"}, cobra.ShellCompDirectiveNoFileComp
	}
	branches := strings.Split(strings.TrimSpace(string(out)), "\n")
	var filtered []string
	for _, b := range branches {
		if strings.HasPrefix(b, toComplete) {
			filtered = append(filtered, b)
		}
	}
	return filtered, cobra.ShellCompDirectiveNoFileComp
}

func runPieceStatus(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	status, err := handler.Status(ctx, wd)
	if err != nil {
		return err
	}

	// Output to stderr for human-readable text
	if status.InPiece {
		fmt.Fprintf(os.Stderr, "Working on piece: %s\n", status.PieceName)
		fmt.Fprintf(os.Stderr, "Worktree path: %s\n", status.WorktreePath)
	} else {
		fmt.Fprintf(os.Stderr, "In main repository\n")
		if status.RepoRoot != "" {
			fmt.Fprintf(os.Stderr, "Repo root: %s\n", status.RepoRoot)
		}
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal status: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func runPieceNew(cmd *cobra.Command, args []string) error {
	// --schema mode: output JSON schema and exit
	if flagPieceNewSchema {
		schema, err := piececmd.NewPieceSchema()
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

	// Detect monkeypuzzle source directory
	monkeypuzzleSourceDir, err := findMonkeypuzzleSource(wd)
	if err != nil {
		return fmt.Errorf("failed to find monkeypuzzle source directory: %w", err)
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	// Get validated input from flags/stdin/TUI
	input, err := getPieceNewInput(deps, wd)
	if err != nil {
		return err
	}

	opts := piececmd.CreatePieceOptions{
		OverwriteSession: input.OverwriteSession,
	}

	// Unified handler routes based on input
	info, err := handler.CreatePieceWithInput(ctx, monkeypuzzleSourceDir, input, opts)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal info: %w", err)
	}
	fmt.Println(string(jsonData))

	// Switch to the new piece (unless skip_switch is set)
	if !input.SkipSwitch {
		_, err := handler.SwitchPiece(ctx, info.Name)
		if err != nil {
			// Non-fatal: log warning but don't fail
			fmt.Fprintf(os.Stderr, "Warning: failed to switch to piece: %v\n", err)
		}
	}

	return nil
}

func getPieceNewInput(deps core.Deps, workDir string) (piececmd.NewPieceInput, error) {
	var input piececmd.NewPieceInput
	var err error

	// Mode 1: Flags provided
	if flagIssuePath != "" || flagPieceName != "" {
		input = piececmd.NewPieceInput{
			IssuePath: flagIssuePath,
			Name:      flagPieceName,
		}
	} else if cli.HasStdinData() {
		// Mode 2: Stdin JSON
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.NewPieceInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseNewPieceJSON(data)
		if err != nil {
			return piececmd.NewPieceInput{}, err
		}
	} else if cli.IsTerminal() {
		// Mode 3: Interactive TUI
		input, err = runPieceNewTUI(deps, workDir)
		if err != nil {
			return piececmd.NewPieceInput{}, err
		}
	} else {
		return piececmd.NewPieceInput{}, fmt.Errorf("no input provided; use --schema to see expected format")
	}

	// Flags override stdin/TUI options (flags take priority)
	if flagSkipSwitch {
		input.SkipSwitch = true
	}
	if flagOverwriteSession {
		input.OverwriteSession = true
	}

	// Apply defaults and validate inside input layer
	input = piececmd.WithNewPieceDefaults(input)
	if err := piececmd.ValidateNewPieceInput(input); err != nil {
		return piececmd.NewPieceInput{}, err
	}

	return input, nil
}

func runPieceNewTUI(deps core.Deps, workDir string) (piececmd.NewPieceInput, error) {
	issueHandler := issue.NewHandler(deps, workDir)
	issues, err := issueHandler.ListIssues([]string{piececmd.StatusTodo})
	if err != nil {
		// Fall through to error - no issues available
		return piececmd.NewPieceInput{}, fmt.Errorf("failed to list issues: %w", err)
	}

	if len(issues) == 0 {
		return piececmd.NewPieceInput{}, fmt.Errorf("no todo issues found; create one with 'mp issue create' or use --name flag")
	}

	p := tea.NewProgram(issuepicker.New(issues))
	m, err := p.Run()
	if err != nil {
		return piececmd.NewPieceInput{}, fmt.Errorf("TUI error: %w", err)
	}

	finalModel := m.(issuepicker.Model)
	if finalModel.Cancelled {
		return piececmd.NewPieceInput{}, fmt.Errorf("cancelled")
	}

	return piececmd.NewPieceInput{
		IssuePath: issues[finalModel.Selected].Path,
	}, nil
}


func runPieceUpdate(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Default to "main" if not specified
	mainBranch := flagMainBranch
	if mainBranch == "" {
		mainBranch = "main"
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	if err := handler.UpdatePiece(ctx, wd, mainBranch); err != nil {
		return err
	}

	return nil
}

func runPieceMerge(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Default to "main" if not specified
	mainBranch := flagMainBranch
	if mainBranch == "" {
		mainBranch = "main"
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	if err := handler.MergePiece(ctx, wd, mainBranch); err != nil {
		return err
	}

	return nil
}

func runPieceCleanup(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Default to "main" if not specified
	mainBranch := flagMainBranch
	if mainBranch == "" {
		mainBranch = "main"
	}

	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	// Get repo root (either from piece or main repo)
	status, err := handler.Status(ctx, wd)
	if err != nil {
		return fmt.Errorf("failed to get piece status: %w", err)
	}

	repoRoot := status.RepoRoot
	if repoRoot == "" {
		return fmt.Errorf("not in a git repository")
	}

	opts := piececmd.CleanupOptions{
		DryRun:     flagDryRun,
		Force:      flagForce,
		MainBranch: mainBranch,
	}

	results, err := handler.CleanupMergedPieces(ctx, repoRoot, opts)
	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}
	fmt.Println(string(jsonData))

	return nil
}

func runPieceAbandon(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	// Get validated input
	input, err := getAbandonInput(ctx, handler)
	if err != nil {
		if err.Error() == "cancelled" || err.Error() == "no pieces" {
			return nil
		}
		return err
	}

	opts := piececmd.AbandonOptions{
		Force:        input.Force,
		DeleteBranch: input.DeleteBranch,
	}

	result, err := handler.AbandonPiece(ctx, input.Name, opts)
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

func getAbandonInput(ctx context.Context, handler *piececmd.Handler) (piececmd.AbandonInput, error) {
	var input piececmd.AbandonInput
	var err error

	if flagAbandonName != "" {
		input = piececmd.AbandonInput{Name: flagAbandonName}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.AbandonInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseAbandonJSON(data)
		if err != nil {
			return piececmd.AbandonInput{}, err
		}
	} else if cli.IsTerminal() {
		input, err = runAbandonTUI(ctx, handler)
		if err != nil {
			return piececmd.AbandonInput{}, err
		}
	} else {
		return piececmd.AbandonInput{}, fmt.Errorf("no input provided; use --name flag or run interactively")
	}

	// Flags override stdin/TUI options
	if flagForce {
		input.Force = true
	}
	if flagDeleteBranch {
		input.DeleteBranch = true
	}

	input = piececmd.WithAbandonDefaults(input)
	if err := piececmd.ValidateAbandonInput(input); err != nil {
		return piececmd.AbandonInput{}, err
	}

	return input, nil
}

func runAbandonTUI(ctx context.Context, handler *piececmd.Handler) (piececmd.AbandonInput, error) {
	pieces, err := handler.ListPieces(ctx)
	if err != nil {
		return piececmd.AbandonInput{}, err
	}

	if len(pieces) == 0 {
		fmt.Fprintln(os.Stderr, "No pieces to abandon.")
		return piececmd.AbandonInput{}, fmt.Errorf("no pieces")
	}

	p := tea.NewProgram(pieceswitch.New(pieces))
	m, err := p.Run()
	if err != nil {
		return piececmd.AbandonInput{}, fmt.Errorf("TUI error: %w", err)
	}

	finalModel := m.(pieceswitch.Model)
	if finalModel.Cancelled {
		return piececmd.AbandonInput{}, fmt.Errorf("cancelled")
	}

	return piececmd.AbandonInput{Name: pieces[finalModel.Selected].Name}, nil
}


// findMonkeypuzzleSource tries to find the monkeypuzzle source directory
// by walking up from the current directory looking for go.mod with monkeypuzzle module
func findMonkeypuzzleSource(startDir string) (string, error) {
	dir := startDir
	for {
		goModPath := filepath.Join(dir, "go.mod")
		if data, err := os.ReadFile(goModPath); err == nil {
			// Check if this is the monkeypuzzle module
			content := string(data)
			if containsMonkeypuzzleModule(content) {
				return dir, nil
			}
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root
			break
		}
		dir = parent
	}

	// Fallback: return current directory if we can't find it
	// This allows the command to work even if not in the monkeypuzzle repo
	return startDir, nil
}

func containsMonkeypuzzleModule(content string) bool {
	// Check for monkeypuzzle module name in go.mod
	return strings.Contains(content, "module github.com/jewell-lgtm/monkeypuzzle") ||
		strings.Contains(content, "module monkeypuzzle")
}

func runPieceSwitch(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()
	deps := core.Deps{
		FS:     adapters.NewOSFS(""),
		Output: adapters.NewTextOutput(os.Stderr),
		Exec:   adapters.NewOSExec(),
	}
	handler := piececmd.NewHandler(deps)

	// Get validated input
	input, err := getSwitchInput(ctx, handler)
	if err != nil {
		if err.Error() == "cancelled" || err.Error() == "no pieces" {
			return nil
		}
		return err
	}

	result, err := handler.SwitchPiece(ctx, input.Name)
	if err != nil {
		return err
	}

	// Output JSON to stdout (only if not printing path, which already uses stdout)
	if result.Method != "path" {
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal result: %w", err)
		}
		fmt.Println(string(jsonData))
	}

	return nil
}

func getSwitchInput(ctx context.Context, handler *piececmd.Handler) (piececmd.SwitchInput, error) {
	var input piececmd.SwitchInput
	var err error

	if flagSwitchName != "" {
		input = piececmd.SwitchInput{Name: flagSwitchName}
	} else if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return piececmd.SwitchInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		input, err = piececmd.ParseSwitchJSON(data)
		if err != nil {
			return piececmd.SwitchInput{}, err
		}
	} else if cli.IsTerminal() {
		input, err = runSwitchTUI(ctx, handler)
		if err != nil {
			return piececmd.SwitchInput{}, err
		}
	} else {
		return piececmd.SwitchInput{}, fmt.Errorf("no input provided; use --name flag or run interactively")
	}

	input = piececmd.WithSwitchDefaults(input)
	if err := piececmd.ValidateSwitchInput(input); err != nil {
		return piececmd.SwitchInput{}, err
	}

	return input, nil
}

func runSwitchTUI(ctx context.Context, handler *piececmd.Handler) (piececmd.SwitchInput, error) {
	pieces, err := handler.ListPieces(ctx)
	if err != nil {
		return piececmd.SwitchInput{}, err
	}

	if len(pieces) == 0 {
		fmt.Fprintln(os.Stderr, "No pieces found. Use 'mp piece new' to create one.")
		return piececmd.SwitchInput{}, fmt.Errorf("no pieces")
	}

	p := tea.NewProgram(pieceswitch.New(pieces))
	m, err := p.Run()
	if err != nil {
		return piececmd.SwitchInput{}, fmt.Errorf("TUI error: %w", err)
	}

	finalModel := m.(pieceswitch.Model)
	if finalModel.Cancelled {
		return piececmd.SwitchInput{}, fmt.Errorf("cancelled")
	}

	return piececmd.SwitchInput{Name: pieces[finalModel.Selected].Name}, nil
}

