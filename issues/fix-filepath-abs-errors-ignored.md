---
title: Fix filepath.Abs errors being ignored
status: done
priority: medium
---

# Fix filepath.Abs errors being ignored

## Problem

Multiple locations discard `filepath.Abs` errors with blank identifier:

- `git.go:83` - `gitDir, _ = filepath.Abs(gitDir)`
- `git.go:92` - `absGitDir, _ := filepath.Abs(gitDir)`
- `git.go:104` - `repoRoot, _ = filepath.Abs(repoRoot)`
- `git.go:165` - `mainRepoRoot, _ = filepath.Abs(mainRepoRoot)`
- `exec.go:127` - `dir, _ = filepath.Abs(dir)` in MockExec
- `exec.go:149` - `dir, _ = filepath.Abs(dir)` in MockExec

## Impact

- filepath.Abs can fail with invalid characters or inaccessible cwd
- Silent failures could cause subtle bugs
- Inconsistent with error handling elsewhere

## Location

- `internal/adapters/git.go:83,92,104,165`
- `internal/adapters/exec.go:127,149`

## Fix

Either handle errors explicitly or document why safe to ignore:

```go
// Option 1: Handle error
absPath, err := filepath.Abs(path)
if err != nil {
    return "", fmt.Errorf("failed to get absolute path: %w", err)
}

// Option 2: Document why safe (if truly safe)
// filepath.Abs only fails if cwd is deleted mid-operation; accept risk
absPath, _ := filepath.Abs(path)
```
