---
type: strategy
status: active
updated: 2026-09-13
---

# Product Truth

## Product boundary

Monkeypuzzle (`mp`) is a sharp, terminal-first development workflow manager. It
organizes concurrent Git work and hands the actual editing, coding, reviewing,
and communication to tools developers already use.

## Verified capabilities after the stacked-branch refactor

- A piece owns one isolated Git worktree.
- A linear stack of branches and PRs can live inside that piece's worktree; the
  worktree remains on the stack tip.
- GitHub and GitLab PR/MR operations use `gh` and `glab`.
- Lifecycle hooks make transitions scriptable.
- Commands support flags, JSON input, and schema introspection.
- `mp open` and `--with` hand a piece to an editor, terminal, or coding tool.
- Multiplexers and the MCP bridge are integrations, not prerequisites.

## Not yet a public promise

- First-class puzzle objects as durable epic/objective containers.
- `snap` as a shipped cross-piece dependency command.
- A packaged, discoverable plugin ecosystem. Hooks and opener templates are the
  current extension surfaces.
- Automatic agent orchestration.
- A production PaaS or team coordination product.

## Language constraints

- Say **works with your tools**, not **replaces your tools**.
- Say **scriptable hooks and templates** until plugin packaging exists.
- Say **ordinary Git branches and PRs**; lack of lock-in is part of the product.
- Do not say “one piece, one PR.” A piece can contain a branch/PR stack.
- Keep server and remote capabilities out of the core promise until users pull
  them into the buying/adoption story.
