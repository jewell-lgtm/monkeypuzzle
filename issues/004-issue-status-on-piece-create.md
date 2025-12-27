---
title: Update issue status to in-progress when creating piece
status: done
---

# Update issue status to in-progress when creating piece

## Description

When creating a piece from an issue using `mp piece new --issue <path>`, automatically update the issue status from `todo` to `in-progress`.

## Requirements

- After creating piece from issue, update issue status
- Only update if status is currently `todo` or missing
- Use status management functions from issue 02
- Log warning if status update fails (non-fatal)

## Implementation Details

### Files to Modify
- `internal/core/piece/handler.go` - Update `CreatePieceFromIssue` function
- Add dependency on issue status management functions

### Implementation Flow
1. Create piece from issue (existing logic)
2. Extract issue path from marker
3. Update issue status to `in-progress`
4. Log warning if update fails, but don't fail piece creation

### Error Handling
- Non-fatal: If status update fails, log warning and continue
- Don't rollback piece creation if status update fails

## Testing
- Test status updates when creating piece from todo issue
- Test no update if issue already has non-todo status
- Test graceful handling of status update failure
- Test with issues missing status field

