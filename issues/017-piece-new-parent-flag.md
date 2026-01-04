---
title: Create piece with --parent flag
status: done
---

# Create piece with --parent flag

## Description

Add `--parent` flag to `mp piece new` to create child pieces. Depends on 016 (metadata).

## Requirements

- `mp piece new --parent <piece-name>` creates child of that piece
- Child branches from parent's current commit (not main)
- Writes parent to piece metadata
- Validates parent piece exists
- Default: parent=main (current behavior)

## Implementation

### CLI Changes

`cmd/mp/piece.go`:

```go
pieceNewCmd.Flags().StringP("parent", "p", "", "parent piece name")
```

### Input Changes

`internal/core/piece/input.go`:

```go
type CreatePieceInput struct {
    Name       string
    ParentName string  // new field, default "main"
}
```

### Handler Changes

`internal/core/piece/handler.go` - CreatePiece:

1. If ParentName != "main":
   - Validate parent piece exists
   - Get parent piece worktree path
   - Get parent's current branch name
2. Create worktree from parent's branch (not main)
3. Write piece metadata with parent

### Git Commands

```bash
# Current (from main)
git worktree add <path> -b <branch>

# From parent piece
git worktree add <path> -b <branch> parent-branch-name
```

## Testing

- Create child of existing piece
- Validate parent must exist
- Child metadata has correct parent
- Child branches from parent's commit
- Default creates root piece (parent=main)
