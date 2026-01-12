---
title: Fix ListPieces not populating Parent field
status: done
priority: critical
---

# Fix ListPieces not populating Parent field

## Problem

`ListPieces()` in `handler.go:1251-1257` never populates the `Parent` field in `PieceListItem`. The Parent metadata is stored in piece metadata files but is never read when building the list.

## Impact

- `BuildPieceTree()` relies on Parent field to construct hierarchies
- All pieces are incorrectly treated as orphaned
- `mp piece list` shows broken tree structure

## Location

- `internal/core/piece/handler.go:1251-1257`

## Fix

Read piece metadata for each piece and populate the Parent field:

```go
// In ListPieces, after creating piece entry:
metadata, err := ReadPieceMetadata(piecePath, h.deps.FS)
if err == nil {
    piece.Parent = metadata.Parent
}
```

## Testing

- Add test that verifies Parent field is populated
- Test BuildPieceTree with actual parent-child relationships
