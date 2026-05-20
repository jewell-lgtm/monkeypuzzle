---
title: "Unified mp switch picker: pieces, issues, and branches"
status: in-progress
description: "Extend mp switch with a fuzzy-filtered cross-project picker covering existing pieces, open todo issues (create-from-issue + attach), and unadopted local branches (adopt + attach). Non-interactive --issue and --branch flags + JSON stdin equivalents."
---

# Unified mp switch picker: pieces, issues, and branches

## Goal

Make `mp switch` the single entry point for starting or resuming work — from existing pieces, open todo issues, or stray local branches — so the user no longer needs to check out branches in the main worktree.

## What shipped

### Row model — `internal/tui/dashboard/dashboard.go`
- New `RowKind`s: `RowIssue`, `RowBranch` (plus existing `RowProject`, `RowPiece`).
- New `Row` fields: `IssuePath`, `IssueTitle`, `IssueNumber`, plus reuse of `Branch`.
- Fuzzy filter via `textinput.Model`, matching against project/piece/branch/issue title/path/number.
- Visible rows capped at `MaxVisibleRows = 20` with a `… N more` footer.
- Selection bounded to the visible slice.

### Shared fuzzy matcher — `pkg/fuzzy/fuzzy.go`
- Lifted `fuzzyMatch` out of `internal/tui/issuepicker` and `internal/core/issue/markdown_provider.go` into a shared package. Both call sites now use `fuzzy.Match`.

### Dispatch — `cmd/mp/switch.go`
- New flags: `--issue`, `--branch` (and unchanged `--project`, `--piece`).
- JSON stdin gains `issue` and `branch` fields; `--schema` reflects them.
- Mutual exclusion enforced for `--piece`/`--issue`/`--branch`.
- Dispatch by row kind: `RowProject`/`RowPiece` attach; `RowIssue` runs `CreatePieceWithInput` then attaches; `RowBranch` runs `AdoptPiece` then attaches.

### Handler refactor — `internal/core/piece/handler.go`, `input.go`
- `AdoptPieceInput` gains `RepoRoot`. When set, `AdoptPiece` skips the cwd-based repo discovery and uses the provided path directly. Lets `mp switch` adopt a branch in a project other than cwd.

### Adapter — `internal/adapters/git.go`
- `Git.ListLocalBranches` — `git for-each-ref refs/heads --format=%(refname:short)`.
- `Git.CheckedOutBranches` — parses `git worktree list --porcelain` so the picker can exclude branches already checked out in any worktree (main or piece).

### Data collection — `cmd/mp/dash.go`
- `collectDashboard` now gathers per project: open todo issues (capped at 10) and local branches (capped at 10), skipping main/master and any branch checked out in a worktree.
- Issue rows surface only `todo` issues (creating a piece transitions issue status to in-progress, so claimed issues fall out automatically).
- `dashProject` JSON shape extended with `issues` and `branches` arrays.

### Docs
- README command table.
- `docs/commands.md` — new `mp switch` section.
- `docs/workflow.md` — updated multi-piece section.
- `internal/core/claude/handler.go` — agent-facing help.
- `.claude/skills/managing-monkeypuzzle/SKILL.md` and `.claude/skills/monkeypuzzle/SKILL.md`.

## Tests
- Integration: `cmd/mp/switch_unified_integration_test.go`
  - `TestSwitchUnified_FromIssue_CreatesPieceAndAttaches`
  - `TestSwitchUnified_FromBranch_AdoptsAndAttaches`
  - `TestSwitchUnified_DashJSON_IncludesIssuesAndBranches`
  - `TestSwitchUnified_MutuallyExclusiveSelectors`
- Unit: `internal/tui/dashboard/dashboard_test.go` (filter, max-length truncation, selection bounds, esc/cancel).
- Unit: `pkg/fuzzy/fuzzy_test.go`.

## Out of scope (deferred)
- Status-filter toggle in interactive picker (issues default to `todo`).
- Distinct grouping/headers between row kinds inside the picker — currently a single flat fuzzy list.
- `CreatePiece` / `CreatePieceFromIssue` taking an explicit `RepoRoot` — switch currently uses a scoped `tempChdir` for those calls since they still read cwd.
