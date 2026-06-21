# Workflow guide

Monkeypuzzle gives each in-flight piece of work its own git worktree + tmux session, then fires shell hooks at every lifecycle moment. Hooks decide what label state machines, reviewer policies, and notifications your workflow needs — mp stays out of the way.

## Core concepts

### Pieces

A **piece** is an isolated git worktree for a single change. Each piece:

- Lives in its own directory under `~/.local/share/monkeypuzzle/pieces/{repo-hash}/` (Linux) or `~/Library/Application Support/monkeypuzzle/pieces/{repo-hash}/` (macOS)
- Has its own branch
- Has its own tmux session (`mp/<project>/<piece>`) **when you work interactively from inside tmux** — agents and scripts driving mp through its stateless API get the worktree path instead of a session (see [Sessions are interactive-only](#sessions-are-interactive-only))

### Why worktrees?

- Switch between in-flight work without stashing
- Run tests in one piece while editing in another
- A long-running dev server in one session, a fresh checkout in another
- Hooks have a stable `MP_WORKTREE_PATH` to chdir into

### Why hooks?

Workflows differ. PHProcess flips GitLab labels on draft→ready; another team auto-assigns reviewers; another posts to Slack; another runs `cargo fmt` on piece create. mp does the worktree/session/branch orchestration and emits a hook at every transition. The hook is a shell script — write whatever you want.

## The lifecycle

```
mp create [--name <name> | --prompt <text>]
        │
        ▼  on-piece-create.sh   (detached, fire-and-forget; logs to .monkeypuzzle/logs/)
        │                       (env: MP_SESSION_NAME)
   worktree + tmux session ready
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
   mp merge — merge piece into main
        ▼  after-piece-merge.sh              (same env)
        │
        ▼  is-piece-done.sh (optional)       — exit 0 = merged (for squash-merge detection)
   mp done / cleanup — remove worktree + session
```

Piece basics always available to every hook: `MP_PIECE_NAME`, `MP_WORKTREE_PATH`, `MP_REPO_ROOT`.

## A worked example — GitLab MR with a label flip + reviewer

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

## Hook recipes

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

`on-piece-create.sh` runs **detached** — `mp create` doesn't wait for it, so a
slow `go mod download` (or `npm install`, submodule init, etc.) never holds up
the worktree being ready. Its output goes to
`.monkeypuzzle/logs/on-piece-create-<piece-name>.log`; tail that file if a piece
seems to be missing its dependencies.

### Post-merge promote

Cherry-pick onto a staging branch after merge — mp's `piece merge` only knows about one downstream, so multi-stage deploys live in this hook:

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

Hooks are shell scripts. Non-zero exit aborts the calling operation, except for `after-*` hooks where failure logs a warning but doesn't fail the operation (the side-effect already happened).

## Multiple concurrent pieces

```bash
mp create --name feature-a            # worktree A + session mp/<proj>/feature-a
mp create --name feature-b            # worktree B + session mp/<proj>/feature-b

mp switch                             # TUI selector across pieces
mp switch --name feature-a            # by name (uses tmux switch-client if you're in tmux)
```

Long-running processes survive switching — each piece's session keeps its own dev server, log tail, REPL.

## Forge support

| Provider | PRs/MRs |
| --- | --- |
| GitHub | `pr_provider: github`, uses `gh` |
| GitLab | `pr_provider: gitlab`, uses `glab mr` |

`mp pr create` pushes the branch and opens a PR/MR via the configured provider; its title defaults to the piece name. Everything beyond that — labels, reviewers, downstream tickets — is hook territory.

## Tmux integration

Sessions are namespaced `mp/<project>/<piece>` so worktrees from different repos never collide. `<project>` comes from `project.name` in `.monkeypuzzle/monkeypuzzle.json` (defaults to the repo dir name).

```bash
mp switch                             # TUI selector (works with/without tmux)
mp switch --name foo                  # by name
tmux ls | grep "^mp/"                       # all mp sessions
tmux attach -t mp/<project>/<piece>         # raw tmux attach
```

When you're already inside tmux, `mp switch` calls `switch-client` so you stay attached.

### tmux plugin

For an in-tmux UI that doesn't take over your current pane, the companion plugin
in [`apps/tmux`](../apps/tmux/README.md) binds keys to a fuzzy `fzf` popup
for switching between pieces (`prefix + p`) and creating a new one
(`prefix + P`). It reads state with `mp go --json` and delegates the actual
session work back to `mp`.

### Sessions are interactive-only

mp manages a tmux session **only** when you drive it interactively from inside
tmux — a real terminal on stdin (isatty) **and** `$TMUX` set. In that context
`create`/`switch`/`go` create the session and `switch-client` your existing
client to it.

Driven any other way — by an agent or script through the stateless API
(flags / stdin JSON, output captured), or from a terminal that isn't inside
tmux — mp creates **no** session and instead returns the worktree path (in the
result JSON, or on stdout so `cd $(mp switch …)` works). `$TMUX` alone doesn't
count: it is inherited by child processes, so an agent launched from inside your
tmux still has it set; the TTY check is what excludes those callers, so an agent
can never spawn a stray session or `switch-client` your terminal out from under
you.

### Tmux 101

If you've never used tmux, the essentials:

| Command | Description |
| --- | --- |
| `tmux` | start a new session |
| `tmux ls` | list sessions |
| `tmux attach -t <name>` | attach to session |
| `Ctrl+b d` | detach (session keeps running) |
| `Ctrl+b s` | session picker |
| `Ctrl+b c` | new window |
| `Ctrl+b %` / `Ctrl+b "` | split pane |

Sessions persist after detaching — your dev server keeps running, your terminal state survives reattach. If tmux isn't installed, mp falls back to printing the worktree path (use `cd $(mp switch --name <p>)`).

## Troubleshooting

### "Main branch is ahead"

`mp merge` refuses to merge a stale piece. Pull main into the piece first:

```bash
mp update
# resolve any conflicts
mp merge
```

### Finding pieces on disk

```bash
mp list                                  # current repo
mp list --all                            # across all registered projects
ls ~/.local/share/monkeypuzzle/pieces/         # raw filesystem (Linux)
ls ~/Library/Application\ Support/monkeypuzzle/pieces/   # macOS
```

### Cleaning up

```bash
mp cleanup                               # remove all merged pieces
mp cleanup --dry-run                     # preview

mp abandon --name foo                    # discard unmerged piece
mp abandon --name foo --force            # also discard uncommitted changes
mp abandon --name foo --delete-branch    # also delete the git branch
```
