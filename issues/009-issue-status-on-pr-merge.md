---
title: Update issue status to done when PR is merged
status: done
---

# Update issue status to done when PR is merged

## Description

When a piece is cleaned up because its branch has been merged, automatically update the linked issue status from `in-progress` to `done`.

## Requirements

- During piece cleanup, check if issue marker exists
- Read issue path from marker
- Update issue status to `done`
- Only update if status is currently `in-progress`
- Log warning if update fails (non-fatal)

## Implementation Details

### Files to Modify
- `internal/core/piece/handler.go` - Update `CleanupMergedPieces` function
- Integrate with issue status management (issue 02)

### Implementation Flow
1. Detect merged piece (existing logic from issue 06)
2. Read `.monkeypuzzle/current-issue.json` from piece worktree
3. If issue marker exists:
   - Extract issue path
   - Resolve to absolute path in repo root
   - Update issue status to `done`
   - Log warning if update fails (continue cleanup)

### Status Update Logic
- Only update if current status is `in-progress`
- Skip if status is already `done` (idempotent)
- Skip if issue file doesn't exist
- Non-fatal: Don't fail cleanup if status update fails

## Testing
- Test issue status update during cleanup
- Test cleanup when issue marker missing
- Test cleanup when issue file missing
- Test idempotent updates (already done)
- Test graceful handling of update failures

