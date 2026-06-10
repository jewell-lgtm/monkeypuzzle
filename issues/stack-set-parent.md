---
title: mp stack set-parent — re-parent a piece without hand-editing metadata
---

# mp stack set-parent — re-parent a piece without hand-editing metadata

Biggest functional gap vs git-town: moving a piece from parent A to parent B
required hand-editing piece-metadata.json.

`mp stack set-parent [--piece <name>] --parent <piece|main>`:
- defaults to the current piece when run from a piece worktree
- validates the target parent exists; rejects self and descendants (cycles)
- metadata-only; `mp stack sync` restacks the branches
- flags / JSON stdin / --schema per the standard contract

Covered by three integration tests (happy path, cycle, unknown parent) and
table-driven input validation tests.
