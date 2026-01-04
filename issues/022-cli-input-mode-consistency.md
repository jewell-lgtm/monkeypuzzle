---
title: CLI input mode consistency
status: todo
---

# CLI input mode consistency

Commands should support stdin JSON for agent/machine use.

## Current gaps

- `mp piece update` - flags only
- `mp piece merge` - flags only
- `mp piece cleanup` - flags only

## Fix

Add stdin JSON parsing to each command following the pattern:
1. Check flags first
2. Check stdin for JSON
3. Fall back to defaults/TUI

## Acceptance

```bash
echo '{"main_branch":"develop"}' | mp piece update
echo '{"main_branch":"develop"}' | mp piece merge
echo '{"dry_run":true}' | mp piece cleanup
```
