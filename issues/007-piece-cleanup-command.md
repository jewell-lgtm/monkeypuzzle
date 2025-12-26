---
title: Implement `mp piece cleanup` command
status: todo
---

# Implement `mp piece cleanup` command

## Description

Create a command to automatically detect and cleanup merged pieces. Removes worktrees, kills tmux sessions, and updates issue status to `done` for pieces whose branches have been merged.

## Requirements

- Command: `mp piece cleanup` (run from repo root or piece)
- List all pieces in pieces directory
- Check each piece's branch merge status
- For merged pieces:
  - Remove git worktree
  - Kill associated tmux session
  - Update linked issue status to `done` (if exists)
  - Clean up piece directory
- Support flags: `--dry-run`, `--force`
- Output list of cleaned pieces

## Implementation Details

### Files to Create/Modify
- `cmd/mp/piece.go` - Add cleanup subcommand
- `internal/core/piece/handler.go` - Add `CleanupMergedPieces` function
- `internal/core/piece/handler.go` - Add `RemovePiece` helper

### Command Structure
```bash
mp piece cleanup              # Cleanup all merged pieces
mp piece cleanup --dry-run    # Show what would be cleaned
mp piece cleanup --force      # Skip confirmation prompts
```

### Implementation Flow

1. Get pieces directory path
2. List all piece directories
3. For each piece:
   - Check if branch is merged (use detection from issue 05)
   - If merged:
     - Read PR metadata (if exists)
     - Read issue marker (if exists)
     - Remove worktree
     - Kill tmux session
     - Update issue status to `done`
     - Log cleanup action
4. Output summary

### Safety Checks
- Verify we're in correct repo before cleanup
- Confirm piece branch is actually merged
- Handle pieces without PR metadata gracefully
- Handle pieces without issue markers gracefully

## Testing
- Test cleanup of merged piece
- Test dry-run mode
- Test handling of unmerged pieces (skip)
- Test cleanup with issue marker update
- Test cleanup without issue marker
- Test error handling (permissions, locked files)

