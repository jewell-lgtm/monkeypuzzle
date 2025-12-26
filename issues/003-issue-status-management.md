---
title: Add issue status field support
status: todo
---

# Add issue status field support

## Description

Implement parsing and updating of issue status field in markdown issue files. Status should be stored in YAML frontmatter and support values: `todo`, `in-progress`, `done`.

## Requirements

- Parse `status` field from YAML frontmatter
- Support status values: `todo`, `in-progress`, `done`
- Default to `todo` if status not specified
- Update status field in issue file
- Preserve other frontmatter fields when updating

## Implementation Details

### Files to Modify
- `internal/core/piece/issue.go` - Add status parsing functions
- Create `internal/core/issue/status.go` - Status management functions

### Functions Needed
- `ParseIssueStatus(issuePath string, fs core.FS) (string, error)` - Read status from frontmatter
- `UpdateIssueStatus(issuePath string, status string, fs core.FS) error` - Update status in frontmatter
- `ValidateStatus(status string) bool` - Validate status value

### Status Values
- `todo` - Issue not yet started (default)
- `in-progress` - Issue is currently being worked on
- `done` - Issue is completed

## Testing
- Test parsing existing status
- Test default status when missing
- Test updating status preserves other frontmatter
- Test invalid status values
- Test status in various frontmatter formats

