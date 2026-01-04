---
title: Show hierarchy in mp piece status
status: done
---

# Show hierarchy in mp piece status

## Description

Show parent/children info in `mp piece status` output. Depends on 016 (metadata).

## Requirements

- Show parent piece name (if not main)
- Show child pieces (if any)
- Show stack depth
- Indicate merge readiness

## Implementation

### Output Format

```
$ mp piece status
Current piece: auth-oauth

Parent: feature-auth
Children: none
Stack depth: 2

Issue: Add OAuth login
Status: in-progress
Branch: auth-oauth-20250104
```

With children:

```
$ mp piece status
Current piece: feature-auth

Parent: main
Children:
  - auth-oauth
  - auth-saml

⚠ Cannot merge: has unmerged children
```

### Handler Changes

`internal/core/piece/handler.go`:

- Modify GetPieceStatus or add GetPieceHierarchy
- Return parent, children list, stack depth
- Add CanMerge bool (no children, or all children merged)

### Status Output Struct

```go
type PieceHierarchyStatus struct {
    PieceName   string
    Parent      string
    Children    []string
    StackDepth  int
    CanMerge    bool
}
```

## Testing

- Status shows correct parent
- Status shows correct children
- Stack depth calculated correctly
- CanMerge false when children exist
