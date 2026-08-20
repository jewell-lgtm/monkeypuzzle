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

## Your first piece

This is the whole lifecycle — creating a unit of work, shipping it, cleaning up
— in one pass:

```bash
mp create --name my-feature   # worktree + session, branched off main

# ... make your changes in the new worktree ...

mp pr create --draft          # push the branch, open a draft PR/MR
mp pr ready                   # flip it to ready for review
mp merge                      # merge the branch back into main
mp done                       # remove the worktree/session now it's merged
```

Each step fires the matching lifecycle hook if you've dropped one in
`.monkeypuzzle/hooks/` — see [Hooks](commands.md#hooks) and the
[Workflow Guide](workflow.md).

To jump back to work you've already started, run `mp` (scoped to the current
repo) or `mp go` (across every registered project) for a fuzzy picker — or
`mp switch <piece-or-branch-name>` to go straight there by name.

## Next steps

- [Commands Reference](commands.md) - Full command documentation
- [Workflow Guide](workflow.md) - Using pieces for stacked branches
- [Remote development](remote-development.md) - Drive a project on another machine, or place single pieces on a box with `mp create --remote`
