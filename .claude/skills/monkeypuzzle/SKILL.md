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
| `mp piece create` | ✅ | Use `--skip-switch`; `--issue`/`--prompt`/`--parent` |
| `mp piece list --flat` | ✅ | JSON output |
| `mp piece update` | ✅ | Run from worktree |
| `mp piece merge` | ✅ | Run from worktree |
| `mp piece pr create` | ✅ | Flags for title/body |
| `mp piece done` | ✅ | Run from worktree, after merge |
| `mp piece adopt` | ✅ | Convert a branch into a piece |
| `mp piece cleanup --force` | ✅ | Use `--force` to skip prompts |
| `mp piece abandon` | ✅ | Use `--name` and `--force` |
| `mp flatten --yes` | ✅ | Removes ALL piece worktrees; `--force` for dirty trees |
| `mp stack status/sync/append/prepend/continue` | ✅ | Whole-stack ops; non-interactive |
| `mp project add/list/remove` | ✅ | Global project registry |
| `mp dash --json` | ✅ | Cross-project JSON dashboard |
| `mp move` | ✅ | Relocate the `.monkeypuzzle` dir |
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

# From a prompt (name auto-generated)
mp piece create --prompt "add dark mode" --skip-switch

# Stacked on another piece
mp piece create --name child-feat --parent parent-piece --skip-switch

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

## mp piece done

Clean up the current piece (worktree + tmux) after its branch is merged. Run from the worktree; verifies merge first (use `mp piece abandon` for unmerged pieces).

```bash
mp piece done
mp piece done --main-branch develop
```

## mp piece adopt

Convert an existing git branch into a piece. Local name or remote ref (`origin/foo`).

```bash
mp piece adopt                       # adopt current branch (from main repo)
mp piece adopt my-spike              # adopt a local branch
mp piece adopt --branch origin/foo   # fetch + adopt a remote branch
mp piece adopt my-spike --parent feature-a   # stack under another piece
```

## mp flatten

Remove **all** piece worktrees for the repo, returning to a flat main-only state. Unlike `mp piece cleanup` (merged pieces only), flatten removes every piece regardless of merge status. Interactive runs confirm first; pass `--yes` (or `--force`) to skip the prompt. Branches are kept unless `--delete-branches`.

```bash
# For agents (non-interactive; --yes/--force skip the prompt)
mp flatten --yes
mp flatten --force                 # discard uncommitted changes too
mp flatten --delete-branches       # also delete each piece's branch
mp flatten --dry-run               # preview without removing

# JSON stdin
echo '{"force":true}' | mp flatten
```

## mp stack

Whole-stack ops over pieces stacked via `--parent` (git-town-style). All non-interactive: risky operations abort cleanly and print next steps.

```bash
# Show stack tree, PR state, and drift vs the GitHub PR list
mp stack status

# Propagate main and each parent down the stack
mp stack sync                     # merge (default)
mp stack sync --strategy rebase   # rebase
mp stack sync --push              # push each branch after syncing
mp stack continue                 # resume after resolving rebase conflicts

# Grow the stack
mp stack append --name child-feat     # child of current piece
mp stack append --prompt "add cache"
mp stack prepend --name base-feat      # insert between current and parent
```

## mp project

Global registry of `mp init` projects. Lets mp list pieces/issues across repos and jump between sessions. Aliases: `projects`, `proj`.

```bash
mp project add                   # register current dir
mp project add /path/to/repo
mp project list                  # table (alias: ls, status)
mp project list --json           # machine output
mp project remove my-project     # unregister (alias: rm); repo untouched
```

## mp dash

Cross-project dashboard of projects + piece worktrees. Bare `mp` is equivalent. JSON form includes per-project `pieces`, `issues`, and `branches`.

```bash
mp dash          # interactive (or JSON when not a TTY)
mp dash --json   # force JSON
```

## mp move

Relocate the repo's `.monkeypuzzle` state dir, updating the mapping in `~/.config/monkeypuzzle/project-dirs.json` and repairing piece worktrees.

```bash
mp move .DONOTCOMMIT/monkeypuzzle   # relocate (e.g. into a gitignored path)
mp move .monkeypuzzle               # move back to the default
echo '{"path":".DONOTCOMMIT/monkeypuzzle"}' | mp move
```

## mp config

User-level config (positional args, not JSON stdin).

```bash
mp config get multiplexer
mp config set multiplexer tmux   # tmux, zellij, or none
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
