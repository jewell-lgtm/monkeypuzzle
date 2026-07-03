// Package registry maintains the global list of monkeypuzzle projects known to
// this user. It is stored as JSON in the user data directory and is independent
// of any single repository.
package registry

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
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
	sort.Slice(r.Projects, func(i, j int) bool { return r.Projects[i].Name < r.Projects[j].Name })
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

// UpsertRemote is Upsert for a project living on an ssh host.
func (r *Registry) UpsertRemote(host, path, name string) (Project, bool) {
	return r.upsert(host, path, name)
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
// found.
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

// Remove deletes the project matching name or path. Returns the removed project
// and true if anything was removed.
func (r *Registry) Remove(nameOrPath string) (Project, bool) {
	for i, p := range r.Projects {
		if p.Path == nameOrPath || p.Name == nameOrPath {
			r.Projects = append(r.Projects[:i], r.Projects[i+1:]...)
			return p, true
		}
	}
	return Project{}, false
}
