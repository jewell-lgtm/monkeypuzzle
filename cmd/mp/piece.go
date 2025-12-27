package mp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
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

var flagMainBranch string
var flagPieceName string
var flagIssuePath string
var flagDryRun bool
var flagForce bool

func init() {
	pieceNewCmd.Flags().StringVar(&flagPieceName, "name", "", "Optional piece name (default: auto-generated)")
	pieceNewCmd.Flags().StringVar(&flagIssuePath, "issue", "", "Create piece from issue file (e.g., issues/foo.md)")
	pieceUpdateCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge (default: main)")
	pieceMergeCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge into (default: main)")
	pieceCleanupCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to check for merged status (default: main)")
	pieceCleanupCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Show what would be cleaned without making changes")
	pieceCleanupCmd.Flags().BoolVar(&flagForce, "force", false, "Skip confirmation prompts")
	pieceCmd.AddCommand(pieceNewCmd)
	pieceCmd.AddCommand(pieceUpdateCmd)
	pieceCmd.AddCommand(pieceMergeCmd)
	pieceCmd.AddCommand(pieceCleanupCmd)
	rootCmd.AddCommand(pieceCmd)
}

func runPieceStatus(cmd *cobra.Command, args []string) error {
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

	status, err := handler.Status(wd)
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
		info, err = handler.CreatePieceFromIssue(monkeypuzzleSourceDir, flagIssuePath)
	} else {
		info, err = handler.CreatePiece(monkeypuzzleSourceDir, flagPieceName)
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

	return nil
}

func runPieceUpdate(cmd *cobra.Command, args []string) error {
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

	if err := handler.UpdatePiece(wd, mainBranch); err != nil {
		return err
	}

	return nil
}

func runPieceMerge(cmd *cobra.Command, args []string) error {
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

	if err := handler.MergePiece(wd, mainBranch); err != nil {
		return err
	}

	return nil
}

func runPieceCleanup(cmd *cobra.Command, args []string) error {
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
	status, err := handler.Status(wd)
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

	results, err := handler.CleanupMergedPieces(repoRoot, opts)
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
