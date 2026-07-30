package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	integrationcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/integration"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

var integrationCmd = &cobra.Command{
	Use:   "integration",
	Short: "Install agent integrations",
}

var integrationInstallCmd = &cobra.Command{
	Use:       "install <tool>",
	Short:     "Install an agent-status integration (claude)",
	Long: `Wire an agent's own hook system to 'mp agent report' so its status shows up
per piece. For claude, merges hooks into .claude/settings.json at the current
repo root — run it in the main repo and commit so every piece worktree gets it.`,
	Args:      cobra.ExactArgs(1),
	ValidArgs: []string{"claude"},
	RunE:      runIntegrationInstall,
}

func init() {
	integrationCmd.AddCommand(integrationInstallCmd)
	rootCmd.AddCommand(integrationCmd)
}

func runIntegrationInstall(cmd *cobra.Command, args []string) error {
	if args[0] != "claude" {
		return fmt.Errorf("unknown tool %q (supported: claude)", args[0])
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	git := adapters.NewGit(adapters.NewOSExec())
	repoRoot, err := git.RepoRoot(cmd.Context(), wd)
	if err != nil {
		return fmt.Errorf("not in a git repository: %w", err)
	}

	result, err := integrationcmd.NewHandler(newAgentDeps()).InstallClaude(repoRoot)
	if err != nil {
		return err
	}
	return cli.PrintJSON(result)
}
