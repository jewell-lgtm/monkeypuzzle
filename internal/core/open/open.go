// Package open resolves how to hand a piece's worktree to an external tool —
// an editor, an IDE, a new terminal window. It is the no-multiplexer answer to
// "take me to this piece": instead of attaching a session, mp opens the
// worktree wherever you actually work.
package open

import (
	"fmt"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Target is what an opener command is expanded against.
type Target struct {
	Path    string
	Piece   string
	Project string
	Branch  string
}

// Placeholders an opener command may use. A command with none of them gets the
// path appended, so a bare "code" behaves like "code {path}".
const (
	phPath    = "{path}"
	phPiece   = "{piece}"
	phProject = "{project}"
	phBranch  = "{branch}"
)

var placeholders = []string{phPath, phPiece, phProject, phBranch}

// Command expands an opener template against a target, producing a shell
// command line. Every substituted value is shell-quoted, so a path with spaces
// and a branch name with punctuation cannot break out of their word.
func Command(template string, t Target) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", fmt.Errorf("no opener configured")
	}
	if t.Path == "" {
		return "", fmt.Errorf("no path to open")
	}

	used := false
	for _, ph := range placeholders {
		if strings.Contains(template, ph) {
			used = true
			break
		}
	}
	if !used {
		// "code" / "open -a Zed" — the path is the implied final argument.
		return template + " " + cli.ShQuote(t.Path), nil
	}

	r := strings.NewReplacer(
		phPath, cli.ShQuote(t.Path),
		phPiece, cli.ShQuote(t.Piece),
		phProject, cli.ShQuote(t.Project),
		phBranch, cli.ShQuote(t.Branch),
	)
	return r.Replace(template), nil
}

// Recipes are the opener commands worth suggesting when none is configured.
var Recipes = []string{
	`mp config set open_command 'code {path}'           # VS Code`,
	`mp config set open_command 'cursor {path}'         # Cursor`,
	`mp config set open_command 'zed {path}'            # Zed`,
	`mp config set open_command 'idea {path}'           # IntelliJ`,
	`mp config set open_command 'open -a iTerm {path}'  # a new iTerm window`,
	`mp config set open_command 'wezterm start --cwd {path}'`,
}
