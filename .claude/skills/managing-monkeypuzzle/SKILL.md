---
name: managing-monkeypuzzle
description: Drives the mp CLI — worktree-per-piece git-flow with lifecycle hooks. Use when working in a .monkeypuzzle project: create pieces, switch between them, open draft PRs/MRs, flip them ready, merge and clean up.
---

# mp CLI

All commands accept JSON stdin (`echo '{...}' | mp <cmd>`) and emit JSON to stdout.
Use `mp <cmd> --schema` to see the expected input shape.

## Pieces (worktree + multiplexer session)

```bash
# Show current piece status (no piece = main repo)
mp status

# Create a new piece
mp create --name my-feature --skip-switch
echo '{"name":"my-feature","skip_switch":true}' | mp create
echo '{"prompt":"add dark mode"}' | mp create

# Tree or flat list of pieces (--all = across registered projects)
mp list
echo '{"flat":true}' | mp list

# Switch to anything by name: a piece, a branch (adopted on the fly), or —
# with "create":true — a brand-new piece. Project defaults to the current repo.
echo '{"target":"my-feature"}' | mp switch
echo '{"target":"feat/new-thing","create":true}' | mp switch
mp switch feat/new-thing --create

# Sync piece with its parent (prefers origin/<parent>; --local for local branch)
echo '{}' | mp sync

# Sync piece with main
echo '{}' | mp update

# Merge piece back to main
echo '{}' | mp merge

# Bring an existing branch into mp's worktree management
echo '{}' | mp adopt
echo '{"name":"custom","parent":"main"}' | mp adopt

# After merge: clean up worktree + multiplexer session
echo '{}' | mp done

# Sweep all merged pieces
echo '{"dry_run":true}' | mp cleanup
echo '{"force":true}' | mp cleanup

# Discard an unmerged piece
echo '{"force":true,"delete_branch":true}' | mp abandon
```

## Stacks (git-town-style stacked branches)

```bash
mp stack status                      # tree + PR state + drift vs the forge
mp stack sync                        # propagate main + parents down the stack (snapshots first)
mp stack sync --strategy rebase --push
mp stack append --name <child>       # new piece on top of the current one
mp stack prepend --name <between>    # insert between current piece and its parent
mp stack set-parent --parent <piece|main>  # re-parent the current piece; sync restacks
mp stack continue                    # resume after resolving a rebase conflict
mp stack undo                        # restore every branch to the pre-sync snapshot
```

## PRs / MRs (forge-agnostic via configured pr_provider)

```bash
# Open a PR/MR for the current piece
echo '{}' | mp pr create
echo '{"draft":true,"title":"WIP: ..."}' | mp pr create

# Flip a draft to ready-for-review (always a separate step — never auto)
mp pr ready
```

Draft creation fires `before-pr-create.sh` / `after-pr-create.sh`.
Ready-flip fires `before-pr-ready.sh` / `after-pr-ready.sh`.
Both pass `MP_PR_NUMBER`, `MP_PR_URL`, `MP_PR_BASE_BRANCH` in env.

## Hooks

Drop executable scripts in `.monkeypuzzle/hooks/`:

| Hook | Fires | Env beyond piece basics |
| --- | --- | --- |
| `on-piece-create.sh` | after worktree+session ready | `MP_SESSION_NAME` |
| `before-piece-update.sh` / `after-piece-update.sh` | around `mp update` / `mp sync` | `MP_MAIN_BRANCH` |
| `before-piece-merge.sh` / `after-piece-merge.sh` | around `mp merge` | `MP_MAIN_BRANCH` |
| `before-pr-create.sh` / `after-pr-create.sh` | around `mp pr create` | `MP_PR_NUMBER`, `MP_PR_URL`, `MP_PR_BASE_BRANCH` |
| `before-pr-ready.sh` / `after-pr-ready.sh` | around `mp pr ready` | same as PR create |
| `is-piece-done.sh` | consulted first by `IsBranchMerged` | exit 0 = merged |

Piece basics always set: `MP_PIECE_NAME`, `MP_WORKTREE_PATH`, `MP_REPO_ROOT`.

Non-zero exit aborts the calling operation, except for `after-*` hooks which warn but don't fail (the side-effect already happened).

## Init & Config

```bash
# One-time per repo
echo '{"name":"project","pr_provider":"github"}' | mp init
# pr_provider: github | gitlab

# User-level multiplexer choice (uses args, not stdin)
mp config get multiplexer
mp config set multiplexer tmux   # tmux, zellij, cmux, or none

# Relocate the .monkeypuzzle state dir (e.g. into a gitignored path)
mp move .DONOTCOMMIT/monkeypuzzle
mp move .monkeypuzzle                # move back to the default
```

## Typical flow

1. `mp create` — worktree + session + on-piece-create hook
2. Work in the worktree, commit normally
3. `mp pr create --draft` — push, open draft PR/MR, fires pr-create hooks
4. Human reviews
5. `mp pr ready` — flip to ready (separate command, never auto), fires pr-ready hooks
6. After merge: `mp done` or `mp cleanup`

Draft → ready is always a manual step. Don't call `mp pr ready` unless the human has approved the flip.
