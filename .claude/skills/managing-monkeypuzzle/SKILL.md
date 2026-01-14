---
name: managing-monkeypuzzle
description: Manages development workflow with mp CLI. Creates pieces (git worktrees), tracks issues, creates PRs. Use when working with .monkeypuzzle projects, mp commands, or piece-based development.
---

# mp CLI

All commands support JSON stdin (`echo '{...}' | mp <cmd>`) with JSON output to stdout. Use `mp <cmd> --schema` for input schema.

## Issues

```bash
# List issues (all or filtered by status)
echo '{}' | mp issue list
echo '{"status":["todo"]}' | mp issue list
echo '{"status":["todo","in-progress"]}' | mp issue list

# Create issue
echo '{"title":"Feature X","description":"Details..."}' | mp issue create

# Search issues (fuzzy match)
echo '{"query":"auth","status":["todo"]}' | mp issue search
```

## Pieces (worktrees)

```bash
# Show current piece status
mp piece

# List all pieces (tree view or flat)
mp piece list
echo '{"flat":true}' | mp piece list

# Create new piece from issue (via --issue flag)
mp piece new --issue issues/feat.md --skip-switch

# Create new piece by name
echo '{"name":"my-feature","skip_switch":true}' | mp piece new

# Create stacked piece (child of another piece)
echo '{"name":"child-feat","parent":"parent-piece"}' | mp piece new

# Switch to existing piece
echo '{"name":"my-feature"}' | mp piece switch

# Update piece with latest from main
echo '{}' | mp piece update
echo '{"main_branch":"develop"}' | mp piece update

# Merge piece back to main (requires no unmerged children)
echo '{}' | mp piece merge
echo '{"force":true}' | mp piece merge  # force even with children

# Create PR for current piece
echo '{}' | mp piece pr create
echo '{"title":"Add X","body":"Description"}' | mp piece pr create

# Cleanup after merge
echo '{}' | mp piece done

# Cleanup all merged pieces
echo '{"force":true}' | mp piece cleanup
echo '{"dry_run":true}' | mp piece cleanup

# Abandon unmerged piece
echo '{"force":true}' | mp piece abandon
echo '{"name":"piece-name","delete_branch":true}' | mp piece abandon

# Convert existing branch to piece
echo '{}' | mp piece adopt
echo '{"name":"custom-name","parent":"main"}' | mp piece adopt
```

## Init & Config

```bash
# Init monkeypuzzle in repo
echo '{"name":"project","issue_provider":"markdown","pr_provider":"github"}' | mp init

# Config (uses args, not JSON stdin)
mp config get multiplexer
mp config set multiplexer tmux  # tmux, zellij, or none
```

## Workflow

1. `mp issue create` or find existing issue
2. `mp piece new --issue issues/foo.md` creates worktree
3. Work in worktree, commit changes
4. `mp piece pr create` pushes and creates PR
5. After PR merged: `mp piece done` or `mp piece cleanup`
