# monkeypuzzle

**Worktree-per-piece git-flow with lifecycle hooks at every transition.**

Monkeypuzzle (`mp`) gives every in-flight piece of work its own git worktree and tmux session, then fires shell hooks — pre-populated with `MP_PIECE_NAME`, `MP_PR_URL`, and friends — at every lifecycle moment. Bring whatever label state machines, reviewer policies, and notifications your workflow needs; mp stays out of the way and does the boring orchestration.

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
brew install jewell-lgtm/tap/monkeypuzzle   # or: go install github.com/jewell-lgtm/monkeypuzzle/apps/mp@latest
mp config set multiplexer tmux

cd path/to/your/repo
mp init                    # configure project name + pr provider (github/gitlab)

mp create --name add-login     # spawn worktree + tmux session + on-piece-create hook fires
# ... do the thing ...
mp pr create --draft           # push, open draft PR/MR, fires before/after-pr-create hooks
# ... self-review ...
mp pr ready                    # flip to ready, fires before/after-pr-ready hooks
```

Each `mp` step fires a shell hook in `.monkeypuzzle/hooks/` with the piece + PR context in env. The hook decides what label state machine, reviewer policy, or downstream system to touch.

## A worked example — GitLab MR with a label flip + reviewer

`.monkeypuzzle/hooks/after-pr-create.sh` — flip the MR from Draft to Doing on open:

```bash
#!/bin/bash
[ -z "$MP_PR_NUMBER" ] && exit 0
glab mr update "$MP_PR_NUMBER" --label Doing
```

`.monkeypuzzle/hooks/after-pr-ready.sh` — flip the MR to Code-Review-ausstehend + assign reviewer when the user calls `mp pr ready`:

```bash
#!/bin/bash
[ -z "$MP_PR_NUMBER" ] && exit 0
glab mr update "$MP_PR_NUMBER" \
  --label "Code Review ausstehend" --unlabel Doing \
  --reviewer my-reviewer
```

That's the whole integration. No `--reviewer` baked in, no opinion about labels — mp just hands the hook everything it needs and gets out of the way.

## Hook reference

| Hook | Fires when | Extra env on top of piece basics |
| --- | --- | --- |
| `on-piece-create.sh` | `mp create` finishes the worktree (runs detached/fire-and-forget; output → `.monkeypuzzle/logs/`) | `MP_SESSION_NAME` |
| `before-piece-update.sh` / `after-piece-update.sh` | around `mp update` / `mp sync` | `MP_MAIN_BRANCH` |
| `before-piece-merge.sh` / `after-piece-merge.sh` | around `mp merge` | `MP_MAIN_BRANCH` |
| `before-pr-create.sh` / `after-pr-create.sh` | around `mp pr create` | `MP_PR_NUMBER`, `MP_PR_URL`, `MP_PR_BASE_BRANCH` |
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
| `mp init` / `mp reinit` | Configure project name + PR provider (first run) or refresh `.gitignore` + Claude skill (re-run) |
| `mp` | Picker scoped to the current repo (falls through to `mp go`'s view outside one) |
| `mp go` | Cross-project picker; `mp switch --all` under a shorter name |
| `mp switch [target]` | Jump to a piece or branch by name — adopts an existing branch, or creates a new one with `--create` |
| `mp create` | Spawn worktree + multiplexer session (`--name` or `--prompt`) |
| `mp adopt <branch>` | Bring an existing branch into mp |
| `mp list` | Show pieces (`--all` for cross-project) |
| `mp update` | Sync piece with main |
| `mp sync` | Sync piece with its parent (prefers origin's version) |
| `mp merge` | Merge piece back to main |
| `mp pr create [--draft]` | Push + open PR/MR via configured provider |
| `mp pr ready` | Flip a draft PR/MR to ready |
| `mp done` | After merge: clean up worktree + session |
| `mp agent focus [id\|piece] [--blocked]` | Switch the client to an agent's pane, or fall back to a piece switch |
| `mp config get/set multiplexer` | tmux / zellij / cmux / none |

See [docs/commands.md](docs/commands.md) for full reference.

## Docs

- [Getting started](docs/getting-started.md) — install + first piece
- [Workflow guide](docs/workflow.md) — lifecycle, hook patterns
- [Commands reference](docs/commands.md) — flags, example inputs
- [Remote development](docs/remote-development.md) — drive a project on another machine over ssh, or place single pieces on a box (`mp create --remote`)
- [Architecture](docs/architecture.md) — how it's built
- [Self-hosting](docs/self-hosting.md) — run the server on your own infra via Helm
- [Contributing](docs/contributing.md) — dev setup, testing philosophy
- [tmux plugin](apps/tmux/README.md) — fzf popup to switch/create pieces in tmux

## License

Per-component — see [LICENSING.md](LICENSING.md).

- CLI, MCP server, and tmux plugin: MIT ([LICENSE](LICENSE))
- Server (`apps/mp-server`, `internal/server`): source-available under FSL-1.1-MIT

The "Monkey Puzzle" name and logos are trademarks — see [TRADEMARK.md](TRADEMARK.md).
