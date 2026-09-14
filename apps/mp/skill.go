package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	skillcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/skill"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var skillCmd = &cobra.Command{
	Use:     "skill",
	Aliases: []string{"skills"},
	Short:   "Agent skill documents mp ships",
	Long: `Skill documents teach an agent this CLI. They are a portable format, so mp
writes the canonical copy to .agents/skills/<name>/SKILL.md and links
.claude/skills/<name> at it for agents that only read their own directory.

Use --user for a skill that is useful outside any one project; it writes under
your home directory instead of the repo.`,
	Args: cobra.NoArgs,
	RunE: runSkillList,
}

var skillListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List the skills mp ships",
	Args:    cobra.NoArgs,
	RunE:    runSkillList,
}

var skillShowCmd = &cobra.Command{
	Use:   "show [name]",
	Short: "Print one skill document",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runSkillShow,
}

var skillCreateCmd = &cobra.Command{
	Use:     "create [name]",
	Aliases: []string{"install", "new"},
	Short:   "Write a skill document",
	Long: `Write a skill document to .agents/skills/<name>/SKILL.md and link it from
.claude/skills/<name>. Re-running is how you refresh it after upgrading mp;
the result reports whether the file was created, updated, or already current.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSkillCreate,
}

var (
	flagSkillJSON   bool
	flagSkillUser   bool
	flagSkillSchema bool
)

func init() {
	for _, cmd := range []*cobra.Command{skillCmd, skillListCmd} {
		cmd.Flags().BoolVar(&flagSkillJSON, "json", false, "Output JSON even on a terminal")
	}
	skillCreateCmd.Flags().BoolVar(&flagSkillUser, "user", false, "Write under your home directory instead of the repo")
	skillCreateCmd.Flags().BoolVar(&flagSkillSchema, "schema", false, "Print an example input document and exit")
	skillCreateCmd.Flags().BoolVar(&flagSkillJSON, "json", false, "Output JSON even on a terminal")
	for _, cmd := range []*cobra.Command{skillShowCmd, skillCreateCmd} {
		cmd.ValidArgsFunction = completeSkillNames
	}
	skillCmd.AddCommand(skillListCmd, skillShowCmd, skillCreateCmd)
	rootCmd.AddCommand(skillCmd)
}

func newSkillHandler() *skillcmd.Handler {
	deps := core.NewDeps(adapters.NewOSFS(""), adapters.NewTextOutput(os.Stderr), adapters.NewOSExec(), http.DefaultClient, adapters.SetupNoopLoading())
	return skillcmd.NewHandler(deps)
}

func completeSkillNames(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	var names []string
	for _, s := range skillcmd.Catalog() {
		names = append(names, s.Name)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func runSkillList(_ *cobra.Command, _ []string) error {
	skills := skillcmd.Catalog()
	if !cli.IsTerminal() || !cli.IsStdoutTerminal() || flagSkillJSON {
		return cli.PrintJSON(map[string]any{"skills": skills})
	}
	w := tabwriter.NewWriter(os.Stderr, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "NAME\tDESCRIPTION")
	for _, s := range skills {
		_, _ = fmt.Fprintf(w, "%s\t%s\n", s.Name, truncate(s.Description, 80))
	}
	return w.Flush()
}

// truncate cuts on rune boundaries: descriptions are prose and contain
// multi-byte runes, so slicing bytes can emit a broken character.
func truncate(s string, n int) string {
	if n <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func runSkillShow(_ *cobra.Command, args []string) error {
	name := skillcmd.DefaultSkill
	if len(args) == 1 {
		name = args[0]
	}
	target, err := skillcmd.Get(name)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(target.Body)
	return err
}

func runSkillCreate(cmd *cobra.Command, args []string) error {
	if flagSkillSchema {
		schema, err := skillcmd.Schema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	var input skillcmd.Input
	if cli.HasStdinData() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &input); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}
	}
	if len(args) == 1 {
		input.Name = args[0]
	}
	if cmd.Flags().Changed("user") {
		input.User = flagSkillUser
	}

	root, err := skillRoot(cmd, input.User)
	if err != nil {
		return err
	}

	result, err := newSkillHandler().Install(root, input)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}

// skillRoot resolves where a skill is written. Project scope anchors on the git
// repo root — the same anchor as 'mp integration install' — so the location
// does not depend on which subdirectory you happen to be standing in.
func skillRoot(cmd *cobra.Command, user bool) (string, error) {
	if user {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to resolve home directory: %w", err)
		}
		return home, nil
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := adapters.NewGit(adapters.NewOSExec()).RepoRoot(cmd.Context(), wd)
	if err != nil {
		return "", fmt.Errorf("not in a git repository (use --user to write a skill outside a project): %w", err)
	}
	return root, nil
}
