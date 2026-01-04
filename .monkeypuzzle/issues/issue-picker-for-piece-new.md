---
title: Issue picker for piece new
status: done
---

# Issue picker for piece new

Add proper input layer abstraction to `mp piece new` following the established pattern:
- `input.go`: Field defs, Input struct, Schema(), ParseJSON(), Validate()
- TUI picker for selecting issues (when interactive)
- Handler receives validated `Input`, executes

## Acceptance Criteria

- [x] `mp piece new` shows issue picker when run interactively without flags
- [x] `mp piece new --schema` outputs JSON schema
- [x] `echo '{"issue_path":"..."}' | mp piece new` works (stdin mode)
- [x] `mp piece new --issue <path>` still works (flags mode)
- [x] Integration test covers happy path
- [x] Unit tests cover edge cases
