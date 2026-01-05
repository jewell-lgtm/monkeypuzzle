# monkeypuzzle

**Every feature is a piece of the puzzle. Work on them separately, assemble when ready.**

Monkeypuzzle (`mp`) gives each piece of work its own isolated workspace—git worktree, tmux session, linked issue. Pick up a piece, put it down, pick up another. They all fit when you're ready to merge.

## Why?

- **No context-switching** — Each piece is a separate worktree. No stashing, no branch juggling.
- **Pick up where you left off** — Dedicated tmux session per piece. Your terminal state survives.
- **Built for humans and agents** — Interactive TUI for humans, JSON stdin/stdout for AI agents. Same commands, different constraints, one workflow.

## Quick Start

```bash
go install github.com/jewell-lgtm/monkeypuzzle@latest

mp init                    # Interactive setup
mp piece new               # Start a new piece
# ... do the thing ...
mp piece merge             # Assemble it into main
```

## Works With AI Agents

Every command accepts JSON and outputs structured data. Agents can request schemas, pipe data, and stay in flow:

```bash
mp piece new --schema                     # Get expected input format
echo '{"name":"my-feature"}' | mp piece new   # Create piece via JSON
mp piece list                             # JSON output for parsing
```

No special "agent mode"—the same CLI that works interactively works programmatically.

## Commands

| Command           | What it does                   |
| ----------------- | ------------------------------ |
| `mp init`         | Initialize project             |
| `mp piece new`    | Create worktree + tmux session |
| `mp piece list`   | Show all pieces                |
| `mp piece switch` | Jump to a piece                |
| `mp piece update` | Sync with main                 |
| `mp piece merge`  | Assemble piece into main       |

See [docs/commands.md](docs/commands.md) for full reference.

## Docs

- [Getting Started](docs/getting-started.md) — Installation, first piece
- [Workflow Guide](docs/workflow.md) — Stacked pieces, lifecycle
- [Commands Reference](docs/commands.md) — All flags, JSON schemas
- [Architecture](docs/architecture.md) — How it's built
- [Contributing](docs/contributing.md) — Dev setup, testing philosophy

## License

MIT — see [LICENSE](LICENSE)
