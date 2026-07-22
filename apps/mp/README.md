# mp — the monkeypuzzle CLI

The `mp` command. Each unit of work is a **piece**: a branch in its own git
worktree (+ optional multiplexer session — tmux, zellij, or cmux), with shell hooks fired at every lifecycle
transition. Pieces stack — `mp stack` operates over the whole stack — and
`mp pr create` / `mp pr ready` open and flip forge PRs.

Every command takes flags, stdin JSON, or runs interactively; `--schema` prints
the JSON shape. Same surface for humans and agents — there is no separate agent
mode.

MIT. Build with `make build` (→ `bin/mp`). Fuller docs: [root README](../../README.md) and [docs/](../../docs).
