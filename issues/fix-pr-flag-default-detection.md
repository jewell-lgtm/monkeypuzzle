---
title: Fix PR base flag default triggering mode detection
status: done
priority: high
---

# Fix PR base flag default triggering mode detection

## Problem

In `pr.go:98`, the condition checks if flags are provided:
```go
if flagPRTitle != "" || flagPRBody != "" || flagPRBase != "" {
```

But `flagPRBase` has a default value of "main" (pr.go:43), so this condition always triggers when base isn't explicitly set.

## Impact

- Users cannot pipe JSON to stdin for `mp piece pr create`
- Behavior contradicts other commands
- Breaks scripting workflows

## Location

- `cmd/mp/pr.go:98`

## Fix

Don't include defaulted flags in the "flags provided" check:

```go
// Only check non-defaulted flags
if flagPRTitle != "" || flagPRBody != "" {
```

Or track whether flag was explicitly set by user.
