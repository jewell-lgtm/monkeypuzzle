---
title: Piece parent/child metadata storage
status: done
---

# Piece parent/child metadata storage

## Description

Store parent-child relationship metadata for pieces. Foundation for stacked pieces (013, 014).

## Requirements

- Store parent piece name in piece metadata
- Track children for blocking merge
- Default parent = "main" (backward compat)
- Persist across sessions

## Implementation

### Metadata Structure

New file `.monkeypuzzle/piece-metadata.json` in each piece worktree:

```json
{
  "parent": "main",
  "created_from_branch": "main"
}
```

Or for child piece:

```json
{
  "parent": "parent-piece-name",
  "created_from_branch": "parent-piece-branch-name"
}
```

### Input/Types

Add to `internal/core/piece/input.go`:

```go
type PieceMetadata struct {
    Parent            string `json:"parent"`
    CreatedFromBranch string `json:"created_from_branch"`
}
```

### Handler Functions

New in `internal/core/piece/handler.go`:

- `ReadPieceMetadata(piecePath) PieceMetadata` - read from worktree
- `WritePieceMetadata(piecePath, meta)` - write to worktree
- `GetPieceChildren(pieceName) []string` - scan all pieces for children
- `HasChildren(pieceName) bool` - for merge blocking

### Files to Modify

- `internal/core/piece/input.go` - add PieceMetadata struct
- `internal/core/piece/handler.go` - add read/write/query funcs
- `internal/core/piece/handler.go` - CreatePiece writes metadata

## Testing

- Read/write metadata round-trip
- GetPieceChildren finds correct children
- HasChildren returns correct bool
- Missing metadata returns default (parent=main)
