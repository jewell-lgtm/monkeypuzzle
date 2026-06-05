// Package project implements the `mp project` commands: managing the global
// registry of monkeypuzzle projects (see internal/registry).
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
	"github.com/jewell-lgtm/monkeypuzzle/internal/registry"
)

// Info is a registered project enriched with best-effort live state.
type Info struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Exists     bool   `json:"exists"`     // repo root still present on disk
	IsProject  bool   `json:"is_project"` // .monkeypuzzle/monkeypuzzle.json present
	Branch     string `json:"branch,omitempty"`
	PieceCount int    `json:"piece_count"`
	OpenIssues int    `json:"open_issues"`
}

// Add registers the project containing dir. It returns the stored project and
// whether it was newly added.
func Add(dir string) (registry.Project, bool, error) {
	root, err := registry.ResolveRepoRoot(dir)
	if err != nil {
		return registry.Project{}, false, err
	}
	if !registry.IsProject(root) {
		return registry.Project{}, false, fmt.Errorf("%s is not a monkeypuzzle project (run `mp init` there first)", root)
	}
	name, _ := registry.ProjectName(root)

	reg, err := registry.Load()
	if err != nil {
		return registry.Project{}, false, err
	}
	p, added := reg.Upsert(root, name)
	if err := reg.Save(); err != nil {
		return registry.Project{}, false, err
	}
	return p, added, nil
}

// Remove unregisters the project matching nameOrPath. The repository itself is
// untouched.
func Remove(nameOrPath string) (registry.Project, error) {
	reg, err := registry.Load()
	if err != nil {
		return registry.Project{}, err
	}
	p, ok := reg.Remove(nameOrPath)
	if !ok {
		return registry.Project{}, fmt.Errorf("no registered project matching %q", nameOrPath)
	}
	if err := reg.Save(); err != nil {
		return registry.Project{}, err
	}
	return p, nil
}

// List returns all registered projects with best-effort enrichment.
func List() ([]Info, error) {
	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}
	out := make([]Info, 0, len(reg.Projects))
	for _, p := range reg.Projects {
		out = append(out, enrich(p))
	}
	return out, nil
}

// Get returns the enriched Info for the registered project matching nameOrPath
// (by symlink-resolved path or by name). The bool is false when nothing matches.
func Get(nameOrPath string) (Info, bool, error) {
	reg, err := registry.Load()
	if err != nil {
		return Info{}, false, err
	}
	p, ok := reg.Find(nameOrPath)
	if !ok {
		return Info{}, false, nil
	}
	return enrich(p), true, nil
}

func enrich(p registry.Project) Info {
	info := Info{Name: p.Name, Path: p.Path}
	fi, err := os.Stat(p.Path)
	if err != nil || !fi.IsDir() {
		return info
	}
	info.Exists = true
	info.IsProject = registry.IsProject(p.Path)
	if b, err := currentBranch(p.Path); err == nil {
		info.Branch = b
	}
	info.PieceCount = pieceCount(p.Path)
	info.OpenIssues = openIssueCount(p.Path)
	return info
}

func currentBranch(repoRoot string) (string, error) {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--abbrev-ref", "HEAD").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func pieceCount(repoRoot string) int {
	piecesDir, err := projectdir.PiecesDir(repoRoot)
	if err != nil {
		return 0
	}
	entries, err := os.ReadDir(piecesDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() {
			n++
		}
	}
	return n
}

// openIssueCount counts markdown issue files without a "done" status. It only
// supports the markdown provider; other providers report 0.
func openIssueCount(repoRoot string) int {
	dir := issuesDir(repoRoot)
	if dir == "" {
		return 0
	}
	entries, err := os.ReadDir(filepath.Join(repoRoot, dir))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoRoot, dir, e.Name()))
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "status: done") || strings.Contains(string(data), "status: \"done\"") {
			continue
		}
		n++
	}
	return n
}

// issuesDir returns the configured issues directory for a markdown-provider
// project, or "" if not applicable.
func issuesDir(repoRoot string) string {
	data, err := os.ReadFile(registry.ConfigPath(repoRoot))
	if err != nil {
		return ""
	}
	var cfg struct {
		Issues struct {
			Provider string            `json:"provider"`
			Config   map[string]string `json:"config"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil || cfg.Issues.Provider != "markdown" {
		return ""
	}
	if d := cfg.Issues.Config["directory"]; d != "" {
		return d
	}
	return "issues"
}
