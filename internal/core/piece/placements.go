package piece

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

// placementsFilename is the controller-side link store: which pieces of this
// project live on a box (`mp create --remote`). It sits next to
// monkeypuzzle.json, deliberately NOT under pieces/ — ListPieces treats every
// directory there as a worktree.
const placementsFilename = "placements.json"

// PlacedState is the State a placed piece reports until a live refresh has
// talked to its box.
const PlacedState = "unknown"

// Placement is one controller-side link to a piece living on a box. The
// worktree, hooks and forge calls all happen there; the controller only
// remembers where to proxy.
type Placement struct {
	// Box is the ssh destination the piece lives on.
	Box string `json:"box"`
	// RemotePath is the piece worktree path on the box; empty while Pending.
	RemotePath string `json:"remote_path,omitempty"`
	// RemoteProject is the box-side clone's repo root.
	RemoteProject string `json:"remote_project,omitempty"`
	// Pending is true from the moment the link is written until the box-side
	// create has succeeded. A pending link after a crash is reported by
	// `mp remote doctor` and reaped by `mp cleanup`.
	Pending bool `json:"pending,omitempty"`
	// Cached is the last list row fetched from the box, if any.
	Cached *PieceListItem `json:"cached,omitempty"`
}

// Placements maps piece name → Placement.
type Placements map[string]Placement

// PlacementsPath returns the placements.json path for repoRoot.
func PlacementsPath(repoRoot string) string {
	return filepath.Join(projectdir.Dir(repoRoot), placementsFilename)
}

// ReadPlacements loads the link store; a missing file is an empty store.
func ReadPlacements(repoRoot string, fs core.FS) (Placements, error) {
	if repoRoot == "" {
		return Placements{}, nil
	}
	data, err := fs.ReadFile(PlacementsPath(repoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return Placements{}, nil
		}
		return nil, fmt.Errorf("failed to read placements: %w", err)
	}
	var p Placements
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("failed to parse placements: %w", err)
	}
	if p == nil {
		p = Placements{}
	}
	return p, nil
}

// WritePlacements stores the link store atomically (write-temp-then-rename,
// like piece metadata). An empty store removes the file.
func WritePlacements(repoRoot string, p Placements, fs core.FS) error {
	path := PlacementsPath(repoRoot)
	if len(p) == 0 {
		if err := fs.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove placements: %w", err)
		}
		return nil
	}
	if err := fs.MkdirAll(filepath.Dir(path), DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create monkeypuzzle directory: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal placements: %w", err)
	}
	tmp := path + ".tmp"
	if err := fs.WriteFile(tmp, data, initcmd.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write placements: %w", err)
	}
	if err := fs.Rename(tmp, path); err != nil {
		return fmt.Errorf("failed to write placements: %w", err)
	}
	return nil
}

// LockPlacements serializes read-modify-write cycles on the link store when
// the FS supports locking. The unlock func is a no-op otherwise.
func LockPlacements(repoRoot string, fs core.FS) (func(), error) {
	locker, ok := fs.(core.FileLocker)
	if !ok {
		return func() {}, nil
	}
	path := PlacementsPath(repoRoot)
	if err := fs.MkdirAll(filepath.Dir(path), DefaultDirPerm); err != nil {
		return nil, err
	}
	return locker.LockFile(path + ".lock")
}

// UpdatePlacements runs fn against the locked, freshly-read store and writes
// the result back. fn may mutate the map in place.
func UpdatePlacements(repoRoot string, fs core.FS, fn func(Placements) error) error {
	unlock, err := LockPlacements(repoRoot, fs)
	if err != nil {
		return err
	}
	defer unlock()
	p, err := ReadPlacements(repoRoot, fs)
	if err != nil {
		return err
	}
	if err := fn(p); err != nil {
		return err
	}
	return WritePlacements(repoRoot, p, fs)
}

// RemovePlacement drops one link; a missing name is not an error.
func RemovePlacement(repoRoot, name string, fs core.FS) error {
	return UpdatePlacements(repoRoot, fs, func(p Placements) error {
		delete(p, name)
		return nil
	})
}

// On returns the names of pieces placed on box, sorted.
func (p Placements) On(box string) []string {
	var names []string
	for name, pl := range p {
		if pl.Box == box {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

// PieceExists reports whether name is taken in this project, either as a
// local worktree or as a placement on a box. Piece names are one namespace
// across both so `mp <verb> <name>` is never ambiguous.
func (h *Handler) PieceExists(repoRoot, name string) (bool, error) {
	piecesDir, err := getPiecesDir(repoRoot)
	if err != nil {
		return false, err
	}
	if _, err := h.deps.FS.Stat(filepath.Join(piecesDir, name)); err == nil {
		return true, nil
	}
	placements, err := ReadPlacements(repoRoot, h.deps.FS)
	if err != nil {
		return false, err
	}
	_, placed := placements[name]
	return placed, nil
}

// placedItems renders placements as list rows. They are never worktrees on
// this machine: WorktreePath is the box-side path and State starts as
// PlacedState until something refreshes it from the box.
func (h *Handler) placedItems(repoRoot string) []PieceListItem {
	placements, err := ReadPlacements(repoRoot, h.deps.FS)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		return nil
	}
	var items []PieceListItem
	for name, pl := range placements {
		item := PieceListItem{
			Name:         name,
			Host:         pl.Box,
			WorktreePath: pl.RemotePath,
			SessionName:  h.pieceSessionName(repoRoot, name),
			Parent:       "main",
			State:        PlacedState,
		}
		if pl.Cached != nil {
			c := *pl.Cached
			if c.Parent != "" {
				item.Parent = c.Parent
			}
			item.Branch = c.Branch
			item.ModTime = c.ModTime
			item.AgentStatus = c.AgentStatus
			item.AgentCounts = c.AgentCounts
		}
		if pl.Pending {
			item.State = "pending"
		}
		items = append(items, item)
	}
	return items
}

// IsPlaced reports whether the item is a piece on a box rather than a local
// worktree.
func (p PieceListItem) IsPlaced() bool { return p.Host != "" }

// LocalPieces filters out placed items, for callers that walk worktrees.
func LocalPieces(items []PieceListItem) []PieceListItem {
	out := items[:0:0]
	for _, it := range items {
		if !it.IsPlaced() {
			out = append(out, it)
		}
	}
	return out
}
