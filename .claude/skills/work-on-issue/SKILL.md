---
name: work-on-issue
description: Complete workflow for working on an issue from start to PR. Use when starting work on an issue, implementing features, or fixing bugs. Follows outside-in testing methodology.
triggers:
  - "work on issue"
  - "start issue"
  - "implement issue"
  - "pick up issue"
---

# Work on Issue

Complete workflow for implementing an issue from start to PR.

## Workflow Steps

### 1. Select and Start Issue

```bash
# List available issues
mp issue list --status todo

# Create piece from issue (get worktree path)
mp piece new --issue <issue-path> --skip-switch

# Navigate to worktree
cd <worktree-path>
```

The issue status automatically updates to `in-progress`.

### 2. Read and Understand Issue

```bash
# Read the issue file
cat <issue-path>
```

Before coding:
- Identify acceptance criteria
- Note any dependencies
- Clarify ambiguous requirements (ask user if needed)

### 3. Outside-In Development

**Start with integration test for happy path:**

```bash
# Create failing integration test first
# Test should cover the main use case end-to-end
```

**Then implement to make it pass:**
- Write minimal code to pass the test
- Keep implementation simple

**Add unit tests for edge cases:**
- Error handling
- Boundary conditions
- Invalid inputs

**Run tests frequently:**
```bash
go test ./...
go vet ./...
```

### 4. Update Issue Progress

Keep the issue file updated as work progresses:

```markdown
## Progress

- [x] Integration test for happy path
- [x] Core implementation
- [ ] Unit tests for edge cases
- [ ] Code review
```

### 5. Self Code Review

Before creating PR, review your changes:

```bash
# See all changes
git diff

# Check for common issues:
# - Unused imports/variables
# - Missing error handling
# - Hardcoded values that should be configurable
# - Missing tests for new code paths
# - Security issues (injection, secrets, etc.)
```

**Review checklist:**
- [ ] Tests pass
- [ ] No lint errors
- [ ] Error cases handled
- [ ] No secrets/credentials committed
- [ ] Code is readable and minimal

### 6. Commit Changes

```bash
# Stage and commit with descriptive message
git add -A
git commit -m "feat: <description>

- <bullet points of changes>

Closes #<issue-number>"
```

### 7. Create Pull Request

```bash
# From the piece worktree
mp piece pr create --title "<PR title>" --body "<description>"
```

**PR body should include:**
- Summary of changes
- Test plan
- Link to issue

### 8. After PR Merge

```bash
# Cleanup merged pieces
mp piece cleanup --force
```

Issue status automatically updates to `done`.

## Quick Reference

```bash
# Full workflow
mp issue list --status todo                           # Find issue
mp piece new --issue issues/foo.md --skip-switch      # Start work
cd $(cat /dev/stdin | jq -r .worktree_path)           # Enter worktree

# ... do work with outside-in testing ...

git add -A && git commit -m "feat: implement foo"     # Commit
mp piece pr create --title "Implement foo"            # Create PR

# After merge
mp piece cleanup --force                              # Cleanup
```

## Outside-In Testing Pattern

```
1. Write integration test (fails)
   └── Test the feature end-to-end

2. Write minimal implementation (test passes)
   └── Just enough to make it work

3. Write unit tests for edge cases
   └── Error handling, boundaries, invalid input

4. Refactor if needed (tests still pass)
   └── Clean up, extract functions, improve naming
```

## Issue File Format

Issues should have YAML frontmatter:

```markdown
---
title: Feature title
status: todo|in-progress|done
---

# Feature title

## Description
What needs to be done.

## Acceptance Criteria
- [ ] Criterion 1
- [ ] Criterion 2

## Progress
- [ ] Task 1
- [ ] Task 2
```

## When to Ask User

Stop and ask if:
- Requirements are ambiguous
- Multiple valid approaches exist
- Breaking changes are needed
- Security concerns arise
- Scope seems larger than expected
