---
title: Fix SwitchPiece bypassing structured output
status: done
priority: medium
---

# Fix SwitchPiece bypassing structured output

## Problem

`SwitchPiece()` at `handler.go:1437` uses `fmt.Println(target.WorktreePath)` directly instead of using the structured logging system (`h.deps.Output.Write()`).

## Impact

- Output goes to wrong destination in stdin JSON mode
- Cannot be captured or structured consistently
- Breaks tooling that expects all output through Output interface

## Location

- `internal/core/piece/handler.go:1437`

## Fix

Replace with structured output:

```go
// Instead of:
fmt.Println(target.WorktreePath)

// Use:
h.deps.Output.Write(core.Message{
    Type:    core.MsgInfo,
    Content: target.WorktreePath,
})
```

Note: Lines 1412-1415 and 1424-1427 correctly use `h.deps.Output.Write()`, only the fallback at 1437 doesn't.
