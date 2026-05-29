package mp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	projectcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/project"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var (
	flagProjectAddPath   string
	flagProjectAddSchema bool
	flagProjectRmTarget  string
	flagProjectRmSchema  bool
	flagProjectListJSON  bool
)

var projectCmd = &cobra.Command{
	Use:     "project",
	Aliases: []string{"projects", "proj"},
	Short:   "Manage the global list of monkeypuzzle projects",
	Long: `Manage the global registry of monkeypuzzle projects.

A "project" is any git repository that has been initialised with 'mp init'.
Registering projects lets mp list pieces and issues across all of them and jump
between their tmux sessions.`,
}

var projectAddCmd = &cobra.Command{
	Use:   "add [path]",
	Short: "Register a project (defaults to the current directory)",
	Long: `Register the monkeypuzzle project containing the given path (or the
current directory). 'mp init' registers projects automatically.

Modes:
  Argument/flag:  mp project add [path] | mp project add --path PATH
  Stdin JSON:     echo '{"path":"/repo"}' | mp project add
  --schema:       Output expected JSON format`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectAdd,
}

var projectRemoveCmd = &cobra.Command{
	Use:     "remove <name|path>",
	Aliases: []string{"rm"},
	Short:   "Unregister a project (does not touch the repository)",
	Long: `Remove a project from the global registry. The repository on disk is
left untouched.

Modes:
  Argument/flag:  mp project rm <name|path> | mp project rm --target NAME
  Stdin JSON:     echo '{"target":"my-project"}' | mp project rm
  --schema:       Output expected JSON format`,
	Args: cobra.MaximumNArgs(1),
	RunE: runProjectRemove,
}

var projectListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls", "status"},
	Short:   "List registered projects",
	Long: `List all registered monkeypuzzle projects with best-effort live state
(current branch, number of pieces, number of open issues).

By default a human-readable table is printed; use --json for machine output.
When stdout is not a terminal, JSON is printed automatically.`,
	RunE: runProjectList,
}

func init() {
	projectAddCmd.Flags().StringVar(&flagProjectAddPath, "path", "", "Path to the project repository")
	projectAddCmd.Flags().BoolVar(&flagProjectAddSchema, "schema", false, "Output JSON schema and exit")

	projectRemoveCmd.Flags().StringVar(&flagProjectRmTarget, "target", "", "Name or path of the project to remove")
	projectRemoveCmd.Flags().BoolVar(&flagProjectRmSchema, "schema", false, "Output JSON schema and exit")

	projectListCmd.Flags().BoolVar(&flagProjectListJSON, "json", false, "Output JSON instead of a table")

	projectCmd.AddCommand(projectAddCmd)
	projectCmd.AddCommand(projectRemoveCmd)
	projectCmd.AddCommand(projectListCmd)
	rootCmd.AddCommand(projectCmd)
}

// --- add ---

type projectAddInput struct {
	Path string `json:"path"`
}

func runProjectAdd(cmd *cobra.Command, args []string) error {
	if flagProjectAddSchema {
		return cli.PrintJSON(projectAddInput{Path: ""})
	}

	path, err := resolveProjectAddPath(args)
	if err != nil {
		return err
	}

	p, added, err := projectcmd.Add(path)
	if err != nil {
		return err
	}
	verb := "Already registered"
	if added {
		verb = "Registered"
	}
	fmt.Fprintf(os.Stderr, "%s %s -> %s\n", verb, p.Name, p.Path)
	return cli.PrintJSON(p)
}

func resolveProjectAddPath(args []string) (string, error) {
	switch {
	case len(args) == 1:
		return args[0], nil
	case flagProjectAddPath != "":
		return flagProjectAddPath, nil
	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read stdin: %w", err)
		}
		var in projectAddInput
		if err := json.Unmarshal(data, &in); err != nil {
			return "", fmt.Errorf("invalid JSON: %w", err)
		}
		if in.Path != "" {
			return in.Path, nil
		}
		fallthrough
	default:
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to get working directory: %w", err)
		}
		return wd, nil
	}
}

// --- remove ---

type projectRemoveInput struct {
	Target string `json:"target"`
}

func runProjectRemove(cmd *cobra.Command, args []string) error {
	if flagProjectRmSchema {
		return cli.PrintJSON(projectRemoveInput{Target: ""})
	}

	target, err := resolveProjectRemoveTarget(args)
	if err != nil {
		return err
	}
	if target == "" {
		return fmt.Errorf("no project specified; pass a name or path")
	}

	p, err := projectcmd.Remove(target)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Unregistered %s (%s)\n", p.Name, p.Path)
	return cli.PrintJSON(p)
}

func resolveProjectRemoveTarget(args []string) (string, error) {
	switch {
	case len(args) == 1:
		return args[0], nil
	case flagProjectRmTarget != "":
		return flagProjectRmTarget, nil
	case cli.HasStdinData():
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read stdin: %w", err)
		}
		var in projectRemoveInput
		if err := json.Unmarshal(data, &in); err != nil {
			return "", fmt.Errorf("invalid JSON: %w", err)
		}
		return in.Target, nil
	default:
		return "", nil
	}
}

// --- list ---

func runProjectList(cmd *cobra.Command, args []string) error {
	projects, err := projectcmd.List()
	if err != nil {
		return err
	}

	if flagProjectListJSON || !cli.IsStdoutTerminal() {
		return cli.PrintJSON(projects)
	}

	if len(projects) == 0 {
		fmt.Println("No registered projects. Run `mp init` in a repo, or `mp project add <path>`.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tBRANCH\tPIECES\tOPEN ISSUES\tPATH")
	for _, p := range projects {
		branch := p.Branch
		path := p.Path
		switch {
		case !p.Exists:
			branch = "(missing)"
		case !p.IsProject:
			branch = "(no .monkeypuzzle)"
		case branch == "":
			branch = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\t%s\n", p.Name, branch, p.PieceCount, p.OpenIssues, path)
	}
	return w.Flush()
}
