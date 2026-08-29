package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
	"github.com/jewell-lgtm/monkeypuzzle/internal/tui/configwizard"
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// errWizardCancelled signals that the user dismissed the first-run wizard.
var errWizardCancelled = errors.New("setup cancelled")

// ensureUserConfig runs before any non-exempt command. If the user config file
// is missing it either launches an interactive wizard (when stdin is a TTY) or
// fails with a helpful error message.
func ensureUserConfig(cmd *cobra.Command) error {
	if commandSkipsConfigCheck(cmd) {
		return nil
	}
	if config.UserConfigExists() {
		return nil
	}

	// Errors from here on are not usage errors — don't print cobra's banner.
	cmd.SilenceUsage = true

	if !cli.IsTerminal() {
		return notConfiguredError()
	}

	chosen, err := runConfigWizard()
	if err != nil {
		if errors.Is(err, errWizardCancelled) {
			return err
		}
		// TUI couldn't start (e.g., no /dev/tty in this environment) — fall
		// back to the non-interactive guidance.
		return notConfiguredError()
	}

	cfg := config.DefaultUserConfig()
	cfg.Multiplexer = chosen
	if err := config.SaveUserConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	path, _ := config.UserConfigPath()
	fmt.Fprintf(os.Stderr, "Saved %s (multiplexer = %s)\n\n", path, chosen)
	return nil
}

func notConfiguredError() error {
	return fmt.Errorf("monkeypuzzle is not configured yet. Run `mp config set multiplexer <%s>` to set it up, "+
		"or run any `mp` command in an interactive terminal to use the setup wizard",
		strings.Join(validMultiplexerValues, "|"))
}

// commandSkipsConfigCheck returns true for commands that must work without a
// populated user config: the config command itself, help/completion plumbing,
// and any invocation that's just emitting an example input document.
func commandSkipsConfigCheck(cmd *cobra.Command) bool {
	if cmd == nil {
		return true
	}
	switch cmd.Name() {
	case "help", "completion", "__complete", "__completeNoDesc":
		return true
	}
	for c := cmd; c != nil; c = c.Parent() {
		if c.Name() == "config" {
			return true
		}
	}
	if schema := cmd.Flags().Lookup("schema"); schema != nil && schema.Changed {
		return true
	}
	return false
}

func runConfigWizard() (string, error) {
	m, err := tea.NewProgram(configwizard.New()).Run()
	if err != nil {
		return "", fmt.Errorf("wizard failed: %w", err)
	}
	model := m.(configwizard.Model)
	if model.Cancelled || model.Chosen == "" {
		return "", errWizardCancelled
	}
	return model.Chosen, nil
}
