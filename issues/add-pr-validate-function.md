---
title: Add actual validation to PR Validate function
status: done
priority: low
---

# Add actual validation to PR Validate function

## Problem

`Validate()` in `pr/input.go:88-91` is a no-op:

```go
func Validate(input Input) error {
    // All fields are optional - title can be derived from issue
    return nil
}
```

No validation of format/content even when values are provided.

## Impact

- Invalid titles/bodies pass through to GitHub API
- No early validation feedback to user
- Inconsistent with issue package validation

## Location

- `internal/core/pr/input.go:88-91`

## Fix

Add format validation:

```go
func Validate(input Input) error {
    if input.Title != "" && len(input.Title) > 256 {
        return fmt.Errorf("title too long (max 256 chars)")
    }
    if input.Base != "" && !isValidBranchName(input.Base) {
        return fmt.Errorf("invalid base branch name: %s", input.Base)
    }
    return nil
}
```
