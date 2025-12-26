package piece

// PieceInfo contains information about a created piece
type PieceInfo struct {
	Name        string `json:"name"`
	WorktreePath string `json:"worktree_path"`
	SessionName string `json:"session_name"`
}

// PieceStatus contains information about the current piece status
type PieceStatus struct {
	InPiece     bool   `json:"in_piece"`
	PieceName   string `json:"piece_name,omitempty"`
	WorktreePath string `json:"worktree_path,omitempty"`
	RepoRoot    string `json:"repo_root,omitempty"`
}

