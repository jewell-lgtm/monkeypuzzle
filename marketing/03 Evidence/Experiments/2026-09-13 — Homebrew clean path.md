---
type: experiment
status: passed
owner: engineering
started: 2026-09-13
decide_by: 2026-09-15
---

# Homebrew clean path

## Observation behind the test

The public install command was displayed on the site, but no tap or GitHub
release existed, so Homebrew could not resolve the formula.

## Hypothesis

A tagged release workflow can publish `mp` and `mp-mcp` into a public tap, then
complete a clean core-workflow smoke test on macOS and Linux without private
machine state.

## Smallest test

- Create `jewell-lgtm/homebrew-tap`.
- Validate the GoReleaser configuration on every relevant pull request.
- On a `v*` tag, run all Go tests, publish both binaries and the formula, install
  it on clean macOS and Linux runners, initialize a disposable repository,
  create a piece, and append an in-worktree branch.

## Primary measure and threshold

Both Homebrew smoke jobs pass from the public tap with zero manual repair.

## Guardrails

Use a fine-grained token scoped only to the tap. Never reuse the founder's broad
GitHub CLI OAuth token as a CI secret.

## Results

`v0.1.0` published on 2026-09-13 with four archives and checksums. The public
tap received the generated formula. Clean GitHub-hosted macOS and Linux runners
installed the formula, initialized a disposable repository, created a piece,
and appended an in-worktree branch. Both jobs passed without manual repair.

## Decision

Passed. Move the active constraint from distribution to design-partner
recruitment. Continue running the smoke matrix on every tagged release.
