// Package registry maintains the global list of monkeypuzzle projects known to
// this user. It is stored as JSON in the user data directory and is independent
// of any single repository.
package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

const (
	registryFileName = "projects.json"
	currentVersion   = "1"
)

// Project is a single registered monkeypuzzle project.
type Project struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`           // absolute, symlink-resolved repo root (on Host when set)
	Host    string    `json:"host,omitempty"` // ssh host where the repo lives; empty = this machine
	AddedAt time.Time `json:"added_at"`
	// Hidden rows are bookkeeping, not user-registered projects: the clone
	// `mp create --remote` keeps on a box for a local project. Skipped by
	// `mp project list` (unless --all) and the dashboard; still addressable
	// by --project and still probed by `mp remote doctor`.
	Hidden bool `json:"hidden,omitempty"`
	// LinkedFrom is the controller-side repo root a hidden row was placed
	// from, so the row can be traced back to its project.
	LinkedFrom string `json:"linked_from,omitempty"`
}

// Registry is the on-disk structure stored at {DataDir}/projects.json.
type Registry struct {
	Version  string    `json:"version"`
	Projects []Project `json:"projects"`
}

// Path returns the location of the registry file.
func Path() (string, error) {
	dir, err := paths.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, registryFileName), nil
}

// Load reads the registry. A missing file yields an empty registry.
func Load() (Registry, error) {
	p, err := Path()
	if err != nil {
		return Registry{Version: currentVersion}, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Registry{Version: currentVersion}, nil
		}
		return Registry{}, err
	}
	var r Registry
	if err := json.Unmarshal(data, &r); err != nil {
		return Registry{}, err
	}
	if r.Version == "" {
		r.Version = currentVersion
	}
	return r, nil
}

// Save writes the registry, creating the data directory if needed.
func (r Registry) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if r.Version == "" {
		r.Version = currentVersion
	}
	sort.Slice(r.Projects, func(i, j int) bool {
		a, b := r.Projects[i], r.Projects[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.Path < b.Path
	})
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// indexByPath returns the index of a project with the given path, or -1.
func (r Registry) indexByPath(path string) int {
	for i, p := range r.Projects {
		if p.Path == path {
			return i
		}
	}
	return -1
}

// indexBy returns the index of a project with the given (host, path), or -1.
// The same path on different hosts is two different projects.
func (r Registry) indexBy(host, path string) int {
	for i, p := range r.Projects {
		if p.Host == host && p.Path == path {
			return i
		}
	}
	return -1
}

// Upsert adds a local project (deduped by path) or updates its name. Returns
// the stored Project and whether it was newly added.
func (r *Registry) Upsert(path, name string) (Project, bool) {
	return r.upsert("", path, name)
}

// UpsertRemote is Upsert for a project living on an ssh host. An explicit
// registration of a row that was only hidden bookkeeping promotes it to a
// visible project.
func (r *Registry) UpsertRemote(host, path, name string) (Project, bool) {
	p, added := r.upsert(host, path, name)
	if i := r.indexBy(host, path); i >= 0 && r.Projects[i].Hidden {
		r.Projects[i].Hidden = false
		p = r.Projects[i]
	}
	return p, added
}

// UpsertHidden records the box-side clone a placed piece lives in without
// surfacing it as a project of its own. A row the user already registered
// stays visible; linkedFrom is recorded either way.
func (r *Registry) UpsertHidden(host, path, name, linkedFrom string) (Project, bool) {
	_, added := r.upsert(host, path, name)
	i := r.indexBy(host, path)
	if added {
		r.Projects[i].Hidden = true
	}
	r.Projects[i].LinkedFrom = linkedFrom
	return r.Projects[i], added
}

// Visible returns the projects that are not hidden bookkeeping rows.
func (r Registry) Visible() []Project {
	out := make([]Project, 0, len(r.Projects))
	for _, p := range r.Projects {
		if !p.Hidden {
			out = append(out, p)
		}
	}
	return out
}

func (r *Registry) upsert(host, path, name string) (Project, bool) {
	if i := r.indexBy(host, path); i >= 0 {
		if name != "" {
			r.Projects[i].Name = name
		}
		return r.Projects[i], false
	}
	p := Project{Name: name, Path: path, Host: host, AddedAt: time.Now().UTC()}
	r.Projects = append(r.Projects, p)
	return p, true
}

// Find looks up a project by name or by path. Returns the project and true if
// found; with duplicates it returns the first match — use FindUnique when the
// caller must not guess.
func (r Registry) Find(nameOrPath string) (Project, bool) {
	if i := r.indexByPath(nameOrPath); i >= 0 {
		return r.Projects[i], true
	}
	for _, p := range r.Projects {
		if p.Name == nameOrPath {
			return p, true
		}
	}
	return Project{}, false
}

// matches returns every project whose name, path, or scp-style "host:path"
// matches the query. The same repo cloned locally and on a host shares a
// name, so more than one match is a real possibility, not a corrupt registry.
func (r Registry) matches(query string) []Project {
	var out []Project
	for _, p := range r.Projects {
		if p.Name == query || p.Path == query || (p.Host != "" && p.Host+":"+p.Path == query) {
			out = append(out, p)
		}
	}
	return out
}

// Location renders where a project lives, for error messages and tables.
func (p Project) Location() string {
	if p.Host != "" {
		return p.Host + ":" + p.Path
	}
	return p.Path
}

// ErrNotFound reports no project matched a query.
var ErrNotFound = errors.New("no registered project matched")

// FindUnique looks up a project by name, path, or "host:path", erroring when
// the query is ambiguous instead of guessing a machine.
func (r Registry) FindUnique(query string) (Project, error) {
	m := r.matches(query)
	switch len(m) {
	case 0:
		return Project{}, fmt.Errorf("%w %q (see `mp project list`)", ErrNotFound, query)
	case 1:
		return m[0], nil
	default:
		locs := make([]string, len(m))
		for i, p := range m {
			locs[i] = p.Location()
		}
		return Project{}, fmt.Errorf("project %q is ambiguous: %s — pass the path or host:path", query, strings.Join(locs, ", "))
	}
}

// Remove deletes the project matching name, path, or "host:path". Errors when
// nothing matches or the query is ambiguous.
func (r *Registry) Remove(query string) (Project, error) {
	target, err := r.FindUnique(query)
	if err != nil {
		return Project{}, err
	}
	if i := r.indexBy(target.Host, target.Path); i >= 0 {
		r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
	}
	return target, nil
}
