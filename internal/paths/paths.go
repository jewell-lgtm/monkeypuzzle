// Package paths provides centralized path handling using go-app-paths (GAP).
// Uses XDG spec on Unix, appropriate platform dirs on macOS/Windows.
package paths

import (
	"crypto/sha256"
	"encoding/hex"
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

// RepoIdentifier generates a unique identifier for a repository based on its absolute root path.
// Returns the first 12 characters of the SHA256 hash of the repo root path.
func RepoIdentifier(repoRoot string) (string, error) {
	// Get absolute path to ensure consistency
	absPath, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}

	// Resolve symlinks to ensure consistent hashing across different path representations
	// (e.g., /var vs /private/var on macOS)
	evalPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// If symlink resolution fails (e.g., path doesn't exist), use the absolute path
		evalPath = absPath
	}

	// Compute SHA256 hash
	hash := sha256.Sum256([]byte(evalPath))
	hexHash := hex.EncodeToString(hash[:])

	// Return first 12 characters
	return hexHash[:12], nil
}

// PiecesDir returns the directory for storing piece worktrees.
// If repoRoot is empty, returns the global pieces directory (for backward compatibility).
// If repoRoot is provided, returns a repo-scoped directory: {DataDir}/pieces/{repoIdentifier}/
func PiecesDir(repoRoot string) (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}

	// If no repo root provided, return global directory for backward compatibility
	if repoRoot == "" {
		return filepath.Join(dataDir, "pieces"), nil
	}

	// Generate repo identifier
	repoID, err := RepoIdentifier(repoRoot)
	if err != nil {
		return "", err
	}

	// Return repo-scoped directory
	return filepath.Join(dataDir, "pieces", repoID), nil
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
