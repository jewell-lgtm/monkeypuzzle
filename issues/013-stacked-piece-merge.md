---
title: Stack-aware piece merging
status: todo
---

# Stack-aware piece merging

## Description

Modify `mp piece merge` to handle stacked pieces correctly. Child pieces merge into their parent, not main. Root pieces (parent=main) merge into main.

## Requirements

- Read parent from piece metadata
- Merge into parent branch (not always main)
- Validate parent is not ahead of piece
- Update child pieces when parent merges into main
- Block merge if piece has unmerged children

## Implementation Details

### Merge Target Logic

```
if parent == "main":
    merge into main (current behavior)
else:
    merge into parent's branch
```

### Files to Modify

- `internal/core/piece/handler.go` - `MergePiece` reads parent, changes target
- `internal/adapters/git.go` - May need new helpers for cross-branch ops

### Merge Workflow

1. Read piece metadata to get parent
2. If parent is piece (not main):
   - Switch to parent piece's worktree
   - Merge current piece into parent
3. If parent is main:
   - Current behavior (squash merge into main)

### Child Piece Handling

When parent piece merges to main:
- Child pieces need their parent updated to "main"
- Or: child pieces need rebasing onto main

Options:
1. **Rebase children automatically** - risky, conflicts
2. **Update metadata only** - children keep branching from old parent commit
3. **Block parent merge** - require children merge first (safest)

Recommend option 3: block merge if piece has children.

### Command Output

```bash
$ mp piece merge
Error: piece has children: [child-piece-1, child-piece-2]
Merge children first, or use --force to merge anyway
```

## Testing

- Test merge into parent piece
- Test merge into main (root piece)
- Test blocking merge when children exist
- Test `--force` override
- Test metadata reading

