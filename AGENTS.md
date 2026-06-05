# Monkeypuzzle Development Guide

## Build & Test

```bash
go build -o mp .      # Build binary
go test ./...         # Run all tests
go vet ./...          # Lint
```

## Workflow
Dogfood the mp tool whenever possible during its development. Always use outside-in testing, where a single integration test of the happy path exists before starting any feature work, and then edge cases and other situations are covered in unit tests. When performing feature work, always keep the issue markdown file up to date .

## Pieces
Issues are (in this repo) markdown files managed with the `mp issue` command, and development work consists of `pieces` (also managed with the mp command) which may be stacked on each other (`mp stack` operates over a whole stack: `status`, `sync`, `append`, `prepend`, `continue`). Most pieces of work are complete when there is a PR with a good description, and all tests and code quality checks pass

## CLI Interaction Model

Every `mp` command follows one consistent interaction contract. The modes, in priority order:

1. **Interactive (TTY, default).** With a terminal, a Bubble Tea **wizard/TUI** guides you through the decisions, using smart defaults inferred from detected state. Commands with several decisions (e.g. `mp done`, `mp abandon`) are multi-step wizards.
2. **Flags.** Passing flags **skips the corresponding wizard steps** — each flag pre-answers one decision. Pass enough flags and there is no wizard at all; the command runs straight through.
3. **Stdin JSON (for agents/automation).** `echo '{...}' | mp <cmd>` is fully programmatic: JSON in, JSON out on stdout. There is **no separate "agent mode"** — the same CLI is the API.
4. **`--schema`.** `mp <cmd> --schema` prints the expected JSON-stdin shape for that command.

**The load-bearing rule:** non-interactive invocations (flags or JSON) **FAIL LOUDLY on genuine ambiguity** rather than prompting or guessing — there is no human to ask. Interactive mode is the only place ambiguity gets resolved, via the wizard. A pure no-op is not ambiguous.

Output convention: human-readable text goes to stderr; stdout is reserved for machine-readable JSON.

## Code Style

- Keep functions small
- Table-driven tests
- No error swallowing - propagate or handle explicitly
- Prefer explicit dependencies
