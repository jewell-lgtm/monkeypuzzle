# Workflow guide

Monkeypuzzle gives each change its own branch, its own git worktree and, when you're ready, its own PR. You cut a piece, stack more pieces on it if the change is big, open the PRs, merge, and clean up. mp fires a shell hook at every one of those transitions, so label state machines, reviewer policies and notifications live in your scripts, not in mp.

## Core concepts

### Pieces

A **piece** is one change. Each piece:

- Has its own branch
- Lives in its own git worktree at `<repo>/.monkeypuzzle/pieces/<piece-name>/`, gitignored inside the repo
- Can have a parent piece, which makes it part of a [stack](#stacking)
- Gets its own PR/MR when you run `mp pr create`

Multiplexer sessions (tmux, zellij, cmux, herdr) are optional; see [Integrations](integrations.md#multiplexers).

### Why worktrees?

- Switch between in-flight work without stashing
- Run tests in one piece while editing in another
- Keep a long-running dev server in one worktree and a fresh checkout in another
- Hooks have a stable `MP_WORKTREE_PATH` to chdir into

### Why hooks?

Workflows differ. PHProcess flips GitLab labels on draft→ready; another team auto-assigns reviewers; another posts to Slack; another runs `cargo fmt` on piece create. mp does the worktree/branch/PR orchestration and emits a hook at every transition. The hook is a shell script, so it can do whatever you want. Every transition is also appended to a global history log, hook or not; read it with [`mp history`](./commands.md#mp-history).

## A common recipe

One way to run a piece end to end. Every step is optional and every gate below has a bypass; hooks decide what your flow needs.

```
mp create [--name <name> | --prompt <text>]
        │
        ▼  on-piece-create.sh   (detached, fire-and-forget; logs to .monkeypuzzle/logs/)
        │
   worktree ready
        │
        ▼  (work, commit)
        │
        ▼  before-piece-update.sh / after-piece-update.sh   (env: MP_MAIN_BRANCH)
   mp update — merge main in
        │
        ▼  before-pr-create.sh               (env: MP_PR_BASE_BRANCH)
   mp pr create [--draft] — push + open PR/MR
        ▼  after-pr-create.sh                (env: MP_PR_NUMBER, MP_PR_URL, MP_PR_BASE_BRANCH)
        │
        ▼  (review, iterate)
        │
        ▼  before-pr-ready.sh                (env: MP_PR_NUMBER, MP_PR_URL, MP_PR_BASE_BRANCH)
   mp pr ready — flip draft to ready
        ▼  after-pr-ready.sh                 (same env)
        │
        ▼  before-piece-merge.sh             (env: MP_MAIN_BRANCH)
   mp merge — merge piece into main (or merge the PR on the forge)
        ▼  after-piece-merge.sh              (same env)
        │
        ▼  is-piece-done.sh (optional)       — exit 0 = merged (for squash-merge detection)
   mp done / cleanup — remove the worktree
```

### Gates and their bypasses

| Gate (default)                              | Bypass                                                                                              |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------- |
| `mp done` refuses an unmerged piece         | `--force` / stdin `{"force":true}` / `mp config set done_require_merged false` (branch kept)         |
| `mp merge` refuses when the target is ahead | `--no-update-check` / stdin `{"no_update_check":true}` / `mp config set merge_require_updated false` |
| `mp merge` refuses a piece with children    | `--reparent-children` (re-homes them) / `--force` (leaves them orphaned)                            |
| `mp abandon` refuses a dirty worktree       | `--force` (discards uncommitted changes — data-loss protection, not policy)                          |
| `mp cleanup` removes merged pieces only     | `is-piece-done.sh` decides what "merged" means (e.g. squash-merges)                                 |

Piece basics always available to every hook: `MP_PIECE_NAME`, `MP_WORKTREE_PATH`, `MP_REPO_ROOT`. Pieces placed on a remote box get two more; see [Remote development](remote-development.md#hooks).

## Stacking

A big change goes up as a stack of small pieces, each with its own PR targeting the piece below it. mp records each piece's parent and keeps the stack in sync. The full flag reference is in [`mp stack`](commands.md#mp-stack).

```bash
mp create --name auth-model           # base piece, off main
mp stack append --name auth-api       # child of the current piece
mp stack prepend --name auth-schema   # insert between the current piece and its parent
mp create --name auth-ui --parent auth-api   # same as append, from anywhere
```

`mp pr create` in a stacked piece targets the parent's branch, so each PR shows only its own diff.

Keep the stack current as main moves and lower pieces change:

```bash
mp stack status            # the tree, PR/MR state, and drift vs the forge
mp stack sync              # preview: which pieces would be synced (dry-run)
mp stack sync --apply      # update main from origin, then merge each parent into its children
mp stack sync --strategy rebase --apply   # rebase instead of merge
mp stack continue          # after resolving a rebase conflict
mp stack undo              # restore every branch to the snapshot the last sync took
```

`mp sync` does the same for one piece: it merges `origin/<parent>` into the piece you're in.

When a lower piece merges, `mp done` (and `mp abandon`, `mp cleanup`) re-homes its children onto its parent. Run `mp stack sync --apply` to restack them, and `mp stack status --apply-bases` if the forge still shows the old PR bases. To move a piece by hand, `mp stack set-parent --parent <piece|main>` (metadata only; sync restacks).

## Moving between pieces

```bash
mp create --name feature-a            # worktree A
mp create --name feature-b            # worktree B

mp                                    # picker over this repo's pieces and branches
mp go                                 # picker across every registered project
mp switch feature-a                   # by piece or branch name
mp switch feat/new-idea --create      # brand-new name: create the piece on that branch
mp open feature-a                     # open the worktree in your editor
```

Without a multiplexer, `mp switch` and `mp create` print the worktree path. Load [`mp shell-init`](integrations.md#follow-mp-into-the-worktree-mp-shell-init) and your shell follows mp into the worktree; without it, `cd "$(mp switch feature-a)"` does the same. [`mp open`](integrations.md#editor-and-terminal-mp-open) hands the worktree to your editor or a new terminal window. With a multiplexer configured, `mp switch` attaches the piece's session instead; see [Integrations](integrations.md#multiplexers).

### The inbox

Pieces pile up across repositories, so mp keeps one list of them, in your order:

```bash
mp inbox                     # every piece in every registered project
mp inbox --sort urgency      # what needs you first: blocked agents, then PRs in review
```

Rows you have ranked come first, in your order. mp breaks ties among the rest with what it already knows (an open PR, a merged branch, an agent waiting on you), and `--sort urgency` puts that ahead of your order. Snoozed rows drop to the bottom until their time comes. The tmux and herdr pickers and the dashboard are all views over `mp inbox --json`, so whatever you rearrange in one shows up in the others. See [`mp inbox`](./commands.md#mp-inbox).

Rearrange it from anywhere (`mp inbox move fix-auth --top`, `mp inbox note fix-auth "waiting on review"`, `mp inbox snooze fix-auth --for 2d`) and step through it:

```bash
mp inbox next                # switch to the piece after this one (wraps)
mp inbox prev                # and back
```

Each step is the same switch `mp switch` performs, so the shell wrapper follows it and `cd "$(mp inbox next)"` works too.

## Hooks

Hooks are executable scripts in `.monkeypuzzle/hooks/`, named after the transition they run at. A non-zero exit aborts the calling operation, except for `after-*` hooks, where a failure logs a warning (the side effect already happened). The full list and every environment variable are in the [hooks reference](commands.md#hooks).

### A worked example: GitLab MR with a label flip + reviewer

PHProcess-shape workflow: opening the MR flips it to "Doing", the user-driven ready-flip flips it to "Code Review ausstehend" and assigns a reviewer.

`.monkeypuzzle/hooks/after-pr-create.sh`:

```bash
#!/bin/bash
[ -z "$MP_PR_NUMBER" ] && exit 0
glab mr update "$MP_PR_NUMBER" --label Doing
```

`.monkeypuzzle/hooks/after-pr-ready.sh`:

```bash
#!/bin/bash
[ -z "$MP_PR_NUMBER" ] && exit 0
glab mr update "$MP_PR_NUMBER" \
  --label "Code Review ausstehend" --unlabel Doing \
  --reviewer my-reviewer
```

Then:

```bash
mp create --name add-login   # spawn the piece
# ... work ...
mp pr create --draft         # opens draft MR, "Doing" label flip fires
# ... self-review ...
mp pr ready                  # ready label flip + reviewer assignment fires
```

No `--reviewer` flag, no `--label` arg, no PHProcess-specific code in mp.

### Pre-merge gate

Run tests before a merge can land:

```bash
# .monkeypuzzle/hooks/before-piece-merge.sh
#!/bin/bash
cd "$MP_WORKTREE_PATH"
go test ./... || exit 1
go vet ./... || exit 1
```

### Per-piece dev setup

Install deps as soon as the worktree exists:

```bash
# .monkeypuzzle/hooks/on-piece-create.sh
#!/bin/bash
cd "$MP_WORKTREE_PATH"
go mod download
```

`on-piece-create.sh` runs **detached**: `mp create` doesn't wait for it, so a
slow `go mod download` (or `npm install`, submodule init, etc.) never holds up
the worktree being ready. Its output goes to
`.monkeypuzzle/logs/on-piece-create-<piece-name>.log`; tail that file if a piece
seems to be missing its dependencies.

Setting up a remote box for placed pieces has its own hook, `on-box-connect.sh`; see [Remote development](remote-development.md#hooks).

### Post-merge promote

Cherry-pick onto a staging branch after merge. `mp merge` only knows about one downstream, so multi-stage deploys live in this hook:

```bash
# .monkeypuzzle/hooks/after-piece-merge.sh
#!/bin/bash
cd "$MP_REPO_ROOT"
git fetch origin staging:staging
git checkout staging
git merge "$MP_MAIN_BRANCH" --ff-only
git push origin staging
```

### Squash-merge detection

GitHub squash-merges aren't visible to git's `branch --merged`. Tell mp the piece is merged by exiting 0 from `is-piece-done.sh`:

```bash
# .monkeypuzzle/hooks/is-piece-done.sh
#!/bin/bash
branch="$(cd "$MP_WORKTREE_PATH" && git branch --show-current)"
gh pr list --head "$branch" --state merged --json number | grep -q '"number"'
```

### Slack ping on ready

```bash
# .monkeypuzzle/hooks/after-pr-ready.sh
#!/bin/bash
curl -sS -X POST -H 'Content-Type: application/json' \
  -d "{\"text\":\"PR ready: $MP_PR_URL\"}" "$SLACK_WEBHOOK_URL"
```

## Forge support

| Provider | PRs/MRs |
| --- | --- |
| GitHub | `pr_provider: github`, uses `gh` |
| GitLab | `pr_provider: gitlab`, uses `glab mr` |

`mp pr create` pushes the branch and opens a PR/MR via the configured provider; its title defaults to the piece name. Everything beyond that (labels, reviewers, downstream tickets) is hook territory.

## Troubleshooting

### "Main branch is ahead"

`mp merge` refuses a stale piece by default (`--no-update-check` bypasses). One option is to pull main into the piece first:

```bash
mp update
# resolve any conflicts
mp merge
```

### Finding pieces on disk

```bash
mp list                                  # current repo
mp list --all                            # across all registered projects
ls .monkeypuzzle/pieces/                 # raw filesystem (from the repo root)
```

### Cleaning up

```bash
mp cleanup                               # preview merged pieces (dry-run by default)
mp cleanup --apply                       # remove all merged pieces
mp cleanup --dry-run                     # explicit preview (never prompts)

mp abandon foo                           # discard unmerged piece
mp abandon foo --force                   # also discard uncommitted changes
mp abandon foo --delete-branch           # also delete the git branch
```
