---
title: "Observe GitHub PR events for workflow transitions"
status: todo
description: "Follow-up to the workflow-engine RFC: implement automatic observation of pr.opened.ready / pr.checks.green / pr.preview.ready / pr.closed_unmerged so workflows beyond the default (e.g. pima-one) can transition without manual fires."
---

# Observe GitHub PR events for workflow transitions

## Context

The workflow engine ships with the full event vocabulary defined (see
`internal/core/workflow/workflow.go`), but only two events are automatically
observed:

- `branch.created` — fired by `mp piece create`.
- `pr.merged` — fired by `mp piece merge`.

The default workflow uses only those two, so it works end-to-end. Custom
workflows (pima-one's PIMAO, others) need the rest:

- `pr.opened.draft` — `mp piece pr create --draft`
- `pr.opened.ready` — PR went non-draft (or was created non-draft)
- `pr.checks.green` — required GitHub check runs all passed
- `pr.preview.ready` — a configured preview deployment is healthy
- `pr.closed_unmerged` — PR closed without merging (does NOT auto-cancel;
  see RFC §4.3)

## Scope

1. Extend `internal/core/pr/` (GitHub provider) with a method that, given
   a PR ref or piece's piece-metadata `created_from_branch`, returns the
   current set of observed events: PR draft/ready, latest required-checks
   verdict, latest preview-deployment verdict, closed/open state.
2. Add `mp issue sync` (and `mp piece status --reconcile`) that, for every
   piece linked to an issue, polls the PR provider and fires any new
   events through the workflow engine.
3. Wire `mp piece pr create` to fire `pr.opened.draft` or `pr.opened.ready`
   inline (don't wait for the next poll).
4. Decide where preview-deployment matching lives. Probably a
   per-project config block under `pr.config`:
   `{"preview_environment": "preview"}` for GitHub deployments, or
   provider-specific keys for Vercel/Netlify.

## Out of scope

- Backward regression: if checks were green and went red again, the
  workflow does not move the issue backward. The RFC defers this.
- Multi-PR-per-issue: the engine assumes one PR per issue. The RFC
  proposes "any-PR-merged" as the trigger but defers per-PR tracking.

## Failure modes to cover

- GitHub API rate-limit. Sync should degrade gracefully.
- A PR that was merged via squash from a different branch (so `pr.merged`
  fires but `branch.created` was never recorded). Engine should fire
  `pr.merged` from whatever state the issue is in.
- A closed-unmerged PR. Per the RFC, this is **silent** — record the
  event but do not transition. The user fires `mp issue abandon` if the
  ticket should be killed.
