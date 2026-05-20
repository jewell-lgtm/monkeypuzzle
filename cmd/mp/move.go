package mp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagMovePath   string
	flagMoveSchema bool
)

var moveCmd = &cobra.Command{
	Use:   "move <new-dir>",
	Short: "Relocate this project's monkeypuzzle directory",
	Long: `Move (or rename) the monkeypuzzle state directory for the current repository to a
new location relative to the repo root — e.g. into an already-gitignored
".DONOTCOMMIT/monkeypuzzle". The repo -> directory mapping is recorded in
~/.config/monkeypuzzle/project-dirs.json so all mp commands find the new location.

Existing piece worktrees are moved and their git pointers repaired. Git tracking
itself is left alone: if monkeypuzzle.json was committed you may want to run
"git add -A" for the move yourself.

Modes:
  Arg:        mp move .DONOTCOMMIT/monkeypuzzle
  Flag:       mp move --path .DONOTCOMMIT/monkeypuzzle
  Stdin JSON: echo '{"path":".DONOTCOMMIT/monkeypuzzle"}' | mp move
  --schema:   Output expected JSON format

Pass ".monkeypuzzle" to move back to the default (removes the mapping entry).`,
	RunE: runMove,
}

func init() {
	moveCmd.Flags().StringVar(&flagMovePath, "path", "", "New monkeypuzzle directory, relative to the repo root")
	moveCmd.Flags().BoolVar(&flagMoveSchema, "schema", false, "Output JSON schema and exit")
	rootCmd.AddCommand(moveCmd)
}

type moveInput struct {
	Path string `json:"path"`
}

type moveResult struct {
	RepoRoot string   `json:"repo_root"`
	OldDir   string   `json:"old_dir"`  // relative to repo root
	NewDir   string   `json:"new_dir"`  // relative to repo root
	OldPath  string   `json:"old_path"` // absolute
	NewPath  string   `json:"new_path"` // absolute
	Pieces   []string `json:"pieces"`   // piece worktrees that were moved/repaired
}

func runMove(cmd *cobra.Command, args []string) error {
	if flagMoveSchema {
		return cli.PrintJSON(moveInput{})
	}

	in, err := getMoveInput(args)
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	repoRoot, err := projectdir.MainRepoRoot(wd)
	if err != nil {
		return err
	}

	oldRel := projectdir.RelDir(repoRoot)
	oldAbs := filepath.Join(repoRoot, oldRel)
	if fi, err := os.Stat(oldAbs); err != nil || !fi.IsDir() {
		return fmt.Errorf("no monkeypuzzle directory at %s — run `mp init` first", oldAbs)
	}

	newRel := filepath.Clean(in.Path)
	if !projectdir.ValidRel(newRel) {
		return fmt.Errorf("invalid target %q: must be a relative path within the repo (no \"..\", no absolute paths)", in.Path)
	}
	if newRel == oldRel {
		return fmt.Errorf("monkeypuzzle directory is already at %q", newRel)
	}
	newAbs := filepath.Join(repoRoot, newRel)
	if _, err := os.Stat(newAbs); err == nil {
		return fmt.Errorf("target %s already exists", newAbs)
	}

	// Move the directory.
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return fmt.Errorf("failed to move %s -> %s: %w", oldAbs, newAbs, err)
	}

	// Repair git worktrees and mirror per-piece state directories.
	pieceNames, repairErr := repairWorktrees(repoRoot, newAbs, oldRel, newRel)

	// Record (or clear) the mapping.
	if err := projectdir.Set(repoRoot, newRel); err != nil {
		return fmt.Errorf("monkeypuzzle directory moved to %s but failed to record the mapping: %w", newAbs, err)
	}

	res := moveResult{
		RepoRoot: repoRoot,
		OldDir:   oldRel,
		NewDir:   newRel,
		OldPath:  oldAbs,
		NewPath:  newAbs,
		Pieces:   pieceNames,
	}
	fmt.Fprintf(os.Stderr, "Moved monkeypuzzle directory: %s -> %s\n", oldRel, newRel)
	if repairErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n  run `git -C %s worktree repair` to fix piece worktrees\n", repairErr, repoRoot)
	}
	fmt.Fprintln(os.Stderr, "Note: git tracking is unchanged; if monkeypuzzle.json was committed, run `git add -A` for the move.")
	return cli.PrintJSON(res)
}

// repairWorktrees moves each piece's per-piece state dir from oldRel to newRel
// and runs `git worktree repair` on the relocated worktrees. It returns the
// piece names found and a non-fatal error if the git repair step failed.
func repairWorktrees(repoRoot, newMpDir, oldRel, newRel string) ([]string, error) {
	piecesDir := filepath.Join(newMpDir, "pieces")
	entries, err := os.ReadDir(piecesDir)
	if err != nil {
		return nil, nil // no pieces
	}
	var names, worktrees []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		names = append(names, e.Name())
		wt := filepath.Join(piecesDir, e.Name())
		worktrees = append(worktrees, wt)

		oldInner := filepath.Join(wt, oldRel)
		newInner := filepath.Join(wt, newRel)
		if oldInner == newInner {
			continue
		}
		if fi, err := os.Stat(oldInner); err == nil && fi.IsDir() {
			if err := os.MkdirAll(filepath.Dir(newInner), 0o755); err != nil {
				return names, fmt.Errorf("failed to prepare %s: %w", filepath.Dir(newInner), err)
			}
			if err := os.Rename(oldInner, newInner); err != nil {
				return names, fmt.Errorf("failed to move per-piece dir for %q: %w", e.Name(), err)
			}
		}
	}
	if len(worktrees) == 0 {
		return names, nil
	}
	gitArgs := append([]string{"-C", repoRoot, "worktree", "repair"}, worktrees...)
	if out, err := exec.Command("git", gitArgs...).CombinedOutput(); err != nil {
		return names, fmt.Errorf("git worktree repair failed: %w: %s", err, string(out))
	}
	return names, nil
}

func getMoveInput(args []string) (moveInput, error) {
	switch {
	case len(args) > 0 && args[0] != "":
		return moveInput{Path: args[0]}, nil
	case flagMovePath != "":
		return moveInput{Path: flagMovePath}, nil
	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return moveInput{}, fmt.Errorf("failed to read stdin: %w", err)
		}
		var in moveInput
		if err := json.Unmarshal(data, &in); err != nil {
			return moveInput{}, fmt.Errorf("invalid JSON: %w", err)
		}
		if in.Path == "" {
			return moveInput{}, fmt.Errorf("path is required")
		}
		return in, nil
	default:
		return moveInput{}, fmt.Errorf(`no target given; pass a path arg, --path, or stdin JSON {"path":...}`)
	}
}
