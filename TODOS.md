# TODOS

## P2 — Define "agent output" mechanically
- **What:** Specify how mp distinguishes agent-created PRs/branches from human ones (candidates: lifecycle hooks stamp pieces as agent-created; PR label fallback for branches created outside mp; branch-naming convention as last resort).
- **Why:** Every follow-on vessel (landing queue, `mp land` TUI, GitHub bot) must filter agent output; today nothing defines the marker. Surfaced by cross-model review of the Evidence Sprint plan (2026-07-01).
- **Pros:** Unblocks any GO-verdict build on day 1; sharpens demo narrative.
- **Cons:** Mooted if the sprint verdict is ICE.
- **Context:** Sprint plan: `.DONOTCOMMIT/evidence-sprint-plan.md`; design doc: `~/.gstack/projects/jewell-lgtm-monkeypuzzle/mattjewell-main-design-20260701-225142.md`. `internal/core/piece` owns piece lifecycle; hooks fire at transitions (see apps/mp README).
- **Effort:** M (human) → S (CC). **Priority:** P2. **Blocked by:** sprint go/no-go (2026-07-31).
