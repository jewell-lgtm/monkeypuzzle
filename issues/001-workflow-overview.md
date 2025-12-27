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
- `mp issue create` - Create markdown issue files
- `mp piece new --issue <path>` - Create piece from issue, update status to in-progress
- `mp piece status` - Show current piece status
- `mp piece update` - Merge main into piece
- `mp piece merge` - Merge piece back to main locally
- `mp piece pr create` - Push and create GitHub PR
- `mp piece list` - List all active pieces
- `mp piece cleanup` - Remove merged pieces, update issue to done
- Issue name/status extraction from markdown
- PR metadata storage in piece worktree
- MCP server exposing core commands
- Claude Code skill documentation

### ❌ Missing
1. **Stack-aware Merge** - Child pieces merge to parent (issue 013)
2. **Stack-aware PR** - PR base = parent branch (issue 014)
3. **Issue Storage Interface** - Pluggable backends (abstract)

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

## Next Steps

Remaining implementation in priority order:

1. **Issue 013** - Stack-aware piece merging (core workflow enhancement)
2. **Issue 014** - Stack-aware PR creation (depends on 013)
3. **Abstract issue storage** - Lower priority refactor for pluggable backends

## Related Files

- `internal/core/piece/issue.go` - Issue parsing functions
- `internal/core/piece/handler.go` - Piece operations
- `cmd/mp/piece.go` - Piece CLI commands
- `internal/core/init/handler.go` - Config structure

