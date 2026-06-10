---
title: Beta cleanups — stdout contract, chdir restore, missing-CLI hints
---

# Beta cleanups — stdout contract, chdir restore, missing-CLI hints

- piece handler no longer prints worktree paths to stdout from library code
  (the JSON result already carries them); callers read result.Method
- multiplexer SwitchTo failures now warn before degrading to path mode
  (previously a silent fallback — users never learned tmux was broken)
- tempChdir restore failures are reported to stderr instead of swallowed
- gh/glab errors get an install hint appended when the binary is missing from
  PATH ("exit status 1" alone is a dead end for new users)
