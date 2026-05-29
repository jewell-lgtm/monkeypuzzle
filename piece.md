---
title: cant-create-piece
status: in-progress
---

# cant-create-piece

`mp piece adopt` fails when adopting a branch that is currently checked out in
the main repo.

## Symptom

```
$ mp piece adopt
Error: failed to create worktree for branch feat/inspection-report-pipeline: ...
exit status 128
```

## Root cause

Git refuses to create a worktree for a branch that is already checked out in
another worktree. The most common adopt flow is "I've been working on a branch
directly in the main checkout and now want to turn it into a piece" — but in
that case the branch is checked out in the main repo, so `git worktree add`
fails with `exit status 128` and the user only sees the raw git error.

A previous change had deliberately removed the auto-checkout-main behaviour and
left the user to "sort it out", which produced this cryptic failure.

## Fix

Better messaging — do **not** silently move the user's checkout. In
`Handler.AdoptPiece` (local-branch path), before creating the worktree, detect
when the branch is already checked out in another worktree (via
`Git.CheckedOutBranches`) and return a clear, actionable error instead of the
bare `exit status 128`:

- If it is the main repo's current branch: name the branch and repo, and suggest
  `git -C <repo> checkout main` before re-running adopt.
- If it is checked out in another worktree: explain a branch can only live in one
  worktree at a time.

Added `Handler.ensureBranchAdoptable` helper. The main repo's checkout is left
untouched.

## Tests

- `TestIntegration_AdoptPiece_BranchCheckedOutInMainRepo` — tries to adopt a
  branch while it is still HEAD of the main repo; asserts a friendly error (no
  `exit status 128`, names the branch + repo), that the main repo is left on the
  feature branch, and that no stray worktree was created.
- Existing adopt integration tests still pass.
