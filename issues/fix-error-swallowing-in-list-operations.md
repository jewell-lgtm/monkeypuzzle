---
title: Fix error swallowing in list operations
status: todo
priority: medium
---

# Fix error swallowing in list operations

## Problem

Multiple locations silently skip items when errors occur:

1. `MarkdownProvider.List()` at `markdown_provider.go:80-82` - skips files with parse errors
2. `GetPieceChildren()` at `piece_metadata.go:94-97` - skips pieces with unreadable metadata

No warnings logged, making debugging difficult.

## Impact

- Users don't know why items aren't appearing
- Corrupted files silently ignored
- Violates "No error swallowing" principle from CLAUDE.md

## Locations

- `internal/core/issue/markdown_provider.go:80-82`
- `internal/core/piece/piece_metadata.go:94-97`

## Fix

Distinguish between expected errors (file not found) and unexpected errors:

```go
issue, err := p.Get(filePath)
if err != nil {
    if !os.IsNotExist(err) {
        // Log warning for unexpected errors
        log.Printf("warning: skipping %s: %v", filePath, err)
    }
    continue
}
```
