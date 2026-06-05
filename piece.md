---
title: detect-piece
status: in-progress
---

# detect-piece

Detect the current project when running `mp` inside a repo.

## Problem

Running bare `mp` (the dashboard) inside a repo shows pieces/issues/branches for
*every* registered project (e.g. both `monkeypuzzle` and `pima-one`). When run
inside a repo it should only show that repo's project.

## Behaviour

- Bare `mp` / `mp dash`: when the cwd resolves to a registered project (works
  from the main repo *or* from any piece worktree), scope the dashboard to just
  that project.
- `mp dash --all`: show every registered project (the previous behaviour).
- When the cwd is not inside any registered project (e.g. run from `$HOME`),
  fall back to showing all projects so the cross-project view is still
  discoverable.
- `mp switch` is unchanged — it is deliberately the cross-project picker.

This mirrors the existing `mp piece list` / `mp piece list --all` convention.

## Implementation

- `cmd/mp/dash.go`: add `--all` flag; `collectDashboard` takes an `onlyPath`
  filter; new `currentProjectPath()` resolves cwd (via
  `projectdir.MainRepoRoot`, which handles worktrees) to a registered project.
- `cmd/mp/switch.go`: pass `""` (all projects) to `collectDashboard`.

## Tests

- Integration: `mp` from a repo shows only that repo; from a piece worktree
  shows only the owning repo; `--all` shows everything; from outside any repo
  falls back to all.
