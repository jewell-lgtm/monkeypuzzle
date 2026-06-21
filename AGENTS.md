# monkeypuzzle

Monorepo for **monkeypuzzle** (`mp`) — worktree-per-piece git-flow with lifecycle hooks.

**Dogfood `mp` for all work in this repo.** Manage every change as a piece — `mp create`, stack with `mp stack`, ship with `mp pr create` — not raw `git`.

## Apps

- **`apps/mp`** — the `mp` CLI. @apps/mp/README.md
- **`apps/mp-mcp`** — MCP server exposing the mp workflow to agents. @apps/mp-mcp/README.md
- **`apps/mp-server`** — web dashboard + OAuth server (source-available). @apps/mp-server/README.md
- **`apps/tmux`** — tmux plugin: fzf popup to switch/create pieces. @apps/tmux/README.md
- **`website`** — marketing site (Astro). @website/README.md
