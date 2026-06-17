// Package projectdir resolves where a repository's monkeypuzzle state directory
// lives. By default that is "<repoRoot>/.monkeypuzzle", but a repo can be
// remapped (e.g. into an already-gitignored ".DONOTCOMMIT/monkeypuzzle") via a
// user-level mapping file at ~/.config/monkeypuzzle/project-dirs.json. The
// mapping has to live outside the repo because monkeypuzzle.json itself lives
// inside the directory being relocated.
//
// This package imports only the standard library and internal/paths to avoid
// import cycles (registry and the init command both depend on it).
package projectdir

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

// DefaultDirName is the directory used when a repo has no custom mapping.
const DefaultDirName = ".monkeypuzzle"

const (
	mappingFileName = "project-dirs.json"
	mappingVersion  = "1"
)

// mapping is the on-disk structure of project-dirs.json.
type mapping struct {
	Version string `json:"version"`
	// Dirs maps an absolute repo root to the relative monkeypuzzle dir within it.
	Dirs map[string]string `json:"dirs"`
}

func mappingPath() (string, error) {
	dir, err := paths.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, mappingFileName), nil
}

// load reads the mapping file. A missing file yields an empty mapping. A
// corrupt file is treated as empty so a bad prefs file doesn't brick the CLI.
func load() mapping {
	p, err := mappingPath()
	if err != nil {
		return mapping{Version: mappingVersion, Dirs: map[string]string{}}
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return mapping{Version: mappingVersion, Dirs: map[string]string{}}
	}
	var m mapping
	if err := json.Unmarshal(data, &m); err != nil {
		return mapping{Version: mappingVersion, Dirs: map[string]string{}}
	}
	if m.Dirs == nil {
		m.Dirs = map[string]string{}
	}
	if m.Version == "" {
		m.Version = mappingVersion
	}
	return m
}

func (m mapping) save() error {
	p, err := mappingPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	if m.Version == "" {
		m.Version = mappingVersion
	}
	if m.Dirs == nil {
		m.Dirs = map[string]string{}
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// RelDir returns the relative monkeypuzzle directory for repoRoot, defaulting
// to ".monkeypuzzle". It never fails: any problem resolving the mapping yields
// the default.
func RelDir(repoRoot string) string {
	if repoRoot == "" {
		return DefaultDirName
	}
	if rel, ok := load().Dirs[repoRoot]; ok && rel != "" {
		return rel
	}
	return DefaultDirName
}

// Dir returns the absolute monkeypuzzle directory for repoRoot.
func Dir(repoRoot string) string {
	return filepath.Join(repoRoot, RelDir(repoRoot))
}

// PiecesDir returns the directory holding piece worktrees for repoRoot.
func PiecesDir(repoRoot string) (string, error) {
	if repoRoot == "" {
		return "", fmt.Errorf("repoRoot is required")
	}
	return filepath.Join(Dir(repoRoot), "pieces"), nil
}

// HooksDir returns the hooks directory for repoRoot.
func HooksDir(repoRoot string) string {
	return filepath.Join(Dir(repoRoot), "hooks")
}

// LogsDir returns the directory holding hook (and other) logs for repoRoot.
func LogsDir(repoRoot string) string {
	return filepath.Join(Dir(repoRoot), "logs")
}

// ConfigFilePath returns the path to repoRoot's monkeypuzzle.json.
func ConfigFilePath(repoRoot string) string {
	return filepath.Join(Dir(repoRoot), "monkeypuzzle.json")
}

// ValidRel reports whether rel is a safe relative directory: not empty, not the
// repo root itself ("."), no absolute paths, no "..", no leading "/" or volume name.
func ValidRel(rel string) bool {
	if rel == "" || rel == "." {
		return false
	}
	return filepath.IsLocal(rel)
}

// Set records repoRoot -> rel in the mapping file. rel must satisfy ValidRel.
// If rel is the default (".monkeypuzzle"), the entry is removed instead.
func Set(repoRoot, rel string) error {
	if repoRoot == "" {
		return fmt.Errorf("repoRoot is required")
	}
	rel = filepath.Clean(rel)
	if !ValidRel(rel) {
		return fmt.Errorf("invalid monkeypuzzle directory %q: must be a relative path within the repo", rel)
	}
	m := load()
	if rel == DefaultDirName {
		delete(m.Dirs, repoRoot)
	} else {
		m.Dirs[repoRoot] = rel
	}
	return m.save()
}

// MainRepoRoot returns the absolute path of the main working tree for the git
// repository containing path, resolving correctly even when path is inside a
// linked worktree. It errors if path is not inside a git repository.
func MainRepoRoot(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	out, err := exec.Command("git", "-C", abs, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git repository", path)
	}
	commonDir := strings.TrimSpace(string(out))
	// commonDir is the main repo's ".git" directory (or a bare repo dir).
	root := filepath.Dir(commonDir)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root, nil
}

// WorktreeDir returns the per-piece monkeypuzzle state directory inside a piece
// worktree, mirroring the main repo's relocation when possible. If the main
// repo can't be determined (e.g. worktreePath isn't a real git worktree, as in
// some tests), it falls back to "<worktreePath>/.monkeypuzzle".
func WorktreeDir(worktreePath string) (string, error) {
	if worktreePath == "" {
		return "", fmt.Errorf("worktreePath is required")
	}
	mainRoot, err := MainRepoRoot(worktreePath)
	if err != nil {
		return filepath.Join(worktreePath, DefaultDirName), nil
	}
	return filepath.Join(worktreePath, RelDir(mainRoot)), nil
}
