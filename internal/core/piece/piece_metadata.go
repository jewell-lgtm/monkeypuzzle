package piece

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/adapters"
	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

const pieceMetadataFilename = "piece-metadata.json"

// Agent status values, ordered by severity for aggregation.
const (
	AgentBlocked = "blocked" // awaiting human input (permission prompt, question)
	AgentWorking = "working" // actively executing
	AgentDone    = "done"    // finished its turn/task
	AgentIdle    = "idle"    // started but not yet asked to do anything
)

// AgentRecord is one agent process reporting status from inside a piece
// worktree. Keyed in PieceMetadata.Agents by an integration-provided id
// (e.g. a Claude Code session id).
type AgentRecord struct {
	// Kind is the agent flavor: "claude", "codex", ...
	Kind   string `json:"kind,omitempty"`
	Status string `json:"status"`
	// PID is the agent process, used for lazy liveness reaping. 0 = unknown.
	PID int `json:"pid,omitempty"`
	// Pane is the multiplexer pane the agent runs in (tmux $TMUX_PANE),
	// letting UIs jump focus straight to the agent.
	Pane      string    `json:"pane,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AggregateAgents collapses a piece's agents to one status by severity:
// blocked > working > done > idle. Empty map returns "".
func AggregateAgents(agents map[string]AgentRecord) string {
	agg := ""
	rank := map[string]int{AgentIdle: 1, AgentDone: 2, AgentWorking: 3, AgentBlocked: 4}
	for _, rec := range agents {
		if rank[rec.Status] > rank[agg] {
			agg = rec.Status
		}
	}
	return agg
}

// CountAgents returns per-status counts, omitting zero statuses.
func CountAgents(agents map[string]AgentRecord) map[string]int {
	if len(agents) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, rec := range agents {
		counts[rec.Status]++
	}
	return counts
}

// processAlive is swapped out in tests.
var processAlive = adapters.ProcessAlive

// LiveAgents filters records whose process has died (a SIGKILLed agent never
// reports "gone"), so read paths don't surface stale badges forever.
func LiveAgents(agents map[string]AgentRecord) map[string]AgentRecord {
	live := make(map[string]AgentRecord, len(agents))
	for id, rec := range agents {
		if rec.PID > 0 && !processAlive(rec.PID) {
			continue
		}
		live[id] = rec
	}
	return live
}

// PieceMetadata stores parent-child relationship metadata for a piece
type PieceMetadata struct {
	// Parent is the parent piece name or "main" for root pieces
	Parent string `json:"parent"`
	// CreatedFromBranch is the git branch the piece was created from
	CreatedFromBranch string `json:"created_from_branch"`
	// Prompt is the free-form description a prompt-created piece was made from.
	// Empty for pieces created from a bare name.
	Prompt string `json:"prompt,omitempty"`
	// Merged records that `mp merge` squash-merged this piece's branch into its
	// target. It is the authoritative, durable signal cleanup/done consult first:
	// a multi-commit squash collapses into one commit on the target whose
	// patch-id matches none of the piece's individual commits, so git-based
	// heuristics (branch --merged, git cherry) cannot detect it. Recording at
	// merge time sidesteps that entirely.
	Merged bool `json:"merged,omitempty"`
	// Agents tracks agent processes running in this piece's worktree, keyed by
	// agent id. Maintained by `mp agent report`; reaped lazily on write.
	Agents map[string]AgentRecord `json:"agents,omitempty"`
}

// MarkPieceMerged sets the durable merged marker on a piece's metadata so that
// cleanup/done recognise it as merged regardless of squash topology. Reads,
// flips the flag, and writes back, preserving all other fields.
func MarkPieceMerged(worktreePath string, fs core.FS) error {
	metadata, err := ReadPieceMetadata(worktreePath, fs)
	if err != nil {
		return err
	}
	if metadata.Merged {
		return nil
	}
	metadata.Merged = true
	return WritePieceMetadata(worktreePath, *metadata, fs)
}

// DefaultPieceMetadata returns metadata with default values (parent=main)
func DefaultPieceMetadata() PieceMetadata {
	return PieceMetadata{
		Parent:            "main",
		CreatedFromBranch: "",
	}
}

// ReadPieceMetadata reads piece metadata from a worktree.
// Returns default metadata (parent=main) if file doesn't exist.
func ReadPieceMetadata(worktreePath string, fs core.FS) (*PieceMetadata, error) {
	mpDir, err := projectdir.WorktreeDir(worktreePath)
	if err != nil {
		return nil, err
	}
	metadataPath := filepath.Join(mpDir, pieceMetadataFilename)
	data, err := fs.ReadFile(metadataPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default metadata for backward compatibility
			def := DefaultPieceMetadata()
			return &def, nil
		}
		return nil, fmt.Errorf("failed to read piece metadata: %w", err)
	}

	var metadata PieceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse piece metadata: %w", err)
	}

	return &metadata, nil
}

// WritePieceMetadata writes piece metadata to a worktree
func WritePieceMetadata(worktreePath string, metadata PieceMetadata, fs core.FS) error {
	// Ensure the monkeypuzzle state directory exists
	mpDir, err := projectdir.WorktreeDir(worktreePath)
	if err != nil {
		return err
	}
	if err := fs.MkdirAll(mpDir, DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create monkeypuzzle directory: %w", err)
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal piece metadata: %w", err)
	}

	// Write-temp-then-rename: a reader never sees a torn file, even with
	// concurrent writers (agent report hooks fire whenever agents feel like it).
	metadataPath := filepath.Join(mpDir, pieceMetadataFilename)
	tmpPath := metadataPath + ".tmp"
	if err := fs.WriteFile(tmpPath, data, initcmd.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write piece metadata: %w", err)
	}
	if err := fs.Rename(tmpPath, metadataPath); err != nil {
		return fmt.Errorf("failed to write piece metadata: %w", err)
	}

	return nil
}

// LockPieceMetadata serializes read-modify-write cycles on a piece's metadata
// when the FS supports locking (real filesystems; MemoryFS in tests does
// not). Returns an unlock func, which is a no-op when locking is unavailable.
func LockPieceMetadata(worktreePath string, fs core.FS) (func(), error) {
	locker, ok := fs.(core.FileLocker)
	if !ok {
		return func() {}, nil
	}
	mpDir, err := projectdir.WorktreeDir(worktreePath)
	if err != nil {
		return nil, err
	}
	if err := fs.MkdirAll(mpDir, DefaultDirPerm); err != nil {
		return nil, err
	}
	return locker.LockFile(filepath.Join(mpDir, pieceMetadataFilename+".lock"))
}

// GetPieceChildren returns the names of all pieces that have the given piece as their parent.
// It scans all pieces in the piecesDir and returns those whose metadata.Parent matches pieceName.
func GetPieceChildren(pieceName string, piecesDir string, fs core.FS) ([]string, error) {
	entries, err := fs.ReadDir(piecesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read pieces directory: %w", err)
	}

	var children []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		candidateName := entry.Name()
		candidatePath := filepath.Join(piecesDir, candidateName)

		metadata, err := ReadPieceMetadata(candidatePath, fs)
		if err != nil {
			// Log warning for unexpected errors (ReadPieceMetadata returns default if file doesn't exist)
			fmt.Fprintf(os.Stderr, "warning: skipping %s: %v\n", candidateName, err)
			continue
		}

		if metadata.Parent == pieceName {
			children = append(children, candidateName)
		}
	}

	return children, nil
}

// HasChildren returns true if the given piece has any child pieces.
// This is useful for blocking merge operations when children exist.
func HasChildren(pieceName string, piecesDir string, fs core.FS) (bool, error) {
	children, err := GetPieceChildren(pieceName, piecesDir, fs)
	if err != nil {
		return false, err
	}
	return len(children) > 0, nil
}
