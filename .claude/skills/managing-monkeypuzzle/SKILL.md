---
name: managing-monkeypuzzle
description: Manages development workflow with mp CLI. Creates pieces (git worktrees), tracks issues, creates PRs. Use when working with .monkeypuzzle projects, mp commands, or piece-based development.
---

# mp CLI

All commands: `echo '{...}' | mp <cmd>` with JSON output to stdout. Use `mp <cmd> --schema` for input schema.

## Commands

```
echo '{}' | mp issue list
echo '{"status":["todo"]}' | mp issue list
echo '{"title":"Feature","description":"..."}' | mp issue create
echo '{"issue_path":"issues/feat.md","skip_switch":true}' | mp piece new
echo '{"name":"my-feature","skip_switch":true}' | mp piece new
echo '{"flat":true}' | mp piece list
echo '{}' | mp piece update
echo '{}' | mp piece merge
echo '{"title":"Add feature","body":"..."}' | mp piece pr create
echo '{}' | mp piece done
echo '{"force":true}' | mp piece cleanup
echo '{"force":true}' | mp piece abandon
echo '{"name":"project","issue_provider":"markdown","pr_provider":"github"}' | mp init
```
