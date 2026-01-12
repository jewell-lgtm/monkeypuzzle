---
title: convert-branch-to-piece
status: done
description: Add ability to convert existing git branch to piece. User on branch not created via mp should be able to run command to adopt it as piece, creating worktree entry and metadata.
---

# convert-branch-to-piece

Add ability to convert existing git branch to piece. User on branch not created via mp should be able to run command to adopt it as piece, creating worktree entry and metadata.

## Acceptance Criteria

- [x] `mp piece adopt` command exists
- [x] When run in main repo on non-main branch, creates worktree in pieces dir
- [x] Writes piece-metadata.json with parent (default: main)
- [x] Creates tmux session for piece
- [x] Supports --parent flag for stacking
- [x] Supports --name flag to override piece name (defaults to branch name)
- [x] Integration test covers happy path
- [x] Errors if already in a piece
- [x] Errors if on main/master branch
