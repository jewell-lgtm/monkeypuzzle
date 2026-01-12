package mp

import (
	"fmt"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	claudecmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/claude"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var claudeCmd = &cobra.Command{
	Use:   "claude",
	Short: "Claude Code integration",
	Long:  `Commands for Claude Code integration.`,
}

var claudeSkillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Create Claude Code skill for mp CLI",
	Long: `Create or regenerate the Claude Code skill file for the mp CLI.

Creates .claude/skills/managing-monkeypuzzle/SKILL.md with documentation
for using mp commands programmatically.`,
	RunE: runClaudeSkill,
}

var flagClaudeSkillSchema bool

func init() {
	claudeSkillCmd.Flags().BoolVar(&flagClaudeSkillSchema, "schema", false, "Output JSON schema and exit")
	claudeCmd.AddCommand(claudeSkillCmd)
	rootCmd.AddCommand(claudeCmd)
}

func runClaudeSkill(cmd *cobra.Command, args []string) error {
	if flagClaudeSkillSchema {
		schema, err := claudecmd.Schema()
		if err != nil {
			return err
		}
		fmt.Println(string(schema))
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	deps := core.NewDeps(
		adapters.NewOSFS(""),
		adapters.NewTextOutput(os.Stderr),
		adapters.NewOSExec(),
		http.DefaultClient,
		adapters.SetupCLILoading(os.Stderr),
	)
	handler := claudecmd.NewHandler(deps)

	result, err := handler.CreateSkill(wd)
	if err != nil {
		return err
	}

	return cli.PrintJSON(result)
}
