---
title: Fix lint debt surfaced by repaired CI lint job
---

# Fix lint debt surfaced by repaired CI lint job

Lint was red (config/binary mismatch) from 2026-06-05 until PR #23 fixed the
action; first working run surfaced 8 real findings to clear so lint gates PRs
again: unchecked Body.Close x2, capitalized error string, unused cleanupPiece,
S1016 struct conversion, QF1012 Fprintf x3.
