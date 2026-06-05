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
mp pr create

# 6. After it's merged, sweep up the worktree and tmux session
mp done
```

To jump between pieces, or to pick up a stray branch you left behind, just run `mp switch` again — pieces, open todo issues, and unadopted local branches all show in one fuzzy-filtered list.

## How you talk to mp

Every `mp` command exposes the same four-mode interaction contract, in priority order:

1. **Interactive (TTY, default)** — a Bubble Tea **wizard/TUI** walks you through the decisions, pre-filling smart defaults from detected state. Commands with several decisions (e.g. `mp done`, `mp abandon`) are multi-step wizards.
2. **Flags** — passing flags **skips the matching wizard steps**; pass enough and there's no wizard at all (`mp create --name foo`).
3. **Stdin JSON** — `echo '{...}' | mp <cmd>` is fully programmatic: JSON in, JSON out on stdout. There is no separate "agent mode" — the same CLI is the API.
4. **`--schema`** — `mp <cmd> --schema` prints the expected JSON-stdin shape.

The key rule: **non-interactive invocations (flags or JSON) fail loudly on genuine ambiguity** rather than prompting or guessing — there's no human to ask. The wizard is the only place ambiguity gets resolved. (A pure no-op isn't ambiguous.)

## More ways of working

- **[Workflow guide](docs/workflow.md)** — stacked pieces, multiple projects, the full lifecycle, and integrating with GitHub PRs.
- **[Commands reference](docs/commands.md)** — every command, every flag, every JSON schema.
- **[Getting started](docs/getting-started.md)** — slower walkthrough of installation and your first piece.
- **[Architecture](docs/architecture.md)** — how the worktree/session/issue layers fit together.
- **AI agents** — every command accepts JSON stdin and emits JSON to stdout; pass `--schema` for the input shape. No "agent mode" — the same CLI works programmatically. See the commands reference for schemas.

## License

MIT — see [LICENSE](LICENSE)
