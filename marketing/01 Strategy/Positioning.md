---
type: positioning
status: hypothesis
updated: 2026-09-13
---

# Positioning

## Working category

Git workflow manager for developers running several changes at once.

## Working promise

**Run several changes at once without branch juggling or tool lock-in.**

Monkeypuzzle gives every piece of work an isolated Git worktree. Build a
reviewable branch stack inside it, open the workspace with the editor or coding
tool you already use, and automate lifecycle transitions with shell hooks.
Everything remains ordinary Git.

## Pillars

1. **Isolated:** one durable worktree sandbox per piece of concurrent work.
2. **Reviewable:** linear branch/PR stacks without multiplying worktrees.
3. **Composable:** shell hooks, JSON, schemas, and opener templates.
4. **Unopinionated about actors:** humans, editors, terminals, and coding agents
   use the same workflow surface.

## Proof to show

- Install and create the first piece in under ten minutes.
- Open two pieces side by side with different tools.
- Add a branch to a piece's stack without creating another worktree.
- Show the resulting ordinary branches and PR bases.
- Put a real policy or setup action in a lifecycle hook.

## Tone

Direct, compact, technically credible, and a little playful. Use the puzzle
metaphor to clarify the model, never to obscure Git behavior.

## Avoid

“Revolutionary,” “AI-powered,” “all-in-one,” “platform,” productivity claims
without measurements, and any claim that a planned PaaS already exists.
