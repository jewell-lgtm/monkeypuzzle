---
title: Tree display in mp piece list
status: todo
---

# Tree display in mp piece list

## Description

Show piece hierarchy as tree in `mp piece list`. Depends on 016 (metadata).

## Requirements

- Display parent/child relationships
- Indent children under parents
- Show orphaned pieces (parent merged) distinctly
- Flag: `--flat` to disable tree (current behavior)

## Implementation

### Output Format

```
$ mp piece list
main
├── feature-auth (1h ago)
│   └── auth-oauth (30m ago)
├── feature-dashboard (2h ago)
└── bugfix-login (3h ago)

$ mp piece list --flat
feature-auth (1h ago)
auth-oauth (30m ago)
feature-dashboard (2h ago)
bugfix-login (3h ago)
```

### Orphaned Pieces

When parent was merged but child wasn't rebased:

```
$ mp piece list
main
├── feature-x (1h ago)
└── (orphaned)
    └── child-of-deleted-parent (2h ago)
```

### Handler Changes

`internal/core/piece/handler.go`:

- Modify ListPieces to return hierarchy info
- Add parent field to PieceListItem
- New func: BuildPieceTree(pieces []PieceListItem) TreeNode

### CLI Changes

`cmd/mp/piece.go`:

- Add `--flat` flag to list command
- Default: tree view
- Tree rendering logic

## Testing

- Tree renders correctly
- Children under correct parents
- Orphans grouped correctly
- --flat matches current output
