# TODOS

## P2 — Define "agent output" mechanically
- **What:** Specify how mp distinguishes agent-created PRs/branches from human ones (candidates: lifecycle hooks stamp pieces as agent-created; PR label fallback for branches created outside mp; branch-naming convention as last resort).
- **Why:** Every follow-on vessel (landing queue, `mp land` TUI, GitHub bot) must filter agent output; today nothing defines the marker. Surfaced by cross-model review of the Evidence Sprint plan (2026-07-01).
- **Pros:** Unblocks any GO-verdict build on day 1; sharpens demo narrative.
- **Cons:** Mooted if the sprint verdict is ICE.
- **Context:** Sprint plan: `.DONOTCOMMIT/evidence-sprint-plan.md`; design doc: `~/.gstack/projects/jewell-lgtm-monkeypuzzle/mattjewell-main-design-20260701-225142.md`. `internal/core/piece` owns piece lifecycle; hooks fire at transitions (see apps/mp README).
- **Effort:** M (human) → S (CC). **Priority:** P2. **Blocked by:** sprint go/no-go (2026-07-31).

## P3 — Cross-box `mp agent focus`
- **What:** `mp agent list/focus/summary` and tmux `m a`/`m b` reach agents on placed pieces (ssh -t + select-pane).
- **Why:** Cut from remote-boxes plan (D8, 2026-08-20) to ship `mp go` first. Plan: `.DONOTCOMMIT/cloud-box-plan.md`.
- **Effort:** S (CC). **Blocked by:** Phase 3 of that plan.

## P3 — Dashboard enumerates placed pieces
- **What:** mp-server lists pieces on boxes (today: remote projects listed, pieces not).
- **Why:** Known limit carried from remote-development.md; unchanged by remote-boxes plan.
- **Effort:** M. **Blocked by:** remote-boxes Phase 3.

## P3 — Placement follow-ups from the 2026-09-09 review (`.DONOTCOMMIT/placement-review.md` items 5, 8, 11)
- **What:** (5) interrupted first clone leaves a partial box repo trusted forever; pending link with no hidden row invisible to `mp remote doctor` → doctor should scan local placements.json. (8) `proxyPlaced` should run `cli.ValidSSHDest` on the box read from placements.json. (11) `mp go --json` lists placed rows without `host` (tmux picker treats them as local); mp-mcp `mp_piece_create` lacks `remote`; flatten's prompt counts placed pieces it skips; `runPieceListAll` emits an error row per hidden `<project>@<box>`.
- **Why:** not blockers; cut so the stack (#68-72 + placement-fixes) can land.
- **Effort:** S each. **Blocked by:** placement stack merge.

## P3 — `mp merge` into a parent *piece* fails: "failed to checkout <parent>"
- **What:** merging a middle piece of a stack into its parent piece fails because the parent branch is checked out in the parent's worktree; only merges into main work.
- **Why:** found while reproducing the orphaned-children bug (2026-09-09, PR "stack-bugs"). Fix: perform the squash merge inside the parent's worktree (or via a temporary worktree) instead of checking the branch out in the main repo.
- **Effort:** S. **Blocked by:** nothing.
