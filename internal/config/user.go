// Package config handles user-level configuration for monkeypuzzle.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

const (
	configFileName = "config.json"
)

// UserConfig represents user-level monkeypuzzle configuration.
type UserConfig struct {
	Multiplexer string `json:"multiplexer,omitempty"` // "tmux", "zellij", or "none"
}

// DefaultUserConfig returns config with default values.
func DefaultUserConfig() UserConfig {
	return UserConfig{
		Multiplexer: "none",
	}
}

// LoadUserConfig loads user config from ~/.config/monkeypuzzle/config.json.
// Returns default config if file doesn't exist.
func LoadUserConfig() (UserConfig, error) {
	configDir, err := paths.ConfigDir()
	if err != nil {
		return DefaultUserConfig(), nil
	}

	configPath := filepath.Join(configDir, configFileName)
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultUserConfig(), nil
		}
		return UserConfig{}, err
	}

	var cfg UserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return UserConfig{}, err
	}

	// Apply defaults for missing fields
	if cfg.Multiplexer == "" {
		cfg.Multiplexer = "none"
	}

	return cfg, nil
}

// UserConfigPath returns the path where the user config would be stored.
// The file may not exist; use UserConfigExists to check.
func UserConfigPath() (string, error) {
	configDir, err := paths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, configFileName), nil
}

// UserConfigExists reports whether the user config file is present on disk.
func UserConfigExists() bool {
	path, err := UserConfigPath()
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// SaveUserConfig writes user config to ~/.config/monkeypuzzle/config.json.
func SaveUserConfig(cfg UserConfig) error {
	configDir, err := paths.ConfigDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	configPath := filepath.Join(configDir, configFileName)
	return os.WriteFile(configPath, data, 0644)
}
