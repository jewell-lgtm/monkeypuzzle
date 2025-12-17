# monkeypuzzle

CLI tool for managing development workflows. Provides a unified interface for project initialization, issue tracking, and PR management.

## Installation

```bash
go install github.com/jewell-lgtm/monkeypuzzle@latest
```

Or build from source:

```bash
git clone https://github.com/jewell-lgtm/monkeypuzzle.git
cd monkeypuzzle
go build -o mp .
```

## Quick Start

```bash
# Interactive mode (humans)
mp init

# Pipe JSON (agents/scripts)
echo '{"name":"myproject","issue_provider":"markdown","pr_provider":"github"}' | mp init

# Get JSON schema with defaults
mp init --schema
```

## Commands

### `mp init`

Initialize monkeypuzzle in current directory. Creates `.monkeypuzzle/` directory with configuration.

**Modes:**

| Mode | Usage | Description |
|------|-------|-------------|
| Interactive | `mp init` | TUI wizard for humans |
| Stdin JSON | `echo '{"name":"x",...}' \| mp init` | Pipe JSON config |
| Flags | `mp init --name x --issue-provider markdown --pr-provider github` | All flags provided |
| Schema | `mp init --schema` | Output JSON template |

**Flags:**

```
--name              Project name
--issue-provider    Issue provider (markdown)
--pr-provider       PR provider (github)
--schema            Output JSON schema and exit
-y, --yes           Overwrite existing config without prompting
```

**JSON Input Schema:**

```json
{
  "name": "project-name",
  "issue_provider": "markdown",
  "pr_provider": "github"
}
```

## Configuration

Config file: `.monkeypuzzle/monkeypuzzle.json`

```json
{
  "version": "1",
  "project": {
    "name": "my-project"
  },
  "issues": {
    "provider": "markdown",
    "config": {
      "directory": ".monkeypuzzle/issues"
    }
  },
  "pr": {
    "provider": "github",
    "config": {}
  }
}
```

### Directory Structure

```
.monkeypuzzle/
├── monkeypuzzle.json    # Main configuration
└── issues/              # Markdown issue files (if using markdown provider)
```

## Providers

### Issue Providers

| Provider | Description |
|----------|-------------|
| `markdown` | Issues stored as markdown files in `.monkeypuzzle/issues/` |

### PR Providers

| Provider | Description |
|----------|-------------|
| `github` | PR management via `gh` CLI |

## Architecture

Monkeypuzzle uses a clean architecture with dependency injection for testability:

```
internal/
├── core/
│   ├── ports.go           # Interfaces (FS, Output)
│   └── init/
│       ├── input.go       # Input validation + schema generation
│       ├── handler.go     # Business logic
│       └── handler_test.go
├── adapters/
│   ├── filesystem.go      # OS + Memory filesystem implementations
│   └── output.go          # Text + JSON + Buffer output implementations
└── tui/init/              # Bubble Tea interactive UI
```

**Key patterns:**

- **Single source of truth**: Field definitions drive both validation and schema generation
- **Dependency injection**: Handlers accept `core.Deps{FS, Output}` for testability
- **Adapter pattern**: Swap implementations (real FS vs memory FS) without changing business logic

## Development

### Prerequisites

- Go 1.24+

### Build

```bash
go build -o mp .
```

### Test

```bash
go test ./...
```

### Lint

```bash
go vet ./...
```

## Integration with AI Agents

Monkeypuzzle is designed for programmatic use by AI agents:

```bash
# Get schema, modify, pipe back
mp init --schema | jq '.name = "my-project"' | mp init

# Direct JSON input
echo '{"name":"foo","issue_provider":"markdown","pr_provider":"github"}' | mp init

# Check if already initialized
test -f .monkeypuzzle/monkeypuzzle.json && echo "initialized"
```

Output goes to stderr, so stdout remains clean for piping.

## License

MIT License - see [LICENSE](LICENSE)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md)
