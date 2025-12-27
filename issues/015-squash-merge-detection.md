---
title: Detect squash-merged PRs without metadata
status: done
---

# Detect squash-merged PRs without metadata

## Problem

`mp piece cleanup` fails to detect merged pieces when:
1. PR was squash-merged (commit history check fails)
2. No `pr-metadata.json` exists (PR status check skipped)

This happens for pieces created manually or before `mp piece pr create` existed.

## Solution

Add fallback detection via `gh pr list --head <branch> --state merged`.

## Implementation

### Files to Modify
- `internal/core/piece/handler.go` - Add new detection method in `IsBranchMerged`

### Detection Priority (updated)
1. PR metadata file → `gh pr view <number>`
2. `gh pr list --head <branch> --state merged` (NEW)
3. `git branch --merged`
4. Commit history check

### Function Change

```go
// After PR metadata check fails, before git branch check:
merged, err := h.checkPRByBranchName(repoRoot, branchName)
if err == nil && merged {
    status.IsMerged = true
    status.Method = "pr-branch"
    return status, nil
}
```

## Testing

- Test detection of squash-merged PR without metadata
- Test branch with no PR returns false
- Test open PR returns false
