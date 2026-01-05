---
title: Add test coverage for adapters
status: todo
priority: medium
---

# Add test coverage for adapters

## Problem

Critical adapters have 0% test coverage:
- `filesystem.go` - OSFS and MemoryFS completely untested
- `output.go` - All Output implementations untested
- `exec.go` - OSExec methods untested
- `git.go` - 13 of 28 methods untested (WorktreeAddFrom, Merge, Checkout, etc.)

Overall adapter coverage: ~28%

## Impact

- Path traversal protection logic untested (filesystem.go:30-65)
- Output formatting bugs could slip through
- Git operations may have edge case bugs

## Locations

- `internal/adapters/filesystem.go` - needs filesystem_test.go
- `internal/adapters/output.go` - needs output_test.go
- `internal/adapters/git.go` - expand git_test.go
- `internal/adapters/exec.go` - expand exec_test.go

## Priority Tests

1. OSFS.path() - path traversal attack prevention (SECURITY)
2. MemoryFS operations - used extensively in tests
3. TextOutput/JSONOutput.Write() - output formatting
4. Git.WorktreeAddFrom() - new feature
5. Git.Merge(), Git.MergeSquash() - critical operations
