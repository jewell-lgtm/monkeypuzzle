---
title: CLI JSON stdout consistency
status: done
---

# CLI JSON stdout consistency

Commands should output structured JSON to stdout for machine consumption.

## Current gaps

- `mp init` - returns error only, no JSON output
- `mp issue create` - discards IssueFile return, no JSON output
- `mp piece update` - returns error only
- `mp piece merge` - returns error only

## Fix

Add `json.MarshalIndent()` + `fmt.Println()` to stdout in each command's RunE function.

## Acceptance

```bash
# All should output parseable JSON to stdout
mp init --name foo | jq .project.name
mp issue create --title "Test" | jq .path
mp piece update | jq .status
mp piece merge | jq .merged
```
