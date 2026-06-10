package stack

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

const snapshotFilename = "stack-snapshot.json"

// Snapshot records every piece branch's commit before a stack mutation, so
// `mp stack undo` can restore them. Local-only: remote branches are untouched.
type Snapshot struct {
	CreatedAt time.Time         `json:"created_at"`
	Operation string            `json:"operation"` // e.g. "sync"
	Pieces    map[string]string `json:"pieces"`    // piece name -> commit SHA
}

func snapshotPath(mainRepoRoot string) string {
	return filepath.Join(projectdir.Dir(mainRepoRoot), snapshotFilename)
}

// writeSnapshot records the current SHA of every piece branch. Pieces whose
// branch can't be resolved (e.g. deleted branch) are skipped.
func (h *Handler) writeSnapshot(ctx context.Context, mainRepoRoot, piecesDir, operation string) error {
	entries, err := h.deps.FS.ReadDir(piecesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no pieces, nothing to snapshot
		}
		return fmt.Errorf("failed to list pieces for snapshot: %w", err)
	}

	snap := Snapshot{CreatedAt: time.Now(), Operation: operation, Pieces: map[string]string{}}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sha, err := h.git.GetBranchCommit(ctx, mainRepoRoot, e.Name())
		if err != nil {
			continue
		}
		snap.Pieces[e.Name()] = sha
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	return h.deps.FS.WriteFile(snapshotPath(mainRepoRoot), data, 0644)
}

// readSnapshot loads the last recorded snapshot, or a descriptive error when
// none exists.
func (h *Handler) readSnapshot(mainRepoRoot string) (Snapshot, error) {
	var snap Snapshot
	data, err := h.deps.FS.ReadFile(snapshotPath(mainRepoRoot))
	if err != nil {
		if os.IsNotExist(err) {
			return snap, fmt.Errorf("no stack snapshot found; snapshots are written by 'mp stack sync'")
		}
		return snap, fmt.Errorf("failed to read stack snapshot: %w", err)
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return snap, fmt.Errorf("failed to parse stack snapshot: %w", err)
	}
	return snap, nil
}

// UndoResult reports what `mp stack undo` restored.
type UndoResult struct {
	Restored []UndoRestoredPiece `json:"restored"`
	Skipped  []string            `json:"skipped,omitempty"` // pieces already at the snapshot SHA, or gone
}

// UndoRestoredPiece is one branch moved back to its snapshot commit.
type UndoRestoredPiece struct {
	Piece string `json:"piece"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// Undo restores every piece branch to the SHA recorded in the last snapshot
// (written by `mp stack sync` before it mutates anything). It refuses to run if
// any affected worktree has uncommitted changes, and never touches remotes —
// re-push with force-with-lease afterwards if branches were already pushed.
func (h *Handler) Undo(ctx context.Context, workDir string) (UndoResult, error) {
	var result UndoResult
	mainRepoRoot, piecesDir, err := h.resolveRepo(ctx, workDir)
	if err != nil {
		return result, err
	}

	snap, err := h.readSnapshot(mainRepoRoot)
	if err != nil {
		return result, err
	}

	// Plan first: find pieces that moved, and refuse on any dirty worktree
	// before touching anything.
	type planned struct{ name, from, to, worktree string }
	var plan []planned
	names := make([]string, 0, len(snap.Pieces))
	for name := range snap.Pieces {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		want := snap.Pieces[name]
		worktree := filepath.Join(piecesDir, name)
		if _, err := h.deps.FS.Stat(worktree); err != nil {
			result.Skipped = append(result.Skipped, name)
			continue
		}
		current, err := h.git.GetBranchCommit(ctx, mainRepoRoot, name)
		if err != nil || current == want {
			result.Skipped = append(result.Skipped, name)
			continue
		}
		// Untracked files survive reset --hard; only tracked changes block undo.
		dirty, err := h.git.HasTrackedChanges(ctx, worktree)
		if err != nil {
			return UndoResult{}, fmt.Errorf("failed to check %q for uncommitted changes: %w", name, err)
		}
		if dirty {
			return UndoResult{}, fmt.Errorf("piece %q has uncommitted changes at %s; commit or stash them, then re-run 'mp stack undo'", name, worktree)
		}
		plan = append(plan, planned{name: name, from: current, to: want, worktree: worktree})
	}

	if len(plan) == 0 {
		h.emit(core.MsgInfo, "Nothing to undo: all pieces already match the snapshot.")
		return result, nil
	}

	for _, p := range plan {
		if err := h.git.ResetHard(ctx, p.worktree, p.to); err != nil {
			return result, fmt.Errorf("failed to restore %q to %s: %w (already restored: %d piece(s))", p.name, p.to[:12], err, len(result.Restored))
		}
		result.Restored = append(result.Restored, UndoRestoredPiece{Piece: p.name, From: p.from, To: p.to})
	}

	h.emit(core.MsgSuccess, fmt.Sprintf("Restored %d piece(s) to the pre-sync snapshot. Remotes were not touched; force-push with lease if you had pushed.", len(result.Restored)))
	return result, nil
}
