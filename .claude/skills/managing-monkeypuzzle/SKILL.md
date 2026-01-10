---
name: managing-monkeypuzzle
description: Manages development workflow with mp CLI. Creates pieces (git worktrees), tracks issues, creates PRs. Use when working with .monkeypuzzle projects, mp commands, or piece-based development.
---

# Monkeypuzzle CLI

Binary: `mp`

## Agent Usage

All commands support:
- `--schema` to get expected JSON input
- JSON via stdin: `echo '{...}' | mp <cmd>`
- Flags: `mp <cmd> --flag value`
- JSON output to stdout

## Commands

### mp issue list

```bash
mp issue list                      # all issues
mp issue list --status todo        # filter
echo '{"status":["todo"]}' | mp issue list
```

### mp issue create

```bash
echo '{"title":"Feature","description":"..."}' | mp issue create
mp issue create --title "Feature"
```

### mp piece new

Creates worktree. Use `--skip-switch` for agents.

```bash
mp piece new --issue issues/feat.md --skip-switch
mp piece new --name my-feature --skip-switch
echo '{"issue_path":"issues/feat.md","skip_switch":true}' | mp piece new
```

### mp piece list

```bash
mp piece list --flat   # JSON array for agents
mp piece list          # tree view for humans
```

### mp piece update

Merge main into current piece. Run from worktree.

```bash
mp piece update
```

### mp piece merge

Squash-merge piece into main. Run from worktree.

```bash
mp piece merge
```

### mp piece pr create

Create GitHub PR. Run from worktree.

```bash
mp piece pr create --title "Add feature" --body "Description"
```

### mp piece cleanup

Remove merged pieces.

```bash
mp piece cleanup --force   # skip prompts
mp piece cleanup --dry-run
```

### mp piece abandon

Remove unmerged piece.

```bash
mp piece abandon --name my-feature --force
```

### mp init

Initialize project.

```bash
echo '{"name":"project","issue_provider":"markdown","pr_provider":"github"}' | mp init
```

## Agent Workflow

```bash
# 1. Pick issue
mp issue list --status todo

# 2. Create piece
echo '{"issue_path":"issues/add-login.md","skip_switch":true}' | mp piece new

# 3. Work in returned worktree_path

# 4. Create PR
mp piece pr create --title "Add login"

# 5. After merge
mp piece cleanup --force
```
