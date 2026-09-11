# monkeypuzzle

**Solve big problems one piece at a time.**

Monkeypuzzle (`mp`) is a git workflow for the terminal. Every change gets its own branch in its own git worktree; mp stacks the pieces, opens the PRs, and cleans up after merge. At every step it fires a shell hook, pre-populated with `MP_PIECE_NAME`, `MP_PR_URL` and friends, so label state machines, reviewer policies and notifications live in your scripts, not in mp.

## Why?

- **No context-switching.** Each piece is its own worktree. No stashing, no branch juggling, no "I had three things going."
- **Small PRs, stacked.** `mp stack append` builds on the piece you're in; each PR targets its parent, and `mp stack sync` keeps the stack current as main moves.
- **Hooks at every transition.** `on-piece-create`, `before-/after-pr-create`, `before-/after-pr-ready`, `before-/after-piece-merge`, `is-piece-done`, all with rich env vars. Label flips, ticket-journal sync, Slack pings: hook scripts, not core code.
- **Forge-agnostic.** GitHub and GitLab as first-class providers (via `gh` and `glab`). Draft↔ready is a first-class state: `mp pr create --draft`, then `mp pr ready`, with hooks around both.
- **One CLI for humans and scripts.** Every command takes flags, stdin JSON, or runs interactively. `--schema` for introspection.

## How does this differ from git-town?

Both stack branches: git-town with `git town append`, mp with `mp stack`. The difference is the working copy. git-town switches branches in one working directory; mp gives every piece its own worktree, so pieces sit side by side and you move between them without stashing. mp also fires a hook at every lifecycle transition.

## Quick start

```bash
brew install jewell-lgtm/tap/monkeypuzzle   # or: go install github.com/jewell-lgtm/monkeypuzzle/apps/mp@latest
eval "$(mp shell-init zsh)"                 # add to ~/.zshrc: mp moves your shell into the worktree

cd path/to/your/repo
mp init                        # first run: pick a multiplexer (none), then project name + PR provider

mp create --name add-login     # new branch + worktree; your shell is now in it
# ... do the thing, commit ...
mp pr create --draft           # push, open a draft PR/MR
mp pr ready                    # flip to ready for review
mp merge                       # merge into main (or merge the PR on the forge)
mp done                        # remove the worktree, back to the main repo
```

To open pieces in your editor: `mp config set open_command 'code {path}'`, then `mp open add-login`.

Each step fires a shell hook in `.monkeypuzzle/hooks/` with the piece and PR context in env. See [hooks](docs/workflow.md#hooks) for the list and recipes.

## Commands

| Command | What it does |
| --- | --- |
| `mp init` | Configure project name + PR provider (first run) or refresh scaffolding (re-run) |
| `mp create` | New branch + worktree (`--name` or `--prompt`; `--parent` to stack) |
| `mp switch [target]` | Go to a piece or branch by name; adopts an existing branch, or creates with `--create` |
| `mp` / `mp go` | Picker over this repo's pieces / across every project |
| `mp open [target]` | Open a worktree in your editor or a new terminal |
| `mp list` | Show pieces as a tree (`--all` for every project) |
| `mp stack append` / `prepend` | Add a piece above / below the current one |
| `mp stack sync` | Propagate main and each parent down the stack (preview; `--apply`) |
| `mp stack status` | The stack tree, PR state, and drift vs the forge |
| `mp update` / `mp sync` | Merge main / the parent into the current piece |
| `mp pr create [--draft]` | Push + open PR/MR via the configured provider |
| `mp pr ready` | Flip a draft PR/MR to ready |
| `mp merge` | Merge the piece into main |
| `mp done` | Remove the worktree after merge (refuses unmerged; `--force`) |
| `mp abandon` | Remove an unmerged piece |
| `mp cleanup` | Remove every merged piece (preview; `--apply`) |
| `mp doctor` | Check this machine's setup |

See [docs/commands.md](docs/commands.md) for the full reference.

## Integrations

Everything beyond the workflow is opt-in: `mp open` for your editor, `mp shell-init` for your shell, tmux/zellij/cmux/herdr sessions per piece, coding-agent status and an MCP server, remote boxes over ssh, and a web dashboard. See [docs/integrations.md](docs/integrations.md).

## Docs

- [Getting started](docs/getting-started.md) — install + first piece
- [Workflow guide](docs/workflow.md) — the lifecycle, stacking, hooks and recipes
- [Commands reference](docs/commands.md) — flags, example inputs
- [Integrations](docs/integrations.md) — editors, shells, multiplexers, coding agents
- [Remote development](docs/remote-development.md) — drive a project on another machine over ssh, or place single pieces on a box (`mp create --remote`)
- [Architecture](docs/architecture.md) — how it's built
- [Self-hosting](docs/self-hosting.md) — run the server on your own infra via Helm
- [Contributing](docs/contributing.md) — dev setup, testing philosophy

## License

Per-component — see [LICENSING.md](LICENSING.md).

- CLI, MCP server, and tmux plugin: MIT ([LICENSE](LICENSE))
- Server (`apps/mp-server`, `internal/server`): source-available under FSL-1.1-MIT

The "Monkey Puzzle" name and logos are trademarks — see [TRADEMARK.md](TRADEMARK.md).
