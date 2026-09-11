package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
	opencmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/open"
	piececmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/piece"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
)

// EnvOpen overrides the configured opener for one invocation.
const EnvOpen = "MP_OPEN"

var openCmd = &cobra.Command{
	Use:   "open [target]",
	Short: "Open a piece's worktree in your editor, IDE, or a new terminal",
	Long: `Hand a piece's worktree to the tool you actually work in — the answer to "take
me to this piece" when you don't use a multiplexer.

TARGET resolves like ` + "`mp switch`" + `: an existing piece (by name or by the branch
checked out in it), or a local/remote branch (adopted as a piece first). Unlike
switch it never creates anything. With no target it opens the piece you're
standing in, or the project's main worktree.

The opener is a command template, first of: --with, $MP_OPEN, or the
` + "`open_command`" + ` config key. It may use {path}, {piece}, {project} and {branch};
a template with none of them gets the path appended, so ` + "`code`" + ` means
` + "`code {path}`" + `. Every value is shell-quoted before substitution.

With no opener configured mp assumes nothing: it prints the worktree path and
shows how to set one.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runOpen,
}

var (
	flagOpenWith string
	flagOpenJSON bool
)

func init() {
	openCmd.Flags().StringVar(&flagOpenWith, "with", "", "Opener command for this call (overrides $MP_OPEN and open_command)")
	openCmd.Flags().BoolVar(&flagOpenJSON, "json", false, "Output JSON even on a terminal")
	openCmd.ValidArgsFunction = completePieceNames
	rootCmd.AddCommand(openCmd)
}

// openResult is what `mp open` reports: what it opened, and how.
type openResult struct {
	Piece   string `json:"piece"`
	Project string `json:"project"`
	Path    string `json:"path"`
	Branch  string `json:"branch,omitempty"`
	Command string `json:"command,omitempty"`
	Opened  bool   `json:"opened"`
}

func runOpen(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	target, err := resolveOpenTarget(ctx, args)
	if err != nil {
		return err
	}
	return openTarget(target, flagOpenWith, flagOpenJSON)
}

// resolveOpenTarget turns the command line into something to open: the named
// piece or branch, else the piece the caller is standing in, else the main
// worktree of the project they're in.
func resolveOpenTarget(ctx context.Context, args []string) (opencmd.Target, error) {
	_, handler := pieceHandlerForSwitch()

	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return opencmd.Target{}, fmt.Errorf("failed to get working directory: %w", err)
		}
		status, err := handler.Status(ctx, wd)
		if err != nil {
			return opencmd.Target{}, err
		}
		if status.RepoRoot == "" {
			return opencmd.Target{}, fmt.Errorf("not inside a monkeypuzzle project; name a piece, or run this from a repo")
		}
		project := projectNameFor(status.RepoRoot)
		if !status.InPiece {
			return opencmd.Target{Path: status.RepoRoot, Piece: "main", Project: project}, nil
		}
		return opencmd.Target{Path: status.WorktreePath, Piece: status.PieceName, Project: project}, nil
	}

	proj, err := resolveSwitchProject(ctx, "")
	if err != nil {
		return opencmd.Target{}, err
	}
	name := strings.TrimSpace(args[0])
	res, err := handler.ResolveSwitchTarget(ctx, proj.Path, name)
	if err != nil {
		return opencmd.Target{}, err
	}
	switch res.Kind {
	case piececmd.TargetMain:
		return opencmd.Target{Path: proj.Path, Piece: "main", Project: proj.Name}, nil
	case piececmd.TargetPiece:
		if res.Piece.IsPlaced() {
			return opencmd.Target{}, fmt.Errorf("%q is on %s at %s — open it there", name, res.Piece.Host, res.Piece.WorktreePath)
		}
		return opencmd.Target{Path: res.Piece.WorktreePath, Piece: res.Piece.Name, Project: proj.Name, Branch: res.Piece.Branch}, nil
	case piececmd.TargetAdoptLocal, piececmd.TargetAdoptRemote:
		info, err := handler.AdoptPiece(ctx, piececmd.AdoptPieceInput{RepoRoot: proj.Path, Branch: res.Branch})
		if err != nil {
			return opencmd.Target{}, err
		}
		return opencmd.Target{Path: info.WorktreePath, Piece: info.Name, Project: proj.Name, Branch: res.Branch}, nil
	default:
		return opencmd.Target{}, fmt.Errorf("nothing named %q in %s (no piece, local branch, or remote branch); create it with `mp create --name %s`", name, proj.Name, res.PieceName)
	}
}

// projectNameFor is the registered name of the project rooted at root, or its
// directory name.
func projectNameFor(root string) string {
	if name, ok := registry.ProjectName(root); ok && name != "" {
		return name
	}
	return filepath.Base(root)
}

// openTarget runs the resolved opener, or explains how to configure one.
func openTarget(target opencmd.Target, with string, jsonFlag bool) error {
	template := openerTemplate(with)
	result := openResult{
		Piece:   target.Piece,
		Project: target.Project,
		Path:    target.Path,
		Branch:  target.Branch,
	}

	if template == "" {
		// Assume nothing: the path is still useful, and so is knowing how to
		// make this do something next time.
		fmt.Println(target.Path)
		fmt.Fprintln(os.Stderr, "No opener configured. Set one:")
		for _, r := range opencmd.Recipes {
			fmt.Fprintln(os.Stderr, "  "+r)
		}
		return emitResult(result, jsonFlag)
	}

	line, err := opencmd.Command(template, target)
	if err != nil {
		return err
	}
	result.Command = line

	c := exec.Command("sh", "-c", line)
	c.Dir = target.Path
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := c.Run(); err != nil {
		return fmt.Errorf("opener failed (%s): %w", line, err)
	}
	result.Opened = true
	return emitResult(result, jsonFlag)
}

// openerTemplate resolves the opener: the per-call override, the environment,
// then the user config. Empty means none is configured.
func openerTemplate(with string) string {
	if s := strings.TrimSpace(with); s != "" {
		return s
	}
	if s := strings.TrimSpace(os.Getenv(EnvOpen)); s != "" {
		return s
	}
	cfg, err := config.LoadUserConfig()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cfg.OpenCommand)
}

// flagOpenAfter is the --open flag shared by the verbs that land you in a
// worktree: after switching (or printing the path), also open it.
var flagOpenAfter bool

// maybeOpenAfter honors --open for a worktree mp just moved to. Failures are
// non-fatal: the piece is there either way, and the path has been surfaced.
func maybeOpenAfter(ctx context.Context, workDir string) {
	if !flagOpenAfter || workDir == "" {
		return
	}
	target := opencmd.Target{Path: workDir, Piece: filepath.Base(workDir)}
	if _, handler := pieceHandlerForSwitch(); handler != nil {
		if status, err := handler.Status(ctx, workDir); err == nil {
			if status.InPiece {
				target.Piece = status.PieceName
			} else {
				target.Piece = "main"
			}
			if status.RepoRoot != "" {
				target.Project = projectNameFor(status.RepoRoot)
			}
		}
	}
	if err := openTarget(target, flagOpenWith, false); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
}
