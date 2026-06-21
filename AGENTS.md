# Monkeypuzzle Development Guide

## Build & Test

```bash
go build -o mp .      # Build binary
go test ./...         # Run all tests
go vet ./...          # Lint
```

## Workflow
Dogfood the mp tool whenever possible during its development. Always use outside-in testing, where a single integration test of the happy path exists before starting any feature work, and then edge cases and other situations are covered in unit tests.

## Pieces
Development work consists of `pieces` (managed with the mp command), each created from a name or a free-form prompt (`mp create --name <name>` / `mp create --prompt "..."`). Pieces may be stacked on each other (`mp stack` operates over a whole stack: `status`, `sync`, `append`, `prepend`, `set-parent`, `continue`, `undo`). Most pieces of work are complete when there is a PR with a good description, and all tests and code quality checks pass

## CLI Interaction Model

Every `mp` command follows one consistent interaction contract. The modes, in priority order:

1. **Interactive (TTY, default).** With a terminal, a Bubble Tea **wizard/TUI** guides you through the decisions, using smart defaults inferred from detected state. Commands with several decisions (e.g. `mp done`, `mp abandon`) are multi-step wizards.
2. **Flags.** Passing flags **skips the corresponding wizard steps** — each flag pre-answers one decision. Pass enough flags and there is no wizard at all; the command runs straight through.
3. **Stdin JSON (for agents/automation).** `echo '{...}' | mp <cmd>` is fully programmatic: JSON in, JSON out on stdout. There is **no separate "agent mode"** — the same CLI is the API.
4. **`--schema`.** `mp <cmd> --schema` prints the expected JSON-stdin shape for that command.

**The load-bearing rule:** non-interactive invocations (flags or JSON) **FAIL LOUDLY on genuine ambiguity** rather than prompting or guessing — there is no human to ask. Interactive mode is the only place ambiguity gets resolved, via the wizard. A pure no-op is not ambiguous.

Output convention: human-readable text goes to stderr; stdout is reserved for machine-readable JSON.

**No tmux sessions for agents.** Terminal-multiplexer management (creating, `switch-client`, attaching a piece session) happens **only** when mp is driven interactively — a real TTY on stdin (isatty, not merely a character device) **and** `$TMUX` set. Agents and scripts work through the stateless API (flags / stdin JSON, output captured), which has no controlling terminal, so mp never opens or switches a session for them: `create`/`switch`/`go` return the worktree path (in the result JSON, or on stdout for `cd $(mp switch …)`) and leave tmux untouched. `$TMUX` alone is not trusted — it is inherited by child processes, so an agent spawned from inside a human's tmux still has it set; the TTY check is what excludes those callers and stops an agent's `switch-client` from hijacking the human's terminal. The gate lives in `interactiveSessionContext()` (apps/mp); session-creating commands route through it via `chooseMultiplexer` and `attachSession`. One explicit exception: `MP_TMUX_PLUGIN=1` substitutes for the TTY check (`$TMUX` is still required), letting the companion tmux plugin (`apps/tmux`) drive mp through the stateless API and still have mp perform the switch-client/session-create. It is safe against the inherited-`$TMUX` problem because only the plugin sets it, per invocation — agents never do.

## Code Style

- Keep functions small
- Table-driven tests
- No error swallowing - propagate or handle explicitly
- Prefer explicit dependencies
