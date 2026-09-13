---
type: personas
status: hypothesis
updated: 2026-09-13
---

# First User Personas

These are recruiting hypotheses, not demographic truth.

## Primary — parallel senior IC

A senior individual contributor on a two-to-ten-person team who lives in the
terminal, uses GitHub on macOS or Linux, and regularly keeps three or more
changes active. They may use coding agents, but still own the Git and review
workflow.

Signals:

- manually uses `git worktree`, several clones, or named terminal sessions;
- loses time switching branches or reconstructing task context;
- sometimes builds two dependent reviewable changes;
- can adopt a personal CLI without a team procurement decision.

Disqualifiers: only one change at a time, unwilling to use the terminal, seeking
a hosted project tracker, or unable to try a new workflow on a real repository.

## Secondary — workflow toolsmith

A team lead or platform-minded engineer who wants hooks, policies, and reusable
tool-launch templates. Strong extensibility fit; likely to demand a stable
contract and excellent reference documentation.

## Secondary — stacked-PR power user

Already uses Graphite, git-town, or hand-built branch stacks. Understands the
problem immediately, but needs a compelling worktree/isolation advantage to
switch from an established workflow.

## Research segment — multi-agent solo builder

Runs several coding tools concurrently and needs clean, durable sandboxes. The
pain may be severe, but Monkeypuzzle must remain actor-agnostic rather than
becoming another agent supervisor.

## Questions the first interviews must answer

- Which current behavior would Monkeypuzzle replace?
- Is isolation or branch stacking the initial hook?
- Does one worktree per piece reduce or increase cognitive load?
- Which integration makes first use feel native?
- What event creates the need for cross-machine or shared visibility?
