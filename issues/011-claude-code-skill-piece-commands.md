---
title: Add mp piece commands to Claude Code skill
status: done
---

# Add mp piece commands to Claude Code skill

## Description

Expand the monkeypuzzle Claude Code skill (`.claude/skills/monkeypuzzle/SKILL.md`) to document all `mp piece` commands for agent usage.

## Current State

The skill only documents `mp init`. Missing:
- `mp piece` - Check piece status
- `mp piece new` - Create new piece/worktree
- `mp piece update` - Sync piece with main branch
- `mp piece merge` - Merge piece back to main

## Requirements

Add documentation sections for each command covering:
- Command syntax and flags
- JSON stdin input (agent-friendly mode)
- Output format (JSON)
- Requirements/preconditions
- Example usage

## Commands to Document

### mp piece (status)

```bash
mp piece
# Output: {"in_piece":true,"piece_name":"feature-x","worktree_path":"/path","repo_root":"/repo"}
```

### mp piece new

```bash
# From issue file
mp piece new --issue .monkeypuzzle/issues/feature.md

# With custom name
mp piece new --name my-feature

# JSON stdin
echo '{"issue":".monkeypuzzle/issues/feature.md"}' | mp piece new
```

Flags:
- `--name` - Custom piece name (mutually exclusive with --issue)
- `--issue` - Create from issue file

### mp piece update

```bash
mp piece update
mp piece update --main-branch develop
```

Flags:
- `--main-branch` - Branch to sync from (default: main)

Requirement: Must run from within piece worktree

### mp piece merge

```bash
mp piece merge
mp piece merge --main-branch develop
```

Flags:
- `--main-branch` - Branch to merge into (default: main)

Requirements:
- Must run from within piece worktree
- Main branch must not have new commits (run `mp piece update` first if needed)

## File to Modify

`.claude/skills/monkeypuzzle/SKILL.md`
