package piece

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jewell-lgtm/monkeypuzzle/internal/core"
	initcmd "github.com/jewell-lgtm/monkeypuzzle/internal/core/init"
)

const prMetadataFilename = "pr-metadata.json"

// PRMetadata stores information about a PR created for a piece
type PRMetadata struct {
	PRNumber   int       `json:"pr_number"`
	PRURL      string    `json:"pr_url"`
	Branch     string    `json:"branch"`
	BaseBranch string    `json:"base_branch"`
	CreatedAt  time.Time `json:"created_at"`
	IssuePath  string    `json:"issue_path,omitempty"` // Set if piece was created from an issue
}

// ReadPRMetadata reads PR metadata from a piece worktree
func ReadPRMetadata(worktreePath string, fs core.FS) (*PRMetadata, error) {
	metadataPath := filepath.Join(worktreePath, initcmd.DirName, prMetadataFilename)
	data, err := fs.ReadFile(metadataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read PR metadata: %w", err)
	}

	var metadata PRMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, fmt.Errorf("failed to parse PR metadata: %w", err)
	}

	return &metadata, nil
}

// WritePRMetadata writes PR metadata to a piece worktree
func WritePRMetadata(worktreePath string, metadata PRMetadata, fs core.FS) error {
	// Ensure .monkeypuzzle directory exists
	mpDir := filepath.Join(worktreePath, initcmd.DirName)
	if err := fs.MkdirAll(mpDir, DefaultDirPerm); err != nil {
		return fmt.Errorf("failed to create .monkeypuzzle directory: %w", err)
	}

	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal PR metadata: %w", err)
	}

	metadataPath := filepath.Join(mpDir, prMetadataFilename)
	if err := fs.WriteFile(metadataPath, data, initcmd.DefaultFilePerm); err != nil {
		return fmt.Errorf("failed to write PR metadata: %w", err)
	}

	return nil
}
