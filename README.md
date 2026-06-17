# monkeypuzzle

**Worktree-per-piece git-flow with issue-aware hooks at every transition.**

Monkeypuzzle (`mp`) gives every in-flight piece of work its own git worktree and tmux session, then fires shell hooks — pre-populated with `MP_ISSUE_ID`, `MP_PR_URL`, and friends — at every lifecycle moment. Bring whatever label state machines, reviewer policies, and notifications your workflow needs; mp stays out of the way and does the boring orchestration.

## Why?

- **No context-switching.** Each piece is its own worktree + tmux session. No stashing, no branch juggling, no "I had three things going."
- **Hooks at every transition.** `on-piece-create`, `before-/after-pr-create`, `before-/after-pr-ready`, `before-/after-piece-merge`, `is-piece-done` — all with rich env vars. Label flips, ticket-journal sync, Slack pings — all hook scripts, not core code.
- **Forge-agnostic.** GitHub and GitLab as first-class providers (via `gh` and `glab`). Draft↔ready is a first-class state — `mp pr create --draft` then `mp pr ready` separately, with hooks around both.
- **Humans and agents share one CLI.** Every command takes flags, stdin JSON, or runs interactive. `--schema` for introspection. No special "agent mode."

## How does this differ from git-town?

git-town manages **stacked branches** in a single working directory. mp manages **isolated worktrees + sessions** for parallel pieces and bolts a hook system onto every lifecycle event. Pick the one whose primary metaphor matches yours — they compose if you really want both.

## Quick start

```bash
# 1. Install and point mp at tmux (once)
go install github.com/jewell-lgtm/monkeypuzzle@latest
mp config set multiplexer tmux

cd path/to/your/repo
mp init                    # configure issue + pr provider (markdown/linear/plane/gitlab + github/gitlab)

mp create --issue 1845         # spawn worktree + tmux session + on-piece-create hook fires
# ... do the thing ...
mp pr create --draft           # push, open draft PR/MR, fires before/after-pr-create hooks
# ... self-review ...
mp pr ready                    # flip to ready, fires before/after-pr-ready hooks
```

Each `mp` step fires a shell hook in `.monkeypuzzle/hooks/` with the piece + PR + issue context in env. The hook decides what label state machine, reviewer policy, or downstream system to touch.

## A worked example — GitLab MR with two label flips

`.monkeypuzzle/hooks/on-piece-create.sh` — flip ticket from Dev-Backlog to Doing on pickup:

```bash
#!/bin/bash
[ -z "$MP_ISSUE_NUMBER" ] && exit 0
glab issue update "$MP_ISSUE_NUMBER" --label Doing --unlabel Dev-Backlog
```

`.monkeypuzzle/hooks/after-pr-ready.sh` — flip ticket to Code-Review-ausstehend + assign reviewer when the user calls `mp pr ready`:

```bash
#!/bin/bash
[ -z "$MP_ISSUE_NUMBER" ] && exit 0
glab issue update "$MP_ISSUE_NUMBER" \
  --label "Code Review ausstehend" --unlabel Doing \
  --assignee my-reviewer
```

That's the whole integration. No `--reviewer` baked in, no opinion about labels — mp just hands the hook everything it needs and gets out of the way.

## Hook reference

| Hook | Fires when | Extra env on top of piece basics |
| --- | --- | --- |
| `on-piece-create.sh` | `mp create` finishes the worktree (runs detached/fire-and-forget; output → `.monkeypuzzle/logs/`) | `MP_ISSUE_ID`, `MP_ISSUE_NUMBER`, `MP_SESSION_NAME` |
| `before-piece-update.sh` / `after-piece-update.sh` | around `mp update` | `MP_MAIN_BRANCH` |
| `before-piece-merge.sh` / `after-piece-merge.sh` | around `mp merge` | `MP_MAIN_BRANCH` |
| `before-pr-create.sh` / `after-pr-create.sh` | around `mp pr create` | `MP_PR_NUMBER`, `MP_PR_URL`, `MP_PR_BASE_BRANCH`, `MP_ISSUE_ID`, `MP_ISSUE_NUMBER` |
| `before-pr-ready.sh` / `after-pr-ready.sh` | around `mp pr ready` | same as PR create |
| `is-piece-done.sh` | consulted by `IsBranchMerged` / `mp cleanup` | exit 0 = merged (use to recognise squash-merges) |

Piece basics always available: `MP_PIECE_NAME`, `MP_WORKTREE_PATH`, `MP_REPO_ROOT`.

Hooks are shell scripts; non-zero exit aborts the calling operation (except after-* hooks, which warn but don't fail).

## Works with AI agents

Every command takes flags, stdin JSON, or `--schema`. Output is JSON on stdout.

```bash
mp create --schema
echo '{"name":"my-feature","skip_switch":true}' | mp create
mp list
```

Run `mp claude skill` to drop a `.claude/skills/managing-monkeypuzzle/SKILL.md` into your repo so Claude Code knows the CLI surface.

The key rule: **non-interactive invocations (flags or JSON) fail loudly on genuine ambiguity** rather than prompting or guessing — there's no human to ask. The wizard is the only place ambiguity gets resolved. (A pure no-op isn't ambiguous.)

| Command | What it does |
| --- | --- |
| `mp init` / `mp reinit` | Configure providers (first run) or refresh `.gitignore` + Claude skill (re-run) |
| `mp` / `mp dash` | Cross-project dashboard |
| `mp create` | Spawn worktree + tmux session (+ `--issue <id>` to link) |
| `mp adopt <branch>` | Bring an existing branch into mp |
| `mp list` | Show pieces (`--all` for cross-project) |
| `mp switch` | Jump to a piece (tmux switch-client when in tmux) |
| `mp update` | Sync piece with main |
| `mp merge` | Merge piece back to main |
| `mp pr create [--draft]` | Push + open PR/MR via configured provider |
| `mp pr ready` | Flip a draft PR/MR to ready |
| `mp done` | After merge: clean up worktree + session |
| `mp config get/set multiplexer` | tmux / none |

See [docs/commands.md](docs/commands.md) for full reference.

## Docs

- [Getting started](docs/getting-started.md) — install + first piece
- [Workflow guide](docs/workflow.md) — lifecycle, hook patterns
- [Commands reference](docs/commands.md) — flags, JSON schemas
- [Architecture](docs/architecture.md) — how it's built
- [Contributing](docs/contributing.md) — dev setup, testing philosophy

## License

MIT — see [LICENSE](LICENSE).
