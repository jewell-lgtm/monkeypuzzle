# Getting Started

## Prerequisites

- **Go 1.24+** - Required for building
- **Git** - Required for version control operations
- **tmux** (optional) - For automatic session management with `mp piece new`

## Installation

### From source (recommended)

```bash
git clone https://github.com/jewell-lgtm/monkeypuzzle.git
cd monkeypuzzle
go build -o mp .
sudo mv mp /usr/local/bin/  # or add to PATH
```

### Via go install

```bash
go install github.com/jewell-lgtm/monkeypuzzle@latest
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
2. Choose issue provider (markdown)
3. Choose PR provider (github)
4. Confirm configuration

Creates `.monkeypuzzle/` directory with configuration.

### Non-interactive initialization

For scripts or CI:

```bash
# Via flags
mp init --name myproject --issue-provider markdown --pr-provider github

# Via JSON stdin
echo '{"name":"myproject","issue_provider":"markdown","pr_provider":"github"}' | mp init

# Get schema with defaults, modify, pipe back
mp init --schema | jq '.name = "custom-name"' | mp init
```

## Next steps

- [Commands Reference](commands.md) - Full command documentation
- [Workflow Guide](workflow.md) - Using pieces for stacked branches
