# Contributing to Monkeypuzzle

> **Testing Philosophy**: We use outside-in testing. Every new feature starts with an integration test that proves the happy path works. Unit tests fill in edge cases. See [docs/contributing.md](docs/contributing.md) for details.
>
> **Reviewing**: See [docs/reviewing.md](docs/reviewing.md) for code review guidelines.

## Development Setup

### Option 1: Docker (Recommended for Reproducible Issues)

The **preferred way** to develop and create reproducible bug reports is using the Docker development environment.

See [docs/docker-development.md](docs/docker-development.md) for complete documentation.

**Quick start:**

```bash
# Build the Docker image
docker build -t monkeypuzzle-dev .

# Run with your source code mounted
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev
```

This provides a clean Ubuntu environment with Go, git, tmux, gh CLI, and mp pre-installed.

### Option 2: Local Development

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

**Prerequisites:**

- Go 1.24+ (see `go.mod` for exact version)
- git (for `mp piece` command)
- tmux (for `mp piece` command)
- gh CLI (for GitHub PR provider)

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
# Unit tests only (fast feedback during development)
go test ./...

# All tests including integration (before PR, CI)
go test -tags=integration ./...

# Specific package with verbose output
go test ./internal/core/init/... -v

# With coverage
go test ./... -cover
```

### Outside-In Testing

**Every new feature must have an integration test for the happy path.**

1. Write integration test first (`*_integration_test.go`, `//go:build integration`)
2. Make it pass with simplest implementation
3. Add unit tests for edge cases (`*_test.go`, mocked deps)
4. Refactor with confidence

Integration tests use real dependencies (filesystem, git, etc). Unit tests use mocks (`MemoryFS`, `BufferOutput`, `MockExec`).

See [docs/contributing.md](docs/contributing.md) for full testing guide.

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
2. **Write integration test for happy path first**
3. Implement the feature
4. Add unit tests for edge cases
5. Ensure all tests pass: `go test -tags=integration ./...`
6. Ensure code is vetted: `go vet ./...`
7. **If reporting a bug**, include Docker reproduction steps (see [docs/docker-development.md](docs/docker-development.md))
8. Open a PR with a clear description

See [docs/reviewing.md](docs/reviewing.md) for what reviewers look for.

### Creating Reproducible Bug Reports

When reporting bugs, please use the Docker environment to ensure reproducibility:

```bash
# Build and test in Docker
docker build -t monkeypuzzle-dev .
docker run -it --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  monkeypuzzle-dev \
  bash -c "mp <command-that-reproduces-bug>"
```

Include the Docker commands and output in your bug report.

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
