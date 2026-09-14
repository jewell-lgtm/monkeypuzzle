package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	skillcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/skill"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// The Claude-specific spelling predates the portable skill format. It stays as
// a working alias because it shipped, but the surface to use is `mp skill`.
var claudeCmd = &cobra.Command{
	Use:    "claude",
	Short:  "Deprecated: use 'mp skill'",
	Hidden: true,
}

var claudeSkillCmd = &cobra.Command{
	Use:    "skill",
	Short:  "Deprecated: use 'mp skill create'",
	Args:   cobra.NoArgs,
	Hidden: true,
	RunE:   runClaudeSkill,
}

var flagClaudeSkillSchema bool

func init() {
	claudeSkillCmd.Flags().BoolVar(&flagClaudeSkillSchema, "schema", false, "Print an example input document and exit")
	claudeCmd.AddCommand(claudeSkillCmd)
	rootCmd.AddCommand(claudeCmd)
}

func runClaudeSkill(cmd *cobra.Command, _ []string) error {
	if flagClaudeSkillSchema {
		schema, err := skillcmd.Schema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	fmt.Fprintln(os.Stderr, "mp claude skill is deprecated; use 'mp skill create'")

	root, err := skillRoot(cmd, false)
	if err != nil {
		return err
	}
	result, err := newSkillHandler().Install(root, skillcmd.Input{})
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}
