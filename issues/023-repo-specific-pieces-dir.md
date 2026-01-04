---
title: Repo-specific pieces directory
status: done
---

# Repo-specific pieces directory

Pieces dir should be scoped to repo/working dir, not global. Currently can't run mp in multiple projects simultaneously without conflicts.

## Problem

Pieces stored in shared location means concurrent mp usage across repos interferes.

## Fix

Store pieces in `.monkeypuzzle/pieces/` within repo root (or use repo-derived path in user data dir).

## Considerations

- Migration path for existing pieces?
- Discovery: always look relative to git root

## Acceptance

- Two separate repos can have active pieces simultaneously
- `mp piece list` only shows pieces for current repo
