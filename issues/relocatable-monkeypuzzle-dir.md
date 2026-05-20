---
title: Relocatable .monkeypuzzle directory
status: in-progress
description: Allow a repo's monkeypuzzle state directory to live somewhere other than ".monkeypuzzle" (e.g. ".DONOTCOMMIT/monkeypuzzle"), with the repo->dir mapping recorded in ~/.config/monkeypuzzle/project-dirs.json. Adds `mp init --dir` and a new `mp move` command.
---

# Relocatable .monkeypuzzle directory

`.monkeypuzzle/` (config, `pieces/` worktrees, `hooks/`, per-piece metadata) used to
be hardcoded at the repo root. Now it can be relocated per-repo — e.g. into an
already-gitignored `.DONOTCOMMIT/monkeypuzzle` — so the tracked tree stays clean
without editing `.gitignore`. The override lives in `~/.config/monkeypuzzle/project-dirs.json`
(it can't live in `monkeypuzzle.json`, which is inside the directory being relocated).

Plan file: `~/.claude/plans/i-want-to-be-tranquil-waffle.md`

## Design

- New package `internal/projectdir` — single source of truth: `RelDir/Dir/PiecesDir/HooksDir/ConfigFilePath(repoRoot)`,
  `Set(repoRoot, rel)`, `MainRepoRoot(path)`, `WorktreeDir(worktreePath)`. Mapping file:
  `~/.config/monkeypuzzle/project-dirs.json` → `{ "version":"1", "dirs": { "<abs repo root>": "<relative dir>" } }`.
  Default (no entry) = `.monkeypuzzle`. Corrupt mapping file falls back to the default.
- `paths.ConfigDir()` gained an `MP_CONFIG_DIR` env override (mirrors `MP_DATA_DIR`) for test isolation.
- Per-piece state dirs inside worktrees mirror the relocation (`<worktree>/<rel>/…`); `WorktreeDir`
  resolves the main repo via `git rev-parse --git-common-dir`, falling back to `<worktree>/.monkeypuzzle`
  for non-git paths (keeps existing unit tests green).
- `mp init --dir <relpath>` — creates the dir at that location and records the mapping (also exposed via
  the `dir` key in `--schema` / stdin JSON). Records nothing for the default; warns if not in a git repo.
- `mp move <relpath>` — relocates an existing project: `os.Rename` the dir, `git worktree repair` the
  relocated piece worktrees, move each per-piece `<rel>` dir, then `projectdir.Set` (which removes the
  entry when moved back to `.monkeypuzzle`). CLI modes: arg / `--path` / stdin JSON / `--schema`.
- Migrated all hardcoded references: `registry.ConfigPath`, `projectdir.PiecesDir/HooksDir/WorktreeDir`
  in `internal/core/piece` (handler, hooks, issue, piece_metadata, pr_metadata), `internal/core/pr`,
  `internal/core/project`, `cmd/mp/switch.go`. Removed `paths.PiecesDir`, `piece.HooksDir`, `registry.ConfigRelPath`.

## Out of scope
- Editing the repo's root `.gitignore` (relocating into `.DONOTCOMMIT/` assumes that path is already ignored;
  the in-dir `.gitignore` for `pieces/` etc. is still created as before).
- Git history preservation for the move (`os.Rename`, not `git mv`); `mp move` prints a note to `git add -A` if needed.

## Work

- [x] `internal/projectdir` package + unit tests
- [x] `paths.ConfigDir` `MP_CONFIG_DIR` override
- [x] migrate all call sites; remove `paths.PiecesDir` / `piece.HooksDir` / `registry.ConfigRelPath`
- [x] `mp init --dir` (flag, input field, validation, schema, mapping persist)
- [x] `mp move` command + integration test
- [x] update stale tests (`TestHooksDir_Constant` → `TestHooksDir_Default`, `ConfigExists`/`EnsureGitignore` signatures)
- [ ] docs: mention `--dir` and `mp move` in docs/commands.md
- [ ] (follow-up) interactive prompt for `dir` in the `mp init` TUI
- [ ] PR
