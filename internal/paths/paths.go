// Package paths provides centralized path handling using go-app-paths (GAP).
// Uses XDG spec on Unix, appropriate platform dirs on macOS/Windows.
package paths

import (
	"fmt"
	"path/filepath"

	gap "github.com/muesli/go-app-paths"
)

const appName = "monkeypuzzle"

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
	dirs, err := scope.DataDirs()
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", err
	}
	return dirs[0], nil
}

// PiecesDir returns the directory for storing piece worktrees.
// Returns {repoRoot}/.monkeypuzzle/pieces/
func PiecesDir(repoRoot string) (string, error) {
	if repoRoot == "" {
		return "", fmt.Errorf("repoRoot is required")
	}
	return filepath.Join(repoRoot, ".monkeypuzzle", "pieces"), nil
}

// ConfigDir returns the user config directory for monkeypuzzle.
// e.g., ~/.config/monkeypuzzle on Linux, ~/Library/Application Support/monkeypuzzle on macOS
func ConfigDir() (string, error) {
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
