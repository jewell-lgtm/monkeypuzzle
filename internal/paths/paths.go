// Package paths provides centralized path handling using go-app-paths (GAP).
// Uses XDG spec on Unix, appropriate platform dirs on macOS/Windows.
package paths

import (
	"os"

	gap "github.com/muesli/go-app-paths"
)

const appName = "monkeypuzzle"

// EnvDataDir, when set, overrides the user data directory. Useful for tests and
// for isolating the global project registry.
const EnvDataDir = "MP_DATA_DIR"

// EnvConfigDir, when set, overrides the user config directory. Useful for tests
// and for isolating the project-dir mapping.
const EnvConfigDir = "MP_CONFIG_DIR"

var scope = gap.NewScope(gap.User, appName)

// overrideDataDir allows tests to override the data directory
var overrideDataDir string

// SetDataDir overrides the data directory (for testing).
func SetDataDir(dir string) {
	overrideDataDir = dir
}

// ResetDataDir clears the override (for testing).
func ResetDataDir() {
	overrideDataDir = ""
}

// DataDir returns the user data directory for monkeypuzzle.
// e.g., ~/.local/share/monkeypuzzle on Linux, ~/Library/Application Support/monkeypuzzle on macOS
func DataDir() (string, error) {
	if overrideDataDir != "" {
		return overrideDataDir, nil
	}
	if env := os.Getenv(EnvDataDir); env != "" {
		return env, nil
	}
	dirs, err := scope.DataDirs()
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", err
	}
	return dirs[0], nil
}

// ConfigDir returns the user config directory for monkeypuzzle.
// e.g., ~/.config/monkeypuzzle on Linux, ~/Library/Application Support/monkeypuzzle on macOS
func ConfigDir() (string, error) {
	if env := os.Getenv(EnvConfigDir); env != "" {
		return env, nil
	}
	dirs, err := scope.ConfigDirs()
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", err
	}
	return dirs[0], nil
}

// CacheDir returns the user cache directory for monkeypuzzle.
// e.g., ~/.cache/monkeypuzzle on Linux, ~/Library/Caches/monkeypuzzle on macOS
func CacheDir() (string, error) {
	return scope.CacheDir()
}
