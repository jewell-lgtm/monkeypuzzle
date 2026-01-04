---
title: CLI --schema flag consistency
status: todo
---

# CLI --schema flag consistency

Commands with JSON input should expose `--schema` for discoverability.

## Current gaps

- `mp piece switch` - has JSON input, missing --schema
- `mp piece abandon` - has JSON input, missing --schema
- `mp piece cleanup` - has JSON output, missing --schema
- `mp piece update` - missing --schema
- `mp piece merge` - missing --schema

## Fix

Add `--schema` flag to each command that outputs the expected JSON input format.

## Acceptance

```bash
mp piece switch --schema | jq .
mp piece abandon --schema | jq .
mp piece update --schema | jq .
mp piece merge --schema | jq .
```
