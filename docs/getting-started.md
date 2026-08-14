# Getting Started

## Prerequisites

- **Go 1.24+** - Required for building
- **Git** - Required for version control operations
- **tmux** (optional) - For automatic session management when you run `mp create`/`mp switch` interactively from inside tmux. Without it (or when driven by an agent/script), mp prints the worktree path instead of opening a session.

## Installation

### From source (recommended)

```bash
git clone https://github.com/jewell-lgtm/monkeypuzzle.git
cd monkeypuzzle
go build -o mp ./apps/mp   # the mp CLI lives in apps/mp (or just: make build → bin/mp)
sudo mv mp /usr/local/bin/  # or add to PATH
```

### Via go install

```bash
go install github.com/jewell-lgtm/monkeypuzzle/apps/mp@latest
```

## Verify installation

```bash
mp --help
```

## Initialize your first project

Navigate to your project directory and run:

```bash
mp init
```

This launches an interactive wizard:
1. Enter project name (defaults to directory name)
2. Choose PR provider (`github` or `gitlab`)
3. Confirm configuration

Creates `.monkeypuzzle/` directory with configuration.

### Non-interactive initialization

For scripts or CI:

```bash
# Via flags
mp init --name myproject --pr-provider github

# Via JSON stdin
echo '{"name":"myproject","pr_provider":"github"}' | mp init

# Get an example input document, fill in a value, pipe it back
mp init --schema | jq '.name = "custom-name"' | mp init
```

## Next steps

- [Commands Reference](commands.md) - Full command documentation
- [Workflow Guide](workflow.md) - Using pieces for stacked branches
