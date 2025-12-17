---
name: monkeypuzzle
description: Interact with the monkeypuzzle (mp) CLI for project workflow management. Use when initializing projects, managing issues, PRs, or working with .monkeypuzzle config files. Supports JSON stdin for programmatic use.
---

# Monkeypuzzle CLI

CLI tool for development workflow management. Binary: `mp`

## mp init

Initialize monkeypuzzle in current directory. Creates `.monkeypuzzle/` with config.

### Modes

**1. Stdin JSON (recommended for agents):**
```bash
echo '{"name":"myproject","issue_provider":"markdown","pr_provider":"github"}' | mp init
```

**2. Get schema with defaults:**
```bash
mp init --schema
# Output: {"name":"dirname","issue_provider":"markdown","pr_provider":"github"}
```

**3. Direct flags:**
```bash
mp init --name myproject --issue-provider markdown --pr-provider github
```

**4. Interactive (default for TTY):**
```bash
mp init
```

### Flags
- `--name` - Project name
- `--issue-provider` - Issue provider (valid: `markdown`)
- `--pr-provider` - PR provider (valid: `github`)
- `--schema` - Output expected JSON format and exit
- `--yes, -y` - Overwrite existing config without prompting

### JSON Input Schema

```json
{
  "name": "project-name",
  "issue_provider": "markdown",
  "pr_provider": "github"
}
```

All fields required. Valid providers:
- `issue_provider`: `markdown`
- `pr_provider`: `github`

## Agent Workflow

```bash
# Get defaults, modify, pipe back
mp init --schema | jq '.name = "my-project"' | mp init

# Or construct JSON directly
echo '{"name":"foo","issue_provider":"markdown","pr_provider":"github"}' | mp init
```

## Config Output

Location: `.monkeypuzzle/monkeypuzzle.json`

```json
{
  "version": "1",
  "project": { "name": "project-name" },
  "issues": {
    "provider": "markdown",
    "config": { "directory": ".monkeypuzzle/issues" }
  },
  "pr": {
    "provider": "github",
    "config": {}
  }
}
```

## Directory Structure

```
.monkeypuzzle/
├── monkeypuzzle.json    # Main config
└── issues/              # Markdown issue files
```

## Providers

- `markdown`: Issues as `.monkeypuzzle/issues/*.md`
- `github`: PRs via `gh` CLI

## Quick Reference

| Mode | Command | Use Case |
|------|---------|----------|
| Stdin | `echo '{"name":"x",...}' \| mp init` | Agents, scripts |
| Schema | `mp init --schema` | Get JSON template |
| Flags | `mp init --name x --issue-provider markdown --pr-provider github` | CI |
| Interactive | `mp init` | Humans |
