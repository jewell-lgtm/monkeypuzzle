---
name: monkeypuzzle
description: Interact with the monkeypuzzle (mp) CLI for project workflow management. Use when initializing projects, managing issues, PRs, or working with .monkeypuzzle config files. Supports JSON stdin for programmatic use.
---

# Monkeypuzzle CLI

CLI tool for git worktree-based development workflow. Binary: `mp`

## Commands Overview

| Command | Description |
|---------|-------------|
| `mp init` | Initialize monkeypuzzle in a project |
| `mp piece` | Show current piece status |
| `mp piece new` | Create new piece (worktree + tmux) |
| `mp piece update` | Sync piece with main branch |
| `mp piece merge` | Merge piece back to main |
| `mp piece cleanup` | Remove merged piece worktrees |
| `mp piece pr create` | Create GitHub PR for piece |
| `mp issue create` | Create a markdown issue file |

## mp init

Initialize monkeypuzzle. Creates `.monkeypuzzle/monkeypuzzle.json`.

```bash
# JSON stdin (recommended for agents)
echo '{"name":"myproject","issue_provider":"markdown","pr_provider":"github"}' | mp init

# Get schema
mp init --schema

# Flags
mp init --name myproject --issue-provider markdown --pr-provider github
```

## mp piece

Show current piece status. Returns JSON.

```bash
mp piece
# Output: {"in_piece":true,"piece_name":"my-feature","worktree_path":"/path","repo_root":"/repo"}
# Or: {"in_piece":false,"repo_root":"/repo"}
```

## mp piece new

Create new piece (git worktree + tmux session).

```bash
# From issue file (recommended)
mp piece new --issue issues/my-feature.md

# With custom name
mp piece new --name my-feature

# Auto-generated name
mp piece new
```

**Flags:**
- `--issue <path>` - Create from issue file (sets piece name from issue title)
- `--name <name>` - Custom piece name (mutually exclusive with --issue)

**Effects:**
- Creates git worktree in `~/.local/share/monkeypuzzle/pieces/<name>`
- Creates tmux session `mp-piece-<name>`
- If from issue: updates issue status to `in-progress`

## mp piece update

Merge main branch into current piece. Must run from piece worktree.

```bash
mp piece update
mp piece update --main-branch develop
```

**Flags:**
- `--main-branch <branch>` - Branch to merge from (default: main)

## mp piece merge

Squash-merge piece back into main. Must run from piece worktree.

```bash
mp piece merge
mp piece merge --main-branch develop
```

**Flags:**
- `--main-branch <branch>` - Branch to merge into (default: main)

**Requirements:**
- Must be in piece worktree
- Main branch must not have new commits (run `mp piece update` first)

## mp piece cleanup

Remove worktrees for merged pieces.

```bash
mp piece cleanup              # Cleanup merged pieces
mp piece cleanup --dry-run    # Preview what would be cleaned
mp piece cleanup --force      # Skip confirmation
```

**Flags:**
- `--dry-run` - Show what would be cleaned without making changes
- `--force` - Skip confirmation prompts
- `--main-branch <branch>` - Main branch to check merge status against

**Effects:**
- Removes git worktrees for merged branches
- Kills associated tmux sessions
- Updates linked issue status to `done`

## mp piece pr create

Create GitHub PR for current piece. Must run from piece worktree.

```bash
mp piece pr create
mp piece pr create --title "My PR" --body "Description"
mp piece pr create --base develop
```

**Flags:**
- `--title <title>` - PR title (default: issue title or piece name)
- `--body <body>` - PR description
- `--base <branch>` - Base branch (default: main)

**Effects:**
- Pushes branch to origin
- Creates PR via `gh pr create`
- Stores PR metadata in `.monkeypuzzle/pr-metadata.json`

## mp issue create

Create a markdown issue file.

```bash
# JSON stdin
echo '{"title":"My Feature","description":"Details here"}' | mp issue create

# Flags
mp issue create --title "My Feature" --description "Details"
```

## Workflow Example

```bash
# 1. Initialize project
echo '{"name":"myapp","issue_provider":"markdown","pr_provider":"github"}' | mp init

# 2. Create issue
echo '{"title":"Add login"}' | mp issue create

# 3. Start working on issue
mp piece new --issue issues/add-login.md

# 4. (in piece worktree) Make changes, commit...

# 5. Create PR
mp piece pr create

# 6. After PR merged, cleanup
mp piece cleanup
```

## Directory Structure

```
project/
├── .monkeypuzzle/
│   └── monkeypuzzle.json
└── issues/
    └── *.md

~/.local/share/monkeypuzzle/pieces/
└── <piece-name>/          # Worktree
    └── .monkeypuzzle/
        ├── current-issue.json   # Link to issue
        └── pr-metadata.json     # PR info
```
