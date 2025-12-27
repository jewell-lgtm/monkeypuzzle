---
title: Stack-aware PR creation
status: todo
---

# Stack-aware PR creation

## Description

Modify `mp piece pr create` to use parent piece's branch as PR base when piece has a non-main parent.

## Requirements

- Read parent from piece metadata
- Set PR base to parent's branch (not main)
- Include stack info in PR description
- Support `--base` override

## Implementation Details

### PR Base Logic

```
if parent == "main":
    base = main (current behavior)
else:
    base = parent's branch name
```

### Files Modified

- `internal/core/pr/handler.go` - Read parent metadata, set base, add stack info to body
- `internal/core/pr/handler_test.go` - Tests for stack-aware PR creation

### PR Description Enhancement

Auto-appends stack context:
```markdown
---
Part of stack:
- parent-piece-branch (#PR_NUMBER)
  - **this-piece** (this PR)
```

### Command Behavior

```bash
$ mp piece pr create
# Detects parent=piece-123, creates PR with base=piece-123-branch

$ mp piece pr create --base main
# Override: create PR against main regardless of parent
```

### Edge Cases

- Parent piece has no PR yet: warn user, suggest creating parent PR first
- Parent is main: use main as base (default behavior)
- Explicit --base override: respect override, don't auto-detect parent

## Testing

- Test PR with parent piece as base
- Test PR with main as base (root piece)
- Test `--base` flag override
- Test stack info in PR body
- Test warning when parent has no PR

