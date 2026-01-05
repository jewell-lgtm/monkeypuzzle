---
title: Add missing CLI input validation
status: todo
priority: high
---

# Add missing CLI input validation

## Problem

Several CLI input functions don't validate before returning:
- `getMergeInput()` at piece.go:549-572
- `getUpdateInput()` at piece.go:485-502
- `getCleanupInput()` at piece.go:636-658 (destructive operation!)

Other functions correctly call validation (e.g., `ValidateNewPieceInput` at piece.go:404).

## Impact

- Invalid input reaches handlers without validation
- Cleanup is destructive; missing validation increases risk
- Inconsistent behavior across commands

## Location

- `cmd/mp/piece.go:549-572` (getMergeInput)
- `cmd/mp/piece.go:485-502` (getUpdateInput)
- `cmd/mp/piece.go:636-658` (getCleanupInput)

## Fix

Add validation calls after applying defaults:

```go
// In getMergeInput:
input = piececmd.WithMergeDefaults(input)
if err := piececmd.ValidateMergeInput(input); err != nil {
    return piececmd.MergeInput{}, err
}
return input, nil
```

Note: May need to add ValidateMergeInput, ValidateUpdateInput, ValidateCleanupInput functions.
