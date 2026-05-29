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

## CLI Modes

All commands should support:

1. **Interactive** (default with TTY) - Bubble Tea TUI
2. **Stdin JSON** - `echo '{}' | mp <cmd>`
3. **Flags** - `mp <cmd> --flag value`
4. **Schema** - `mp <cmd> --schema` outputs expected JSON

## Code Style

- Keep functions small
- Table-driven tests
- No error swallowing - propagate or handle explicitly
- Prefer explicit dependencies
