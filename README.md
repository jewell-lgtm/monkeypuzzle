# monkeypuzzle

**Every feature is a piece of the puzzle. Work on them separately, assemble when ready.**

Monkeypuzzle (`mp`) gives each piece of work its own isolated workspace — git worktree, tmux session, linked issue. Pick up a piece, put it down, pick up another. They all fit when you're ready to merge.

## Why?

- **No context-switching** — Each piece is a separate worktree. No stashing, no branch juggling.
- **Pick up where you left off** — Dedicated tmux session per piece. Your terminal state survives.
- **One picker for everything** — `mp switch` over pieces, open issues, and stray branches. Start or resume work in one keystroke.

## Quick Start (tmux)

This is the recommended flow. One project, one tmux multiplexer, one picker.

```bash
# 1. Install and point mp at tmux (once)
go install github.com/jewell-lgtm/monkeypuzzle@latest
mp config set multiplexer tmux

# 2. In your repo, set it up (once per project)
mp init

# 3. File something to work on
mp issue create --title "Add dark mode"

# 4. Open the picker. Fuzzy-find the issue and hit enter — mp creates a
#    piece worktree, sets the issue to in-progress, and attaches a tmux
#    session for it. From here on, you never leave the picker.
mp switch

# ... do the work in the attached tmux session ...

# 5. Open a PR for the piece (run from inside the worktree)
mp piece pr create

# 6. After it's merged, sweep up the worktree and tmux session
mp piece done
```

To jump between pieces, or to pick up a stray branch you left behind, just run `mp switch` again — pieces, open todo issues, and unadopted local branches all show in one fuzzy-filtered list.

## More ways of working

- **[Workflow guide](docs/workflow.md)** — stacked pieces, multiple projects, the full lifecycle, and integrating with GitHub PRs.
- **[Commands reference](docs/commands.md)** — every command, every flag, every JSON schema.
- **[Getting started](docs/getting-started.md)** — slower walkthrough of installation and your first piece.
- **[Architecture](docs/architecture.md)** — how the worktree/session/issue layers fit together.
- **AI agents** — every command accepts JSON stdin and emits JSON to stdout; pass `--schema` for the input shape. No "agent mode" — the same CLI works programmatically. See the commands reference for schemas.

## License

MIT — see [LICENSE](LICENSE)
