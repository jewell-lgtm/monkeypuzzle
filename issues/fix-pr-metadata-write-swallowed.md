---
title: Fix PR metadata write failure being swallowed
status: done
priority: high
---

# Fix PR metadata write failure being swallowed

## Problem

In `CreatePR()` at `handler.go:128-132`, PR metadata write failure is logged as warning but function returns success. User believes PR was created successfully but metadata wasn't persisted.

## Impact

- Subsequent commands reading PR metadata will fail
- User not aware of incomplete operation
- Breaks cleanup and status commands

## Location

- `internal/core/pr/handler.go:128-132`

## Fix

Make metadata write failure fatal:

```go
if err := piece.WritePRMetadata(status.WorktreePath, metadata, h.deps.FS); err != nil {
    return PRResult{}, fmt.Errorf("failed to write PR metadata: %w", err)
}
```

## Testing

- Add test for metadata write failure scenario
- Verify error is returned, not swallowed
