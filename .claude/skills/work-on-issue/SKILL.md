---
name: work-on-issue
description: Obsolete — issue-driven workflows were removed from mp. Use `mp create` / `mp create --prompt` to start a piece instead.
triggers:
  - "work on issue"
  - "start issue"
  - "implement issue"
  - "pick up issue"
---

# Work on Issue (removed)

The issue-driven workflow this skill described no longer exists. mp has no issue
concept: there are no issue providers, no `issues/` directory, and no
`mp create --issue` / `mp switch --issue`.

Start a piece directly instead:

```bash
mp create --name <name>          # named piece
mp create --prompt "<text>"      # name auto-generated from a free-form prompt
```

Then follow the normal outside-in flow (integration test for the happy path
first, then unit tests for edge cases), commit, and open a PR with
`mp pr create`. See the workflow guide for the full lifecycle.
