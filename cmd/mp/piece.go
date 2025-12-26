package mp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

var flagMainBranch string

func init() {
	pieceUpdateCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge (default: main)")
	pieceMergeCmd.Flags().StringVar(&flagMainBranch, "main-branch", "main", "Main branch name to merge into (default: main)")
	pieceCmd.AddCommand(pieceNewCmd)
	pieceCmd.AddCommand(pieceUpdateCmd)
	pieceCmd.AddCommand(pieceMergeCmd)
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

	info, err := handler.CreatePiece(monkeypuzzleSourceDir)
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
	return containsSubstring(content, "module github.com/jewell-lgtm/monkeypuzzle") ||
		containsSubstring(content, "module monkeypuzzle")
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
