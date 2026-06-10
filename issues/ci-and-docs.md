---
title: CI hardening + stale doc cleanup for beta
---

# CI hardening + stale doc cleanup for beta

- CI now runs the integration-tagged suite (documented in CONTRIBUTING but
  never executed), with -race, on ubuntu + macos
- Deleted .claude/skills/monkeypuzzle (hand-written duplicate of the generated
  managing-monkeypuzzle skill; documented removed commands like mp dash and
  mp issue list)
- Regenerated managing-monkeypuzzle skill; template now documents mp stack
  (status/sync/append/prepend/set-parent/continue/undo)
- work-on-issue skill + CLAUDE.md updated: issues are plain markdown authored
  directly; mp only resolves them
