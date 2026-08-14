package piece

import "errors"

// ErrNotInPiece is the shared "you're not standing in a piece worktree" error.
// One phrasing everywhere — command layers may wrap it with a command-specific
// remedy, but the base sentence never varies.
var ErrNotInPiece = errors.New("not in a piece worktree; run this from inside a piece")
