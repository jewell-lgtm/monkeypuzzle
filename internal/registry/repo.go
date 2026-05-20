package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// ConfigPath returns the location of the project config within repoRoot,
// honoring any relocation of the monkeypuzzle directory.
func ConfigPath(repoRoot string) string {
	return projectdir.ConfigFilePath(repoRoot)
}

// projectConfig is the subset of monkeypuzzle.json we need here.
type projectConfig struct {
	Project struct {
		Name string `json:"name"`
	} `json:"project"`
}

// ResolveRepoRoot returns the absolute, symlink-resolved git repository root
// containing dir. It errors if dir is not inside a git repository.
func ResolveRepoRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	cmd := exec.Command("git", "-C", abs, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git repository", dir)
	}
	root := strings.TrimSpace(string(out))
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root, nil
}

// ProjectName reads the project name from repoRoot's monkeypuzzle.json,
// falling back to the base name of repoRoot when the config is absent or has
// no name. The bool result reports whether a monkeypuzzle config was found.
func ProjectName(repoRoot string) (string, bool) {
	data, err := os.ReadFile(ConfigPath(repoRoot))
	if err != nil {
		return filepath.Base(repoRoot), false
	}
	var cfg projectConfig
	if err := json.Unmarshal(data, &cfg); err != nil || strings.TrimSpace(cfg.Project.Name) == "" {
		return filepath.Base(repoRoot), true
	}
	return cfg.Project.Name, true
}

// IsProject reports whether repoRoot contains a monkeypuzzle config file.
func IsProject(repoRoot string) bool {
	_, err := os.Stat(ConfigPath(repoRoot))
	return err == nil
}
