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
	"github.com/jewell-lgtm/monkeypuzzle/pkg/cli"
)

// Info is a registered project enriched with best-effort live state.
type Info struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Host       string `json:"host,omitempty"` // ssh host where the repo lives; empty = local
	Hidden     bool   `json:"hidden,omitempty"`
	LinkedFrom string `json:"linked_from,omitempty"`
	Exists     bool   `json:"exists"`     // repo root still present on disk
	IsProject  bool   `json:"is_project"` // .monkeypuzzle/monkeypuzzle.json present
	Branch     string `json:"branch,omitempty"`
	PieceCount int    `json:"piece_count"`
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

// AddRemote registers a project living on an ssh host. The directory —
// absolute, or relative to the ssh login home (a quoted `~` never expands) —
// is resolved to an absolute path on the host at add time, and must already
// be an mp project there. The name is read from the remote monkeypuzzle.json.
func AddRemote(host, dir string) (registry.Project, bool, error) {
	if err := cli.ValidSSHDest(host); err != nil {
		return registry.Project{}, false, err
	}
	probe := `root=$(readlink -f ` + cli.ShQuote(dir) + `) && test -f "$root/.monkeypuzzle/monkeypuzzle.json" && printf %s "$root"`
	root, err := sshOutput(host, probe)
	if err != nil {
		return registry.Project{}, false, fmt.Errorf("%s:%s is not a monkeypuzzle project (run `mp init` there first): %w", host, dir, err)
	}
	root = strings.TrimSpace(root)

	name := filepath.Base(root)
	if data, err := sshOutput(host, "cat "+cli.ShQuote(root+"/.monkeypuzzle/monkeypuzzle.json")); err == nil {
		var cfg struct {
			Project struct {
				Name string `json:"name"`
			} `json:"project"`
		}
		if json.Unmarshal([]byte(data), &cfg) == nil && strings.TrimSpace(cfg.Project.Name) != "" {
			name = cfg.Project.Name
		}
	}

	reg, err := registry.Load()
	if err != nil {
		return registry.Project{}, false, err
	}
	p, added := reg.UpsertRemote(host, root, name)
	if err := reg.Save(); err != nil {
		return registry.Project{}, false, err
	}
	return p, added, nil
}

// sshOutput runs a shell command on host and returns its stdout. The command
// is wrapped in `sh -c` so POSIX syntax survives fish/csh login shells.
func sshOutput(host, command string) (string, error) {
	out, err := exec.Command("ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", "--", host, "sh -c "+cli.ShQuote(command)).Output()
	return string(out), err
}

// Remove unregisters the project matching nameOrPath. The repository itself is
// untouched.
func Remove(nameOrPath string) (registry.Project, error) {
	reg, err := registry.Load()
	if err != nil {
		return registry.Project{}, err
	}
	p, err := reg.Remove(nameOrPath)
	if err != nil {
		return registry.Project{}, err
	}
	if err := reg.Save(); err != nil {
		return registry.Project{}, err
	}
	return p, nil
}

// PruneStale removes registry entries whose repository directory no longer
// exists on disk (a "deleted project"). It returns the entries that were
// removed. In dryRun mode the registry is left untouched and the stale entries
// are reported without being removed. Entries whose path cannot be stat'd for
// reasons other than non-existence (e.g. permissions) are kept, to avoid
// pruning a project that is merely temporarily unreachable.
func PruneStale(dryRun bool) ([]registry.Project, error) {
	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}
	var removed, kept []registry.Project
	for _, p := range reg.Projects {
		// Remote projects can't be stat'd locally; never prune them here.
		if p.Host == "" && pathMissing(p.Path) {
			removed = append(removed, p)
			continue
		}
		kept = append(kept, p)
	}
	if len(removed) == 0 {
		return nil, nil
	}
	if !dryRun {
		reg.Projects = kept
		if err := reg.Save(); err != nil {
			return nil, err
		}
	}
	return removed, nil
}

// pathMissing reports whether path is a deleted project root: it does not exist,
// or exists but is no longer a directory. Other stat errors return false so the
// entry is preserved.
func pathMissing(path string) bool {
	fi, err := os.Stat(path)
	if err != nil {
		return os.IsNotExist(err)
	}
	return !fi.IsDir()
}

// List returns the visible registered projects with best-effort enrichment.
// Hidden bookkeeping rows (see registry.Project.Hidden) are skipped; ListAll
// includes them.
func List() ([]Info, error) {
	return list(false)
}

// ListAll is List including hidden rows.
func ListAll() ([]Info, error) {
	return list(true)
}

func list(all bool) ([]Info, error) {
	reg, err := registry.Load()
	if err != nil {
		return nil, err
	}
	projects := reg.Projects
	if !all {
		projects = reg.Visible()
	}
	out := make([]Info, 0, len(projects))
	for _, p := range projects {
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

// Describe returns the enriched Info for a project at the given repo root,
// whether or not it is in the registry. The name is taken from the directory
// (registry.ProjectName), so it works for an init'd repo that hasn't been
// registered yet. Callers should gate this on registry.IsProject(root).
func Describe(root string) Info {
	name, _ := registry.ProjectName(root)
	return enrich(registry.Project{Name: name, Path: root})
}

func enrich(p registry.Project) Info {
	info := Info{Name: p.Name, Path: p.Path, Host: p.Host, Hidden: p.Hidden, LinkedFrom: p.LinkedFrom}
	if p.Host != "" {
		// Validated at add time; live state would need an ssh round-trip per
		// project, which is too slow for list — proxy into it for detail.
		info.Exists = true
		info.IsProject = true
		return info
	}
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
