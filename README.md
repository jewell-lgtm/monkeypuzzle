# monkeypuzzle

**One workflow for humans and AI agents.**

Monkeypuzzle (`mp`) manages atomic "pieces" of work—isolated worktrees with their own tmux sessions, issues, and PRs. Same CLI works interactively for humans or via JSON pipes for agents.

## Why?

- **Isolation** — Each piece is a separate worktree. No stashing, no branch switching mid-task.
- **Agent-friendly** — Every command accepts JSON stdin. Agents get schemas, pipe data, stay in flow.
- **Human-friendly** — Interactive TUI when you want it, flags when you don't.

## Quick Start

```bash
go install github.com/jewell-lgtm/monkeypuzzle@latest

mp init                    # Interactive setup
mp piece new               # Start isolated work
# ... do the thing ...
mp piece merge             # Ship it
```

Or let an agent drive:

```bash
echo '{"name":"my-feature"}' | mp piece new
```

## Commands

| Command         | What it does                        |
| --------------- | ----------------------------------- |
| `mp init`       | Initialize project                  |
| `mp piece new`  | Create worktree + tmux session      |
| `mp piece`      | Show current piece status           |
| `mp piece update` | Sync with main                    |
| `mp piece merge`  | Merge back to main                |

See [docs/commands.md](docs/commands.md) for full reference.

## Docs

- [Workflow Guide](docs/workflow.md) — Stacked branches, piece lifecycle
- [Commands Reference](docs/commands.md) — All flags, JSON schemas
- [Architecture](docs/architecture.md) — How it's built
- [Contributing](docs/contributing.md) — Dev setup, Docker environment

## License

MIT — see [LICENSE](LICENSE)
