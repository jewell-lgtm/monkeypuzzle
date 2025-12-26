package piece

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

