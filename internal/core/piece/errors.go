package piece

import "errors"

// ErrPieceExists reports a piece name already taken in this project — by a
// local worktree or by a piece placed on a box (one namespace across both).
var ErrPieceExists = errors.New("piece already exists")

// ErrNotInPiece is the shared "you're not standing in a piece worktree" error.
// One phrasing everywhere — command layers may wrap it with a command-specific
// remedy, but the base sentence never varies.
var ErrNotInPiece = errors.New("not in a piece worktree; run this from inside a piece")

// ErrPiecePlaced reports a piece that lives on a box (`mp create --remote`):
// core verbs operate on local worktrees only; the CLI proxies placed pieces.
var ErrPiecePlaced = errors.New("piece lives on a box")
