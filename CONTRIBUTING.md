# Contributing to Monkeypuzzle

## Development Setup

1. Clone the repository:
```bash
git clone https://github.com/jewell-lgtm/monkeypuzzle.git
cd monkeypuzzle
```

2. Install dependencies:
```bash
go mod download
```

3. Build:
```bash
go build -o mp .
```

## Project Structure

```
monkeypuzzle/
├── main.go                    # Entry point
├── cmd/mp/                    # CLI commands (Cobra)
│   ├── root.go
│   └── init.go
├── internal/
│   ├── core/                  # Business logic + interfaces
│   │   ├── ports.go           # FS, Output interfaces
│   │   └── init/              # Init command logic
│   ├── adapters/              # Interface implementations
│   │   ├── filesystem.go      # OSFS, MemoryFS
│   │   └── output.go          # TextOutput, JSONOutput
│   └── tui/init/              # Bubble Tea UI
└── pkg/styles/                # Shared Lip Gloss styles
```

## Architecture Principles

1. **Dependency Injection**: Handlers receive dependencies via `core.Deps` struct
2. **Single Source of Truth**: Input field definitions drive validation AND schema generation
3. **Testability**: Use `adapters.MemoryFS` and `adapters.BufferOutput` in tests

## Adding a New Command

1. Create handler in `internal/core/<command>/`
   - `input.go` - Input struct, validation, schema
   - `handler.go` - Business logic

2. Create CLI wrapper in `cmd/mp/<command>.go`
   - Wire up Cobra command
   - Handle input modes (flags, stdin, interactive)
   - Inject real dependencies

3. Add tests in `internal/core/<command>/handler_test.go`

## Running Tests

```bash
# All tests
go test ./...

# Specific package with verbose output
go test ./internal/core/init/... -v

# With coverage
go test ./... -cover
```

## Code Style

- Run `go vet ./...` before committing
- Run `go fmt ./...` to format code
- Keep functions small and focused
- Use table-driven tests

## Commit Messages

Format: `<type>: <description>`

Types:
- `feat`: New feature
- `fix`: Bug fix
- `refactor`: Code change that neither fixes a bug nor adds a feature
- `docs`: Documentation only
- `test`: Adding tests
- `chore`: Maintenance

Examples:
```
feat: add mp status command
fix: handle empty project name in init
docs: update README with new flags
```

## Pull Requests

1. Create a feature branch from `main`
2. Make your changes
3. Ensure tests pass: `go test ./...`
4. Ensure code is vetted: `go vet ./...`
5. Open a PR with a clear description

## Adding Providers

To add a new issue or PR provider:

1. Add to valid values in `internal/core/init/input.go`:
```go
{
    Name:        "issue_provider",
    ValidValues: []string{"markdown", "your-provider"},
}
```

2. Handle the provider in `internal/core/init/handler.go`:
```go
if input.IssueProvider == "your-provider" {
    // Provider-specific setup
}
```

3. Update documentation in README.md
