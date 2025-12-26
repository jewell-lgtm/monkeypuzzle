---
title: Design and implement PR metadata storage
status: todo
---

# Design and implement PR metadata storage

## Description

Design a metadata storage system to track PR information in piece worktrees. This enables linking pieces to PRs and issues, and supports merged branch detection.

## Requirements

- Store PR metadata when PR is created (see issue 04)
- Store in piece worktree (not main repo)
- Link PR to issue if piece created from issue
- Include PR number, URL, branch name, creation time
- Easy to read/update from piece or repo root context

## Implementation Details

### Metadata File Location
`.monkeypuzzle/pr-metadata.json` in piece worktree root

### Metadata Structure
```json
{
  "pr_number": 123,
  "pr_url": "https://github.com/owner/repo/pull/123",
  "branch": "piece-feature-name",
  "created_at": "2024-01-27T10:00:00Z",
  "issue_path": ".monkeypuzzle/issues/feature.md",
  "base_branch": "main"
}
```

### Functions Needed
- `ReadPRMetadata(worktreePath string, fs core.FS) (*PRMetadata, error)`
- `WritePRMetadata(worktreePath string, metadata PRMetadata, fs core.FS) error`
- `UpdatePRMetadata(worktreePath string, updates map[string]interface{}, fs core.FS) error`

### Integration Points
- Write metadata when PR created (`mp piece pr create`)
- Read metadata for merged detection (`mp piece cleanup`)
- Read metadata for issue status updates

### Files to Create/Modify
- `internal/core/piece/pr_metadata.go` - PR metadata struct and functions
- Update `internal/core/pr/handler.go` - Write metadata on PR creation
- Update `internal/core/piece/handler.go` - Read metadata for cleanup

## Testing
- Test writing PR metadata
- Test reading PR metadata
- Test updating PR metadata
- Test metadata with and without issue link
- Test handling missing metadata file gracefully

