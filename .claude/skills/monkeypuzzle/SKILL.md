---
name: monkeypuzzle
description: Interact with the monkeypuzzle (mp) CLI for project workflow management. Use when initializing projects, managing issues, PRs, or working with .monkeypuzzle config files. Supports JSON stdin for programmatic use.
---

# Monkeypuzzle CLI

CLI tool for git worktree-based development workflow. Binary: `mp`

## Agent Usage

**All agent-compatible commands support:**
- `--schema` flag to get expected JSON input format
- JSON via stdin: `echo '{"field":"value"}' | mp <command>`
- Flags: `mp <command> --field value`
- JSON output to stdout

**Commands for agents:**

| Command | Agent-friendly | Notes |
|---------|----------------|-------|
| `mp issue list` | ✅ | JSON output, stdin filter |
| `mp issue create` | ✅ | JSON stdin or flags |
| `mp piece create` | ✅ | Use `--skip-switch` flag |
| `mp piece list --flat` | ✅ | JSON output |
| `mp piece update` | ✅ | Run from worktree |
| `mp piece merge` | ✅ | Run from worktree |
| `mp piece pr create` | ✅ | Flags for title/body |
| `mp piece cleanup --force` | ✅ | Use `--force` to skip prompts |
| `mp piece abandon` | ✅ | Use `--name` and `--force` |
| `mp init` | ✅ | JSON stdin or flags |
| `mp switch` | ✅ | `--project` + (`--piece`/`--issue`/`--branch`); JSON stdin equivalent |

## mp issue list

List issues. Returns JSON array.

```bash
# All issues
mp issue list

# Filter by status
mp issue list --status todo
mp issue list --status todo,in-progress

# JSON stdin
echo '{"status":["todo"]}' | mp issue list

# Schema
mp issue list --schema
```

**Output:**
```json
[
  {"path": "issues/add-login.md", "title": "Add login", "status": "todo"},
  {"path": "issues/fix-bug.md", "title": "Fix bug", "status": "in-progress"}
]
```

## mp issue create

Create markdown issue file. Returns JSON.

```bash
# JSON stdin
echo '{"title":"Add feature","description":"Details"}' | mp issue create

# Flags
mp issue create --title "Add feature" --description "Details"

# Schema
mp issue create --schema
```

**Output:**
```json
{"path": "issues/add-feature.md", "title": "Add feature", "filename": "add-feature.md"}
```

## mp piece create

Create new piece (git worktree). **Use `--skip-switch` for agents.**

```bash
# From issue (recommended)
mp piece create --issue issues/add-login.md --skip-switch

# With name
mp piece create --name my-feature --skip-switch

# JSON stdin
echo '{"issue_path":"issues/add-login.md","skip_switch":true}' | mp piece create

# Schema
mp piece create --schema
```

**Output:**
```json
{"name": "add-login", "worktree_path": "/path/to/pieces/add-login", "session_name": "mp-piece-add-login"}
```

**Effects:**
- Creates worktree in `~/.local/share/monkeypuzzle/pieces/<repo-id>/<name>`
- If from issue: updates issue status to `in-progress`

## mp piece list

List all pieces.

```bash
# JSON output (for agents)
mp piece list --flat

# Tree view (human readable)
mp piece list
```

**Output (--flat):**
```json
[
  {"name": "feature-auth", "worktree_path": "/path", "parent": "main", "mod_time": "2025-01-04T10:00:00Z"},
  {"name": "auth-oauth", "worktree_path": "/path", "parent": "feature-auth", "mod_time": "2025-01-04T11:00:00Z"}
]
```

## mp piece update

Merge main into current piece. Run from piece worktree.

```bash
mp piece update
mp piece update --main-branch develop
```

## mp piece merge

Squash-merge piece into main. Run from piece worktree.

```bash
mp piece merge
mp piece merge --main-branch develop
```

## mp piece pr create

Create GitHub PR. Run from piece worktree.

```bash
mp piece pr create --title "Add login" --body "Implements login feature"
mp piece pr create --base develop
```

**Output:**
```json
{"url": "https://github.com/owner/repo/pull/123", "number": 123}
```

## mp piece cleanup

Remove merged piece worktrees.

```bash
# For agents - skip confirmation
mp piece cleanup --force

# Preview
mp piece cleanup --dry-run
```

## mp piece abandon

Remove unmerged piece.

```bash
# For agents
mp piece abandon --name my-feature --force

# Also delete branch
mp piece abandon --name my-feature --force --delete-branch

# JSON stdin
echo '{"name":"my-feature","force":true}' | mp piece abandon
```

## mp init

Initialize monkeypuzzle in project.

```bash
# JSON stdin
echo '{"name":"myproject","issue_provider":"markdown","pr_provider":"github"}' | mp init

# Schema
mp init --schema
```

## Workflow for Agents

```bash
# 1. List open issues
mp issue list --status todo

# 2. Create piece from issue
echo '{"issue_path":"issues/add-login.md","skip_switch":true}' | mp piece create

# 3. Work in the worktree path returned
cd /path/to/worktree

# 4. After commits, create PR
mp piece pr create --title "Add login" --body "Description"

# 5. After merge, cleanup
mp piece cleanup --force
```

## Directory Structure

```
project/
├── .monkeypuzzle/
│   └── monkeypuzzle.json
└── issues/
    └── *.md

~/.local/share/monkeypuzzle/pieces/<repo-id>/
└── <piece-name>/          # Worktree
    └── .monkeypuzzle/
        ├── current-issue.json
        ├── piece-metadata.json
        └── pr-metadata.json
```
