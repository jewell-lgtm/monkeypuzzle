package mp

import (
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
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/pieceswitch"
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
The worktree will be created in XDG_DATA_HOME/monkeypuzzle/pieces (default: ~/.local/share/monkeypuzzle/pieces).`,
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

var flagMainBranch string
var flagSwitchName string
var flagPieceName string
var flagIssuePath string
var flagSkipSwitch bool
var flagDryRun bool
var flagForce bool

func init() {
	pieceNewCmd.Flags().StringVar(&flagPieceName, "name", "", "Optional piece name (default: auto-generated)")
	pieceNewCmd.Flags().StringVar(&flagIssuePath, "issue", "", "Create piece from issue file (e.g., issues/foo.md)")
	pieceNewCmd.Flags().BoolVar(&flagSkipSwitch, "skip-switch", false, "Don't switch to the new piece after creation")
	pieceUpdateCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge (default: main)")
	pieceMergeCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge into (default: main)")
	pieceCleanupCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to check for merged status (default: main)")
	pieceCleanupCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be cleaned without making changes")
	pieceCleanupCmd.Flags().BoolVar(&flagForce, "force", false, "Skip confirmation prompts")
	pieceSwitchCmd.Flags().StringVar(&flagSwitchName, "name", "", "Piece name to switch to")
	pieceCmd.AddCommand(pieceNewCmd)
	pieceCmd.AddCommand(pieceUpdateCmd)
	pieceCmd.AddCommand(pieceMergeCmd)
	pieceCmd.AddCommand(pieceCleanupCmd)
	pieceCmd.AddCommand(pieceSwitchCmd)
	rootCmd.AddCommand(pieceCmd)

	// Register completion functions
	pieceSwitchCmd.RegisterFlagCompletionFunc("name", completePieceNames)
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
	ctx := cmd.Context()
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Detect monkeypuzzle source directory
	// Try to find it by looking for the monkeypuzzle source repo
	// Start from current directory and walk up looking for go.mod with monkeypuzzle module
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

	var info piececmd.PieceInfo

	// Check if --issue flag is set
	if flagIssuePath != "" {
		// Validate that --name is not also set (they're mutually exclusive)
		if flagPieceName != "" {
			return fmt.Errorf("cannot use both --name and --issue flags together")
		}
		// Validate that issue path is not empty
		if strings.TrimSpace(flagIssuePath) == "" {
			return fmt.Errorf("--issue flag requires a non-empty path")
		}
		info, err = handler.CreatePieceFromIssue(ctx, monkeypuzzleSourceDir, flagIssuePath)
	} else {
		info, err = handler.CreatePiece(ctx, monkeypuzzleSourceDir, flagPieceName)
	}

	if err != nil {
		return err
	}

	// Output JSON to stdout
	jsonData, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal info: %w", err)
	}
	fmt.Println(string(jsonData))

	// Switch to the new piece (unless --skip-switch is set)
	if !flagSkipSwitch {
		_, err := handler.SwitchPiece(ctx, info.Name)
		if err != nil {
			// Non-fatal: log warning but don't fail
			fmt.Fprintf(os.Stderr, "Warning: failed to switch to piece: %v\n", err)
		}
	}

	return nil
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

	// Get available pieces first
	pieces, err := handler.ListPieces(ctx)
	if err != nil {
		return err
	}

	var selectedName string

	// Three input modes: flag, stdin JSON, TUI
	if flagSwitchName != "" {
		// Flag provided
		selectedName = flagSwitchName
	} else if hasSwitchStdinData() {
		// JSON from stdin
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		var input struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return fmt.Errorf("failed to parse JSON: %w", err)
		}
		selectedName = input.Name
	} else if isSwitchTerminal() {
		// Interactive TUI
		if len(pieces) == 0 {
			fmt.Fprintln(os.Stderr, "No pieces found. Use 'mp piece new' to create one.")
			return nil
		}

		p := tea.NewProgram(pieceswitch.New(pieces))
		m, err := p.Run()
		if err != nil {
			return fmt.Errorf("TUI error: %w", err)
		}

		finalModel := m.(pieceswitch.Model)
		if finalModel.Cancelled {
			return nil
		}

		selectedName = pieces[finalModel.Selected].Name
	} else {
		return fmt.Errorf("no piece name provided. Use --name flag or run interactively")
	}

	if selectedName == "" {
		return fmt.Errorf("piece name is required")
	}

	result, err := handler.SwitchPiece(ctx, selectedName)
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

func isSwitchTerminal() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func hasSwitchStdinData() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode()&os.ModeCharDevice) == 0 && fi.Size() > 0 ||
		(fi.Mode()&os.ModeNamedPipe) != 0
}
