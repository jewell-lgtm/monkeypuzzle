---
title: mp stack undo — restore pre-sync branch SHAs
---

# mp stack undo — restore pre-sync branch SHAs

git-town's killer safety feature was missing: a stack sync that goes wrong
left users digging through reflogs across N worktrees.

- `mp stack sync` now snapshots every piece branch SHA to
  `.monkeypuzzle/stack-snapshot.json` before mutating anything
- `mp stack undo` plans first (skips unchanged/missing pieces, fails loudly if
  any affected worktree has tracked uncommitted changes), then `reset --hard`s
  each piece worktree back to its snapshot SHA
- Local-only: remotes untouched; the success message says to force-push with
  lease if branches were already pushed
- Untracked files never block undo (reset --hard leaves them alone)

Covered by integration tests: restore happy path, no-snapshot fails loudly,
tracked-dirty worktree refuses.
