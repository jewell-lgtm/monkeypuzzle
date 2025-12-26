---
title: MCP Server for Monkeypuzzle
status: todo
---

# MCP Server for Monkeypuzzle

Build an MCP (Model Context Protocol) server exposing mp CLI functionality to AI agents.

## Tools to Expose

- `mp_init` - Initialize monkeypuzzle in a directory
- `mp_piece_new` - Create new piece/worktree
- `mp_piece_update` - Update piece from main branch
- `mp_piece_merge` - Merge piece back to main
- `mp_issue_list` - List issues
- `mp_issue_read` - Read issue content

## Implementation Options

1. **Go native** - Build MCP server in Go alongside existing code
2. **Wrapper** - Thin MCP wrapper calling mp CLI via subprocess

## Acceptance Criteria

- [ ] Server starts and registers tools
- [ ] All core commands accessible via MCP
- [ ] JSON schema for each tool matches CLI schema output
- [ ] Error handling preserves mp error messages
