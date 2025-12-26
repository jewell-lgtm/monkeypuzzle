---
title: Complete Issue-to-PR-to-Merge Workflow
status: todo
---

# Complete Issue-to-PR-to-Merge Workflow

## Overview

This document describes the complete workflow from creating markdown issues through PR creation to automatic piece cleanup when PRs are merged.

## Current State

### ✅ Implemented
- `mp init` - Initialize project with markdown issues provider
- `mp piece new --issue <path>` - Create piece from issue file
- `mp piece status` - Show current piece status
- `mp piece update` - Merge main into piece
- `mp piece merge` - Merge piece back to main locally
- Issue name extraction from markdown
- Current issue marker tracking in worktree

### ❌ Missing (Issues Created)
1. **Issue Creation** - `mp issue create` command (issue 01)
2. **Issue Status Management** - Status field parsing/updating (issue 02)
3. **Status Update on Piece Create** - Auto-update to in-progress (issue 03)
4. **PR Creation** - `mp piece pr create` command (issue 04)
5. **PR Metadata Storage** - Track PR info in piece (issue 07)
6. **Merged Detection** - Detect merged branches (issue 05)
7. **Piece Cleanup** - `mp piece cleanup` command (issue 06)
8. **Status Update on Merge** - Auto-update issue to done (issue 08)

## Proposed Complete Workflow

```
1. mp issue create --title "Feature X"
   → Creates .monkeypuzzle/issues/feature-x.md (status: todo)

2. mp piece new --issue .monkeypuzzle/issues/feature-x.md
   → Creates piece worktree
   → Updates issue status: todo → in-progress
   → Creates .monkeypuzzle/current-issue.json

3. [Work on feature, make commits]

4. mp piece pr create
   → Pushes branch to origin
   → Creates GitHub PR
   → Stores PR metadata in .monkeypuzzle/pr-metadata.json

5. [PR Review on GitHub - external]

6. [PR Merged on GitHub - external]

7. mp piece cleanup
   → Detects merged piece branches
   → Removes worktrees for merged pieces
   → Kills tmux sessions
   → Updates issue status: in-progress → done
```

## Implementation Order

Recommended implementation order based on dependencies:

1. **Issue 01** - Issue creation (foundation)
2. **Issue 02** - Issue status management (foundation)
3. **Issue 07** - PR metadata storage (foundation for PR tracking)
4. **Issue 04** - PR creation (uses metadata storage)
5. **Issue 03** - Status update on piece create (uses status management)
6. **Issue 05** - Merged detection (uses PR metadata)
7. **Issue 06** - Piece cleanup (uses merged detection)
8. **Issue 08** - Status update on merge (uses cleanup and status management)

## Related Files

- `internal/core/piece/issue.go` - Issue parsing functions
- `internal/core/piece/handler.go` - Piece operations
- `cmd/mp/piece.go` - Piece CLI commands
- `internal/core/init/handler.go` - Config structure

