package piece

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// PieceInfo contains information about a created piece worktree.
// It includes the piece name, worktree path, and associated tmux session name.
type PieceInfo struct {
	// Name is the unique identifier for this piece (e.g., "piece-20250127-143022")
	Name string `json:"name"`
	// WorktreePath is the absolute path to the git worktree directory
	WorktreePath string `json:"worktree_path"`
	// SessionName is the name of the tmux session created for this piece
	SessionName string `json:"session_name"`
}

// PieceStatus contains information about the current piece status.
// It indicates whether the current directory is in a piece worktree or the main repository.
type PieceStatus struct {
	// InPiece is true if the current directory is within a piece worktree
	InPiece bool `json:"in_piece"`
	// PieceName is the name of the piece, only set when InPiece is true
	PieceName string `json:"piece_name,omitempty"`
	// WorktreePath is the path to the worktree, only set when InPiece is true
	WorktreePath string `json:"worktree_path,omitempty"`
	// RepoRoot is the path to the main repository root
	RepoRoot string `json:"repo_root,omitempty"`
}

// PieceListItem represents a piece available for switching.
type PieceListItem struct {
	Name         string    `json:"name"`
	WorktreePath string    `json:"worktree_path"`
	SessionName  string    `json:"session_name"`
	HasSession   bool      `json:"has_session"`
	ModTime      time.Time `json:"mod_time"`
}

// SwitchResult contains the result of a switch operation.
type SwitchResult struct {
	Piece  PieceListItem `json:"piece"`
	Method string        `json:"method"` // "tmux-switch", "tmux-attach", "path"
}

// NewPieceInput holds input for the piece new command.
// Either IssuePath or Name must be provided (mutually exclusive).
type NewPieceInput struct {
	IssuePath string `json:"issue_path,omitempty"`
	Name      string `json:"name,omitempty"`
}

// NewPieceSchema returns the JSON schema for piece new input.
func NewPieceSchema() ([]byte, error) {
	schema := map[string]any{
		"issue_path": "",
		"name":       "",
	}
	return json.MarshalIndent(schema, "", "  ")
}

// ParseNewPieceJSON parses JSON input into NewPieceInput.
func ParseNewPieceJSON(data []byte) (NewPieceInput, error) {
	var input NewPieceInput
	if err := json.Unmarshal(data, &input); err != nil {
		return NewPieceInput{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return input, nil
}

// ValidateNewPieceInput validates the input.
// One of IssuePath or Name must be provided, but not both.
func ValidateNewPieceInput(input NewPieceInput) error {
	hasIssue := strings.TrimSpace(input.IssuePath) != ""
	hasName := strings.TrimSpace(input.Name) != ""

	if hasIssue && hasName {
		return fmt.Errorf("cannot specify both issue_path and name")
	}
	if !hasIssue && !hasName {
		return fmt.Errorf("must specify either issue_path or name")
	}
	return nil
}

// WithNewPieceDefaults returns input with whitespace trimmed.
func WithNewPieceDefaults(input NewPieceInput) NewPieceInput {
	return NewPieceInput{
		IssuePath: strings.TrimSpace(input.IssuePath),
		Name:      strings.TrimSpace(input.Name),
	}
}
