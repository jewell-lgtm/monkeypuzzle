// Package inbox is the global ordered list of pieces across every registered
// project: the user's manual order first, derived urgency as a tie-break.
// Every UI (pickers, dashboard) is a view over List.
package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/paths"
)

const (
	stateFileName = "inbox.json"
	stateVersion  = 1
)

// ErrNoConfigDir is returned when the user config directory cannot be resolved.
var ErrNoConfigDir = errors.New("inbox: cannot resolve config dir (set MP_CONFIG_DIR or HOME)")

// State is the on-disk inbox: ~/.config/monkeypuzzle/inbox.json. Keys are
// "project/piece". Order is the manual ranking; Notes and Snoozed are keyed
// annotations; Cache holds the last forge lookup per key and may be dropped.
type State struct {
	Version int                   `json:"version"`
	Order   []string              `json:"order"`
	Notes   map[string]string     `json:"notes,omitempty"`
	Snoozed map[string]time.Time  `json:"snoozed,omitempty"`
	Cache   map[string]CacheEntry `json:"cache,omitempty"`
}

// CacheEntry is one key's forge lookup: the PR (nil when the branch has none)
// and when it was fetched.
type CacheEntry struct {
	PR        *PR       `json:"pr,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
}

// PR is the forge record a row carries.
type PR struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
	State  string `json:"state"` // open, merged, closed
	Draft  bool   `json:"draft"`
}

func emptyState() State {
	return State{Version: stateVersion, Order: []string{}}
}

// Path resolves the state file inside the user config dir (MP_CONFIG_DIR wins).
func Path() (string, error) {
	dir, err := paths.ConfigDir()
	if err != nil || dir == "" {
		return "", ErrNoConfigDir
	}
	return filepath.Join(dir, stateFileName), nil
}

// Load reads the state file; a missing file is an empty state.
func Load(path string) (State, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyState(), nil
	}
	if err != nil {
		return State{}, fmt.Errorf("inbox: read %s: %w", path, err)
	}
	st := emptyState()
	if err := json.Unmarshal(data, &st); err != nil {
		return State{}, fmt.Errorf("inbox: parse %s: %w", path, err)
	}
	if st.Order == nil {
		st.Order = []string{}
	}
	return st, nil
}

// Save writes the state atomically (temp file + rename) next to path.
func Save(path string, st State) error {
	st.Version = stateVersion
	if st.Order == nil {
		st.Order = []string{}
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("inbox: encode: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("inbox: mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("inbox: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("inbox: rename: %w", err)
	}
	return nil
}

// Update runs fn on the loaded state under the inbox lock and saves it back
// when fn reports a change. Every read-modify-write (rank edits, stale-key
// drops, cache refreshes) goes through here so concurrent mp processes never
// lose each other's writes. fs is type-asserted for core.FileLocker; an FS
// without locking (MemoryFS) runs unlocked, as elsewhere in core.
func Update(fs core.FS, fn func(st *State) (changed bool, err error)) error {
	path, err := Path()
	if err != nil {
		return err
	}
	if locker, ok := fs.(core.FileLocker); ok {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return fmt.Errorf("inbox: mkdir: %w", err)
		}
		unlock, err := locker.LockFile(path + ".lock")
		if err != nil {
			return fmt.Errorf("inbox: lock: %w", err)
		}
		defer unlock()
	}
	st, err := Load(path)
	if err != nil {
		return err
	}
	changed, err := fn(&st)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return Save(path, st)
}
