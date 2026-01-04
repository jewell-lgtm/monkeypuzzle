---
title: Refactor input/execution layer separation
status: todo
---

# Refactor input/execution layer separation

Clean up inconsistencies in input layer abstraction across commands.

## Issues

1. **Inconsistent validation placement** - `getIssueInput()` validates internally, `getPieceNewInput()` leaves it to caller
2. **Handler has two entry points** - cmd layer routes between `CreatePiece`/`CreatePieceFromIssue` instead of handler
3. **Options bypass input struct** - `OverwriteSession`, `SkipSwitch` read from flags directly
4. **Duplicate terminal detection** - 6 copies of `isTerminal()`/`hasStdinData()` functions

## Tasks

- [ ] Move validation inside `getPieceNewInput()` to match `getIssueInput()` pattern
- [ ] Unify handler to single `CreatePiece(ctx, srcDir, input, opts)` that routes internally
- [ ] Add `SkipSwitch`/`OverwriteSession` to `NewPieceInput` struct
- [ ] Extract shared `isTerminal()`/`hasStdinData()` to pkg/cli or similar
- [ ] Apply same pattern to `piece switch`, `piece abandon` commands
