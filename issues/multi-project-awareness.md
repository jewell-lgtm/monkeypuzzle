---
title: Multi-project awareness (registry, cross-repo views, project-aware tmux, dashboard)
status: done
---

# Multi-project awareness

Make `mp` aware of multiple repos/projects: a global registry of projects, cross-repo
listing of pieces and issues, project-namespaced tmux sessions (one session per worktree
per project, `mp/{project}/{piece}`), and a global Bubble Tea dashboard launched by bare
`mp`.

Plan file: `~/.claude/plans/i-want-monkeypuzzle-to-joyful-firefly.md`

## Phase 1 — project registry ✅

- `internal/registry` — `projects.json` in the user data dir (`MP_DATA_DIR` env override
  for isolation/tests); `Load`/`Save`, `Upsert` (dedupe by resolved repo root), `Find`,
  `Remove`; `ResolveRepoRoot`, `ProjectName`, `IsProject` helpers.
- `internal/core/project` — `Add`, `Remove`, `List` (with best-effort enrichment: branch,
  piece count, open-issue count).
- `cmd/mp/project.go` — `mp project add [path]`, `mp project rm <name|path>`,
  `mp project list` (table by default, `--json` / non-TTY → JSON). All support
  flags / stdin JSON / `--schema`.
- `mp init` auto-registers the repo (non-fatal if not in a git repo).
- Tests: `internal/registry/registry_test.go` (unit), `cmd/mp/project_integration_test.go`
  (`mp init` in two temp repos → `mp project list` shows both → `rm` works).

## Phase 2 — cross-repo views ✅

- `mp piece list --all` and `mp issue list --all` iterate registered projects, reusing the
  existing per-repo handlers, grouped output (text + JSON). Integration test:
  `cmd/mp/project_integration_test.go:TestPieceAndIssueListAll`.

## Phase 3 — project-aware tmux sessions ✅

- `internal/core/session` — `Name(project, piece)` → `mp/{project}/{piece}`,
  `MainName(project)` → `mp/{project}`, `Sanitize`.
- Piece handler resolves the project name from `.monkeypuzzle/monkeypuzzle.json`
  (`h.projectName`) and routes all session names through `h.pieceSessionName` /
  `h.mainSessionName`. Tests updated; migration note in `docs/workflow.md` (breaking
  rename from `mp-{repo}` / `mp-piece-{name}`). `internal/core/session/session_test.go`.

## Phase 4 — global dashboard & cross-project switch ✅

- `internal/tui/dashboard` — flat, always-expanded list of projects + piece worktrees;
  Enter returns the chosen row.
- `cmd/mp/dash.go` — `mp dash` and bare `mp`: interactive dashboard with a terminal,
  JSON (`{"projects":[...]}`) otherwise / with `--json`. Enter attaches the worktree's
  tmux session (or prints its path when no multiplexer is configured).
- `cmd/mp/switch.go` — `mp switch --project NAME [--piece NAME]` / stdin JSON / `--schema` /
  interactive picker; attaches the right session.
- Integration test: `cmd/mp/project_integration_test.go:TestDashboardAndSwitch`.

## Follow-up: merge any node of a stack ✅

- `mp piece merge --reparent-children` — merge a non-leaf piece (a stack base or a
  middle node). After the squash-merge, child pieces are re-homed onto the merge target
  and each direct child's `parent` metadata is re-pointed.
- Two re-home strategies via `--reparent-strategy`: `rebase` (default — `git rebase --onto
  <target> <merged-branch> <child>`, recursively, using captured pre-rebase tips so deeper
  descendants replant cleanly; rewrites history, needs force-push) or `merge` (merges the
  target into direct children, no history rewrite). `MergeResult.reparented_children` lists
  touched branches.
- Interactive `mp piece merge` on a piece with children opens a Bubble Tea chooser
  (`internal/tui/chooser`, a generic single-select list) to pick rebase / merge / cancel;
  non-interactive errors with the `--reparent-children` hint.
- Fixed a latent bug: `mp piece create --parent X` ran `git worktree add <path> <branch>`
  (fails because the parent branch is already checked out) — now `git worktree add -b
  <piece> <path> <parent-branch>`.
- Helpers: `adapters.Git.RebaseOnto`/`RebaseAbort`/`MergeAbort`;
  `Handler.rebaseSubtree`/`mergeIntoChild`/`ensureSubtreeClean`.
- Tests: `cmd/mp/project_integration_test.go:TestMergeStackBaseReparentsChildren`,
  `TestMergeStackBaseReparentMergeStrategy`.

## Known follow-ups (not done)

- Dashboard `n`/`i` keys to create a piece/issue in the selected project (currently
  read-only + attach).
- Optional configured scan roots for auto-discovering projects.
- Stale integration tests in `internal/core/piece` and `internal/core/pr` reference
  removed APIs (pre-existing; unrelated to this work).
