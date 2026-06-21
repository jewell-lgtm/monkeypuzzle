package piece

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
	"github.com/jewell-lgtm/monkeypuzzle/internal/projectdir"
)

const pieceMetadataFilename = "piece-metadata.json"

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

	metadataPath := filepath.Join(mpDir, pieceMetadataFilename)
	if err := fs.WriteFile(metadataPath, data, initcmd.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write piece metadata: %w", err)
	}

	return nil
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
