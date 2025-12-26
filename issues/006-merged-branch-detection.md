---
title: Detect merged piece branches
status: todo
---

# Detect merged piece branches

## Description

Implement functionality to detect when a piece's branch has been merged to main via GitHub PR. This enables automatic cleanup of merged pieces.

## Requirements

- Check if piece branch exists on remote
- Check if branch has been merged to main branch
- Support multiple detection methods:
  - Via `gh pr view` - check PR status (MERGED)
  - Via `git branch --merged` - check if branch is merged locally
  - Via `git log` - check if branch commits are in main
- Handle both local merges and remote PR merges

## Implementation Details

### Files to Create/Modify
- `internal/core/piece/handler.go` - Add `IsBranchMerged` function
- `internal/adapters/git.go` - Add merged branch checking methods

### Detection Methods (Priority Order)

1. **GitHub PR Status** (most reliable)
   - Read PR metadata from piece worktree
   - Run `gh pr view <number> --json state,merged`
   - Return true if state is "MERGED"

2. **Git Branch Merged Check**
   - Run `git branch --merged main` in repo root
   - Check if piece branch appears in output
   - Also check remote: `git branch -r --merged origin/main`

3. **Commit History Check** (fallback)
   - Check if piece branch HEAD commit is in main history
   - Use `git log --oneline main | grep <commit-hash>`

### Function Signature
```go
func (h *Handler) IsBranchMerged(repoRoot, branchName, mainBranch string) (bool, error)
```

## Testing
- Test merged branch detection via PR
- Test merged branch detection via git
- Test unmerged branches return false
- Test error handling when PR doesn't exist
- Test detection from piece worktree vs repo root

