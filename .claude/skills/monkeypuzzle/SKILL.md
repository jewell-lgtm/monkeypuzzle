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
| `mp piece switch` | Switch to an existing piece |
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

Create new piece (git worktree + tmux session). **Auto-switches to the new piece by default.**

```bash
# From issue file (recommended)
mp piece new --issue issues/my-feature.md

# With custom name
mp piece new --name my-feature

# Auto-generated name
mp piece new

# Don't auto-switch
mp piece new --skip-switch
```

**Flags:**
- `--issue <path>` - Create from issue file (sets piece name from issue title)
- `--name <name>` - Custom piece name (mutually exclusive with --issue)
- `--skip-switch` - Don't switch to the new piece after creation

**Effects:**
- Creates git worktree in `~/.local/share/monkeypuzzle/pieces/<name>`
- Creates tmux session `mp-piece-<name>`
- If from issue: updates issue status to `in-progress`
- Switches to the new piece (tmux attach/switch-client or prints path)

## mp piece switch

Switch to an existing piece.

```bash
# Interactive TUI (sorted by modification time)
mp piece switch

# By name
mp piece switch --name my-feature

# JSON stdin
echo '{"name":"my-feature"}' | mp piece switch

# Use with cd (when no tmux)
cd $(mp piece switch --name my-feature)
```

**Flags:**
- `--name <name>` - Piece name to switch to

**Behavior:**
- If tmux session exists and in tmux: uses `switch-client`
- If tmux session exists and outside tmux: uses `attach-session`
- If no tmux: prints path to stdout (use with `cd $(...)`)

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

# 4. Switch to piece (if needed)
mp piece switch --name add-login

# 5. (in piece worktree) Make changes, commit...

# 6. Create PR
mp piece pr create

# 7. After PR merged, cleanup
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
