---
name: managing-monkeypuzzle
description: Drives the mp CLI, a worktree-per-piece git flow with lifecycle hooks. Use when working in a .monkeypuzzle project: create pieces, switch between them, open draft PRs/MRs, flip them ready, merge and clean up.
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

# Switch to anything by name: a piece, a branch (adopted on the fly), or a
# brand-new piece with "create":true. Project defaults to the current repo.
echo '{"target":"my-feature"}' | mp switch
echo '{"target":"feat/new-thing","create":true}' | mp switch
mp switch feat/new-thing --create
mp go --json                         # same picker, across every registered project

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

# Sweep all merged pieces (dry-run by default; --apply / "apply":true to remove)
mp cleanup                           # preview what would be cleaned
echo '{"apply":true}' | mp cleanup

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

# Flip a draft to ready-for-review. Its own command; mp never flips one itself.
mp pr ready
```

Draft creation fires `before-pr-create.sh` / `after-pr-create.sh`.
Ready-flip fires `before-pr-ready.sh` / `after-pr-ready.sh`.
Both pass `MP_PR_NUMBER`, `MP_PR_URL`, `MP_PR_BASE_BRANCH` in env.

**mp does not own the review gate.** Ready is a separate command so that
whatever a project requires before review happens between `pr create` and
`pr ready`. Satisfy that gate, then flip. What it consists of belongs to the
project's workflow docs: an approving human, a green CI run, a reviewer's
verdict, or nothing at all. A project that wants it enforced rather than
remembered puts the check in `before-pr-ready.sh`, which aborts the flip on a
non-zero exit. mp never advances an existing draft by itself, though note that
`mp pr create` opens a non-draft PR unless you pass `draft`.

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

A non-zero exit aborts the calling operation. The `after-*` hooks warn instead,
since their change already happened.

## Init and config

```bash
# One-time per repo
echo '{"name":"project","pr_provider":"github"}' | mp init
# pr_provider: github | gitlab

# User-level multiplexer choice (takes args, not stdin)
mp config get multiplexer
mp config set multiplexer tmux   # tmux, zellij, cmux, or none

# Relocate the .monkeypuzzle state dir (e.g. into a gitignored path)
mp move .DONOTCOMMIT/monkeypuzzle
mp move .monkeypuzzle                # move back to the default
```

## Working across projects

The commands above act on the repo you are standing in. These span every
registered project and work from anywhere:

```bash
mp inbox --json                      # every piece in flight, ranked
mp agent list --json --all           # live agents across all projects
mp history --json --event 'pr.*' --since 24h
mp project list --json               # what mp knows about
mp go --json                         # switch across every project
```

`mp wait --timeout 5m` blocks until no agent is working. It works per repo, so
it fails outside a git repo and only sees the pieces of the one you are in.

The `monkeypuzzle-inbox` skill covers ranking, notes, snoozing, and keying an
external system on a piece `id`. Use it rather than re-deriving the JSON shape
here.

## Remote projects

`--host`, `--dir` and `--project` go **before** the verb. `--host` proxies the
whole command over ssh. `--project` proxies when that project is registered
with a host, and otherwise runs the command in the project's local path. mp
rejects `--dir` unless you pass `--host` or `--project` with it.

```bash
mp --host build-box status
mp --project api inbox --json        # proxied if that project has a host
```

## Piece identity

Every piece has an `id` that outlives its name, branch and worktree path.
`mp create --json` returns it, and `mp piece show --ensure-id` mints one for a
piece that predates ids. Record the `id` when something outside mp needs to
refer back to a piece, since `project/piece` changes with a rename.

## Typical flow

1. `mp create` makes the worktree and session, then fires `on-piece-create.sh`
2. Work in the worktree, commit normally
3. `mp pr create --draft` pushes, opens the draft PR/MR, fires the pr-create hooks
4. Satisfy whatever the project requires before review
5. `mp pr ready` flips it to ready and fires the pr-ready hooks
6. After merge, `mp done` or `mp cleanup`

Step 4 belongs to the project. mp guarantees only that step 5 never fires by
itself. Check the project's workflow docs and `.monkeypuzzle/hooks/` for what
the gate is before you flip a draft.
