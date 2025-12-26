---
title: Implement `mp piece pr create` command
status: todo
---

# Implement `mp piece pr create` command

## Description

Create a command to push the piece branch to origin and create a GitHub PR using `gh pr create`. Store PR number and URL in piece metadata for tracking.

## Requirements

- Command: `mp piece pr create` (must be run from within piece worktree)
- Push current piece branch to origin
- Create PR using `gh pr create`
- Extract PR number and URL from gh output
- Store PR metadata in piece worktree
- Link PR to issue if piece was created from issue
- Support flags: `--title`, `--body`, `--base` (default: main)

## Implementation Details

### Files to Create
- `cmd/mp/pr.go` - CLI command wrapper
- `internal/core/pr/input.go` - Input validation
- `internal/core/pr/handler.go` - Business logic
- `internal/adapters/github.go` - GitHub API via gh CLI

### Command Structure
```bash
mp piece pr create --title "PR Title" --body "PR description"
mp piece pr create  # Use issue title/description if available
```

### PR Metadata Storage
Store in `.monkeypuzzle/pr-metadata.json` in piece worktree:
```json
{
  "pr_number": 123,
  "pr_url": "https://github.com/owner/repo/pull/123",
  "branch": "piece-branch-name",
  "created_at": "2024-01-27T10:00:00Z"
}
```

### GitHub Adapter
Use `gh pr create` command via Exec adapter:
- `gh pr create --title "..." --body "..." --base main`
- Parse output to extract PR number and URL
- Handle errors (branch not pushed, gh not authenticated, etc.)

## Testing
- Unit tests for PR creation logic
- Mock gh CLI output parsing
- Test error handling (no remote, gh not installed, auth issues)
- Integration test with real gh CLI (optional)

