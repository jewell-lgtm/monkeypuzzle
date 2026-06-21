package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/jewell-lgtm/monkeypuzzle/internal/config"
)

// validMultiplexerValues lists the multiplexer options mp recognises.
// tmux is the only real multiplexer; "none" is the no-op (print-the-path) fallback.
// Maybe one day we'll support zellij / wezterm / kitty / others — drop the adapter
// in internal/adapters, wire it in NewMultiplexer, and add the name here.
var validMultiplexerValues = []string{"tmux", "none"}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage user configuration",
	Long:  `Get and set user-level monkeypuzzle configuration.`,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE:  runConfigGet,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Long: `Set a configuration value.

Available keys:
  multiplexer  Terminal multiplexer to use (tmux, none)`,
	Args: cobra.ExactArgs(2),
	RunE: runConfigSet,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)

	// Register completion for config keys
	_ = configGetCmd.RegisterFlagCompletionFunc("", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return []string{"multiplexer"}, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	})
}

func runConfigGet(cmd *cobra.Command, args []string) error {
	key := args[0]

	cfg, err := config.LoadUserConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	var value string
	switch key {
	case "multiplexer":
		value = cfg.Multiplexer
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	result := map[string]string{
		"key":   key,
		"value": value,
	}
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

func runConfigSet(cmd *cobra.Command, args []string) error {
	key := args[0]
	value := args[1]

	cfg, err := config.LoadUserConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	switch key {
	case "multiplexer":
		if !isValidMultiplexer(value) {
			return fmt.Errorf("invalid multiplexer value: %s (valid: tmux, none)", value)
		}
		cfg.Multiplexer = value
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}

	if err := config.SaveUserConfig(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	result := map[string]string{
		"key":   key,
		"value": value,
	}
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(jsonData))
	return nil
}

func isValidMultiplexer(value string) bool {
	for _, v := range validMultiplexerValues {
		if v == value {
			return true
		}
	}
	return false
}
