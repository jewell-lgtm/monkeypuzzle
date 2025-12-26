---
title: Implement `mp issue create` command
status: done
---

# Implement `mp issue create` command

## Description

Create a new command `mp issue create` that allows users to create markdown issue files with proper YAML frontmatter in the configured issues directory.

## Requirements

- Command: `mp issue create`
- Support all input modes (interactive, flags, stdin JSON, schema)
- Create markdown file in `.monkeypuzzle/issues/` (or configured directory)
- Generate filename from title (sanitized, kebab-case)
- Include YAML frontmatter with:
  - `title`: Issue title
  - `description`: Issue description (optional)
  - `status`: Default to `todo`
- Create file with proper permissions (0644)

## Implementation Details

### Files to Create
- `cmd/mp/issue.go` - CLI command wrapper
- `internal/core/issue/input.go` - Input validation and schema
- `internal/core/issue/handler.go` - Business logic

### Command Structure
```bash
mp issue create --title "Add feature X" --description "Description here"
mp issue create --schema  # Output JSON schema
echo '{"title":"..."}' | mp issue create  # JSON stdin
mp issue create  # Interactive TUI
```

### JSON Schema
```json
{
  "title": "Issue title (required)",
  "description": "Optional description"
}
```

### Output File Format
```markdown
---
title: Issue Title
status: todo
description: Optional description
---

# Issue Title

Optional description content here
```

## Testing
- Unit tests for handler
- Integration tests for CLI command
- Test filename sanitization
- Test duplicate filename handling

