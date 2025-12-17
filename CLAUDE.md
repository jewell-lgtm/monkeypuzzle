# Monkeypuzzle Development Guide

## Build & Test

```bash
go build -o mp .      # Build binary
go test ./...         # Run all tests
go vet ./...          # Lint
```

## Architecture

Clean architecture with dependency injection:

- `internal/core/` - Business logic + interfaces (ports)
- `internal/adapters/` - Interface implementations (FS, Output)
- `internal/tui/` - Bubble Tea UI (presentation only)
- `cmd/mp/` - Cobra CLI wiring

### Adding Commands

1. Create `internal/core/<cmd>/input.go` - Input struct, validation, schema (single source of truth)
2. Create `internal/core/<cmd>/handler.go` - Business logic, receives `core.Deps`
3. Create `internal/core/<cmd>/handler_test.go` - Tests with `adapters.MemoryFS` + `adapters.BufferOutput`
4. Create `cmd/mp/<cmd>.go` - Cobra command, wire dependencies

### Key Patterns

- **Single source of truth**: Field definitions in `input.go` drive both validation AND schema generation
- **Dependency injection**: `core.Deps{FS, Output}` passed to handlers
- **Testability**: Use `adapters.MemoryFS` and `adapters.BufferOutput` in tests

## CLI Modes

All commands should support:
1. **Interactive** (default with TTY) - Bubble Tea TUI
2. **Stdin JSON** - `echo '{}' | mp <cmd>`
3. **Flags** - `mp <cmd> --flag value`
4. **Schema** - `mp <cmd> --schema` outputs expected JSON

## Providers

Valid providers defined in `internal/core/init/input.go`:
- Issue: `markdown`
- PR: `github`

Add new providers by updating `ValidValues` in field definitions.

## Testing

```bash
# Unit tests with mock FS
go test ./internal/core/init/... -v

# E2E test
echo '{"name":"test","issue_provider":"markdown","pr_provider":"github"}' | ./mp init
```

## Code Style

- Keep functions small
- Table-driven tests
- No error swallowing - propagate or handle explicitly
