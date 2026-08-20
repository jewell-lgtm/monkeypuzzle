# Command Reference

## Input Modes

Every command follows the same interaction contract, in priority order:

| Mode        | When                        | Usage                    |
| ----------- | --------------------------- | ------------------------ |
| Interactive | TTY detected (default)      | Run command with no args |
| Flags       | One or more flags provided  | `mp <cmd> --flag value`  |
| Stdin JSON  | Piped input                 | `echo '{}' \| mp <cmd>`  |
| Example     | `--schema` flag             | `mp <cmd> --schema`      |

In interactive mode a Bubble Tea wizard walks you through the decisions, using
defaults inferred from detected state; commands with several decisions are
multi-step wizards. **Flags skip the corresponding wizard steps** — pass enough
flags and the command runs straight through with no wizard. Stdin JSON is the
fully programmatic path (JSON in, JSON out): there is no separate "agent mode",
the same CLI is the API. `--schema` prints an example input document in the
same shape stdin JSON expects — edit it and pipe it right back in
(`mp init --schema | jq '.name = "x"' | mp init`); it is not a formal JSON
Schema document.

**Ambiguity rule:** non-interactive invocations (flags or JSON) **fail loudly on
genuine ambiguity** rather than prompting or guessing — there is no human to ask.
The wizard is the only place ambiguity gets resolved. (A pure no-op is not
ambiguous.)

Output goes to stderr (human-readable) while stdout is reserved for JSON (machine-readable).

**Remote execution.** Three flags, placed **between `mp` and the verb**, route
a command somewhere else before any of the above happens: `mp --project <name>
<cmd>` runs it against a registered project (proxied over ssh when the registry
entry has a host, from its path when local), and `mp --host <ssh-host> [--dir
<path>] <cmd>` (env: `MP_HOST` / `MP_DIR`) is the raw form. Placement matters:
after the verb these names belong to the verb (`mp init --dir`, `mp switch
--project`). The whole invocation is forwarded verbatim to the `mp` binary on
the host, so the remote surface is byte-identical — flags, stdin JSON, JSON
out. See [Remote development](remote-development.md) and
[`mp remote doctor`](#mp-remote).

**Session management is interactive-only.** mp creates or switches a
multiplexer session **only** when driven interactively — a real terminal on
stdin (isatty) **and** running inside the configured multiplexer (`$TMUX` for
tmux, `$ZELLIJ` for zellij, `$CMUX_WORKSPACE_ID` for cmux). Agents and scripts
(flags / stdin JSON, output captured) have no controlling terminal, so
`create`/`switch`/`go` skip the multiplexer and return the worktree path
instead (in JSON, or on stdout for `cd $(mp switch …)`). The in-session env
vars are inherited by child processes, so they are not sufficient on their
own — the TTY check is what keeps an agent from creating a stray session or
hijacking the human's terminal.

---

## Shell Completion

Enable tab completion for mp commands:

### Bash

```bash
source <(mp completion bash)
# Permanent: echo 'source <(mp completion bash)' >> ~/.bashrc
```

### Zsh

```bash
source <(mp completion zsh)
# Or: mp completion zsh > "${fpath[1]}/_mp"
```

### Fish

```bash
mp completion fish | source
# Permanent: mp completion fish > ~/.config/fish/completions/mp.fish
```

### PowerShell

```powershell
mp completion powershell | Out-String | Invoke-Expression
```

### What completes

| Flag                            | Completes To            |
| ------------------------------- | ----------------------- |
| `mp abandon --name`       | Available piece names   |
| `mp init --pr-provider`         | `github`, `gitlab`      |
| `mp update --main`        | Git branch names        |
| `mp sync --main`          | Git branch names        |
| `mp merge --main`         | Git branch names        |

---

## mp init

Initialize monkeypuzzle in current directory.

### Usage

```bash
mp init                        # Interactive TUI
mp init --name foo             # With flags
echo '{"name":"foo"}' | mp init  # JSON stdin
mp init --schema               # Output schema
```

### Flags

| Flag                 | Description                                                       | Default        |
| -------------------- | ----------------------------------------------------------------- | -------------- |
| `--name`             | Project name                                                      | Directory name |
| `--pr-provider`      | PR provider (`github`, `gitlab`)                                  | `github`       |
| `--dir`              | Directory (relative to repo root) for monkeypuzzle state          | `.monkeypuzzle`|
| `--gitignore`        | Regenerate `<dir>/.gitignore` only (no other changes)             | `false`        |
| `--schema`           | Print an example input document and exit                          | -              |
| `-y, --yes`          | Overwrite existing config                                         | `false`        |

`--dir` lets you keep all monkeypuzzle state somewhere already ignored by git,
e.g. `mp init --dir .DONOTCOMMIT/monkeypuzzle`. The repo→directory mapping is
recorded in `~/.config/monkeypuzzle/project-dirs.json` (it can't live in
`monkeypuzzle.json`, which is inside the directory being relocated). To relocate
an existing project later, use [`mp move`](#mp-move).

`mp reinit` is an alias for `mp init` — same flags, same behavior — for the
explicit "refresh scaffolding in an already-initialized repo" case.

### Example input

```json
{
  "name": "project-name",
  "pr_provider": "github",
  "dir": ".monkeypuzzle"
}
```

### Output

Creates the monkeypuzzle directory (default `.monkeypuzzle/`):

```
.monkeypuzzle/
├── monkeypuzzle.json    # Configuration
└── .gitignore           # Ignores pieces/ and per-piece metadata
```

### Providers

**PR Providers:**

- `github` - PR management via `gh` CLI
- `gitlab` - MR management via `glab` CLI

---

## mp switch

The single switching entry point. Give it whatever you have in your head or clipboard — a piece name, a branch (local, remote, or already adopted), or a brand-new name — and it attaches, adopts, or creates as needed.

### Usage

```bash
mp switch                                          # Interactive: picker scoped to the current project
mp switch --all                                    # Interactive: every registered project (same as `mp go`)
mp switch fix-x                                    # Piece or branch name; resolves in the current repo
mp switch feat/new-idea --create                   # Brand-new name: create piece "new-idea" on that branch
mp switch --project app                            # Attach app's main worktree
mp switch --project app --piece fix-x              # Attach an existing piece (skips resolution)
mp switch --project app --branch spike-token-rotate  # Adopt branch as piece (or attach if already one)
echo '{"target":"fix-x"}' | mp switch
echo '{"target":"feat/new-idea","create":true}' | mp switch
mp switch --schema
```

### Target resolution

A positional `TARGET` resolves in order: `main`/`master` (attach the main worktree) → an existing **piece** by name, sanitized name, or the branch checked out in it (attach) → a local **branch** (adopt, then attach) → a **remote** ref, pasted verbatim (`origin/foo`) or by bare name (fetch + adopt tracking) → **nothing** (create a piece whose branch is `TARGET` verbatim and whose name is derived from it). A piece always beats an unadopted branch of the same name, so switching stays idempotent; `--piece`/`--branch` bypass resolution when you need to be explicit.

Creation is gated: on a terminal you're asked to confirm; non-interactively an unmatched target is an error unless `--create` (or `"create": true` on stdin) is passed — a typo never silently mints a piece.

### Flags

| Flag        | Description                                                            |
| ----------- | ---------------------------------------------------------------------- |
| `--project` | Project name or path (defaults to the repo you're standing in)         |
| `--piece`   | Existing piece to attach (skips target resolution)                     |
| `--branch`  | Git branch to adopt as a piece (attaches if it already is one)         |
| `--create`  | Allow an unmatched target to create a new piece                        |
| `--all`     | Interactive picker across all registered projects                      |
| `--schema`  | Print an example input document and exit                              |

`TARGET`, `--piece`, and `--branch` are mutually exclusive. Omit all of them (with `--project`) to attach that project's main worktree. Without `--project`, mp resolves the project from the current directory — any init'd repo works, registered or not.

### Interactive picker

With a terminal and no selectors, opens a fuzzy-filtered list scoped to the current project (bare `mp` is the same view); `--all` or running outside a project widens it to every registered project:

- **Pieces** — existing worktrees, with a `[<multiplexer>]` indicator if a session is live.
- **Branches** — local git branches available for adoption (excludes main/master, any branch already adopted as a piece, the branch checked out in the main repo, and branches held by locked worktrees). A branch checked out in a worktree mp doesn't manage — e.g. one created by an agent — is offered too; adopting it relocates that worktree into the pieces dir. Selecting one runs `mp adopt`.

Type to filter, ↑/↓ to move, `enter` to select, `esc` to cancel. The picker caps the visible rows at 20; narrow the query to surface anything below the cut.

### What it does

1. Resolves the project: the registry for an explicit `--project`, else the repo the caller is standing in.
2. Based on the chosen selector:
   - **target** — runs the resolution above, then attaches / adopts / creates
   - **piece** — locates the worktree and attaches its multiplexer session
   - **branch** — attaches the piece holding that branch, else runs `AdoptPiece` with `repo_root` set to the project path, then attaches
3. Attaching only happens when run **interactively from inside the configured
   multiplexer** (real TTY on stdin *and* the adapter's in-session env var set).
   When no multiplexer is configured, or when called by an agent/script or from
   a terminal outside it, it prints the worktree path instead so
   `cd $(mp switch ...)` works.

### Non-interactive shape

`mp go --json` (and the JSON form of `mp switch` when stdout isn't a TTY) includes per-project `pieces` and `branches` arrays so callers can build their own pickers.

---

## mp move

Relocate the current repository's monkeypuzzle directory to a new path relative
to the repo root, updating the mapping in `~/.config/monkeypuzzle/project-dirs.json`.

### Usage

```bash
mp move .DONOTCOMMIT/monkeypuzzle          # relocate
mp move --path .DONOTCOMMIT/monkeypuzzle   # same, via flag
echo '{"path":".DONOTCOMMIT/monkeypuzzle"}' | mp move
mp move .monkeypuzzle                      # move back to the default (clears the mapping entry)
mp move --schema
```

### What it does

1. Resolves the repo root (works from a subdirectory or inside a piece worktree).
2. `os.Rename`s the monkeypuzzle directory to the new location.
3. Runs `git worktree repair` on the relocated piece worktrees so git keeps tracking them.
4. Moves each piece's per-piece state directory to mirror the new layout.
5. Records the repo→directory mapping (or removes it when moved back to `.monkeypuzzle`).

Git tracking is left untouched — if `monkeypuzzle.json` was committed, run `git add -A`
for the move yourself. The repo's root `.gitignore` is not modified; relocating into
`.DONOTCOMMIT/` assumes that path is already ignored.

### Output

JSON to stdout with `repo_root`, `old_dir`, `new_dir`, `old_path`, `new_path`, and the
list of `pieces` that were moved/repaired. Human-readable summary to stderr.

---

## mp flatten

Remove **all** piece worktrees for the current repository, returning it to a flat
main-only state. Each piece's multiplexer session is killed and its worktree removed.

Unlike `mp cleanup` (which only removes _merged_ pieces), flatten removes every
piece regardless of merge status. Branches are kept by default.

Dry-run by default, like the other sweep operations (`mp cleanup`,
`mp stack sync`): it previews what would be removed and changes nothing. Pass
`--apply` to actually flatten; in an interactive terminal you see the preview
and confirm (`--yes`/`-y` skips the prompt and applies).

### Usage

```bash
mp flatten                       # Preview (interactive terminal: preview + confirm)
mp flatten --apply               # Remove all pieces
mp flatten -y                    # Remove all (skip confirmation)
mp flatten --apply --force       # Also discard uncommitted changes
mp flatten --delete-branches     # Also delete each piece's git branch (with --apply)
echo '{"apply":true}' | mp flatten
mp flatten --schema
```

### Flags

| Flag                | Description                                       | Default |
| ------------------- | ------------------------------------------------- | ------- |
| `--apply`           | Apply the flatten (default is a dry-run preview)  | `false` |
| `--yes`, `-y`       | Skip the confirmation prompt (implies `--apply`)  | `false` |
| `--force`           | Force removal even with uncommitted changes       | `false` |
| `--delete-branches` | Also delete each piece's git branch               | `false` |
| `--dry-run`         | Force a preview even with `--apply`-style stdin   | `false` |

### What it does

1. Lists all pieces for the repo (works from the main repo or from inside a piece)
2. Previews; in an interactive terminal, asks for confirmation (skip with `--yes`), and non-interactive callers only apply with `--apply` / `"apply": true`
3. For each piece: switches you to the main session if you're inside it, kills the
   multiplexer session, and removes the worktree (use `--force` to discard uncommitted changes)
4. Optionally deletes each piece's branch (`--delete-branches`)
5. Continues past individual failures, reporting them under `failed`

### Output

```json
{
  "removed": [
    { "piece_name": "piece-a", "worktree_path": "/…/pieces/abc123/piece-a", "branch_name": "piece-a" },
    { "piece_name": "piece-b", "worktree_path": "/…/pieces/abc123/piece-b", "branch_name": "piece-b" }
  ],
  "count": 2,
  "main_path": "/home/user/repo"
}
```

Pieces that could not be removed (e.g. uncommitted changes without `--force`) appear in a
`failed` array with an `error` message instead.

---

## mp status

Show a piece's status. Defaults to the piece you're standing in (or the main repo); name one positionally or with `--piece` to inspect it from anywhere in the repo.

### Usage

```bash
mp status
mp status my-feature
```

### Output

JSON to stdout:

```json
{
  "in_piece": true,
  "piece_name": "piece-20241226-143022",
  "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/abc123def456/piece-20241226-143022",
  "repo_root": "/home/user/projects/myproject"
}
```

Human-readable message to stderr.

---

## mp create

Create a new piece (git worktree + multiplexer session).

### Usage

```bash
mp create
mp create --name my-feature
mp create --prompt "add dark mode"        # name auto-generated from the prompt
mp create --parent parent-piece           # stack on another piece
mp create --skip-switch  # Don't auto-switch to new piece
mp create --remote wire --name fix-auth   # place the piece on the ssh box "wire"
```

### Flags

| Flag                  | Description                                       | Default        |
| --------------------- | ------------------------------------------------- | -------------- |
| `--name`              | Custom piece name                                 | Auto-generated |
| `--prompt`            | Create from a prompt (name auto-generated)        | -              |
| `-p, --parent`        | Parent piece name to branch from (stacks the piece) | `main`       |
| `--skip-switch`       | Don't switch to the new piece after creation      | `false`        |
| `--overwrite-session` | Replace existing main repo multiplexer session    | `false`        |
| `--agent`             | Launch an agent in the new piece: `claude` or `codex`. With a session, the launch line is typed into it; without one it runs headless with `--prompt` (output to `.monkeypuzzle/logs/`) | - |
| `--remote`            | Place the piece on this ssh box: the worktree, hooks, agent and PR live there, the project stays here. First use clones + `mp init`s the repo on the box under `~/.local/share/mp/<project>`. `--parent` must be `main` or a piece already on the same box. Also `"remote"` in stdin JSON. See [Remote development](remote-development.md#placing-a-piece-on-a-box) | - |

### What it does

1. Detects current git repository root
2. Generates piece name: `piece-YYYYMMDD-HHMMSS` (or uses `--name`)
3. Creates git worktree at `~/.local/share/monkeypuzzle/pieces/<piece-name>`
4. Fires the `on-piece-create.sh` hook (if exists) in the background — see below
5. **Switches to the new piece** (unless `--skip-switch` is set) — but only when
   run interactively (see below); otherwise prints the worktree path

The `on-piece-create.sh` hook is **fire-and-forget**: it runs detached in the
background so its setup work (dependency installs, submodule init) never blocks
piece creation, and `mp create` returns immediately. The hook's combined output
is redirected to `.monkeypuzzle/logs/on-piece-create-<piece-name>.log`, and the
path is printed when the hook starts. Only a failure to *start* the hook
produces a warning; its exit status is not observed (check the log instead). The
worktree is always kept regardless of how the hook fares.

The auto-switch only manages a session when run **interactively from inside the
configured multiplexer** (a real TTY on stdin *and* the adapter's in-session env
var set): it moves your existing client/tab/workspace to the piece's session.
Run by an agent or script, or from a terminal outside the multiplexer, it
creates no session and prints the worktree path instead — so `--skip-switch` is
only needed to suppress switching in an interactive session.

### Output

JSON to stdout:

```json
{
  "name": "piece-20241226-143022",
  "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/abc123def456/piece-20241226-143022",
  "session_name": "mp-piece-piece-20241226-143022"
}
```

### Piece storage

Pieces are stored in repo-scoped directories within the XDG data directory:

- Linux: `~/.local/share/monkeypuzzle/pieces/{repo-hash}/`
- macOS: `~/Library/Application Support/monkeypuzzle/pieces/{repo-hash}/`
- `$XDG_DATA_HOME/monkeypuzzle/pieces/{repo-hash}/` if set

The `{repo-hash}` is a unique identifier derived from the repository's absolute root path. This ensures:

- Each repository has its own isolated pieces directory
- No naming conflicts between different repositories
- Easier management of pieces per project

---

## mp list

List pieces for the current repo as a tree (parent → child) or a flat list.

### Usage

```bash
mp list           # tree view (human readable)
mp list --flat    # flat list (JSON-friendly)
mp list --all     # across all registered projects
```

### Flags

| Flag     | Description                                      | Default |
| -------- | ------------------------------------------------ | ------- |
| `--flat` | Flat list instead of the tree view               | `false` |
| `--all`  | List pieces across all registered projects       | `false` |

### Output (`--flat`)

```json
[
  { "name": "feature-auth", "worktree_path": "/path", "parent": "main", "mod_time": "2025-01-04T10:00:00Z" },
  { "name": "auth-oauth", "worktree_path": "/path", "parent": "feature-auth", "mod_time": "2025-01-04T11:00:00Z" },
  { "name": "fix-auth", "worktree_path": "/home/u/.local/share/mp/api/.monkeypuzzle/pieces/fix-auth", "host": "wire", "state": "unknown", "parent": "main" }
]
```

Pieces placed on a box (`mp create --remote`, see [Remote development](remote-development.md)) are listed too: `host` names the box, `worktree_path` is a path **on the box**, and `state` is `unknown` until refreshed or `pending` while the remote create is in flight. Local pieces carry neither field.

---

## mp update

Merge main branch into current piece.

### Usage

```bash
mp update                  # Merge from 'main'
mp update --main develop  # Merge from 'develop'
```

### Flags

| Flag            | Description          | Default |
| --------------- | -------------------- | ------- |
| `--main` | Branch to merge from (`--main-branch` is a deprecated alias) | `main`  |

### Requirements

- Must be run from within a piece worktree

### What it does

1. Verifies you're in a piece worktree
2. Runs `before-piece-update.sh` hook (if exists)
3. Merges specified branch into current piece branch
4. Runs `after-piece-update.sh` hook (if exists)
5. Reports success/failure

If any hook fails, the operation is aborted.

---

## mp sync

Sync the current piece with its **parent** (from piece metadata — another piece,
or main for root pieces). Defaults to **origin's version** of the parent:
`origin/<parent>` is fetched and merged. The local parent branch is used only
when origin doesn't have it (or no origin is configured), or with `--local`.

For whole-stack syncing, see [`mp stack sync`](#mp-stack).

### Usage

```bash
mp sync                        # Merge origin/<parent> into the piece
mp sync --local                # Merge the local parent branch instead
mp sync --from upstream/main   # Merge an explicit ref
echo '{"local":true}' | mp sync
```

### Flags

| Flag            | Description                                            | Default |
| --------------- | ------------------------------------------------------ | ------- |
| `--main` | Trunk branch name, used when the piece's parent is main (`--main-branch` is a deprecated alias) | `main`  |
| `--from`        | Explicit ref to sync from (fetched when remote)        | —       |
| `--local`       | Use the local parent branch, skip origin               | `false` |

### Requirements

- Must be run from within a piece worktree

### What it does

1. Reads the piece's parent from metadata (`main` → `--main`)
2. Resolves the ref: `--from` override, else `origin/<parent>` when origin has
   the branch (fetched first), else the local parent branch
3. Runs `before-piece-update.sh` hook (if exists)
4. Merges the resolved ref into the current piece branch
5. Runs `after-piece-update.sh` hook (if exists)

### Output

```json
{
  "piece_name": "auth-oauth",
  "parent": "feature-auth",
  "merged_ref": "origin/feature-auth",
  "source": "origin",
  "status": "synced"
}
```

`source` is `origin`, `local`, or `override` (`--from`). If origin is
configured but unreachable, `mp sync` warns and falls back to the local parent.

---

## mp merge

Merge piece back to main branch.

### Usage

```bash
mp merge                   # Merge to 'main'
mp merge --main develop  # Merge to 'develop'
```

### Flags

| Flag                   | Description                                                              | Default |
| ---------------------- | ------------------------------------------------------------------------ | ------- |
| `--main`                | Branch to merge into (`--main-branch` is a deprecated alias)             | `main`  |
| `--force`              | Merge even if the piece has child pieces (children are **not** re-homed) | `false` |
| `--reparent-children`  | Merge a piece with children, re-homing them onto the merge target        | `false` |
| `--reparent-strategy`  | How to re-home children: `rebase` (rewrites history) or `merge` (no force-push) | `rebase` |

### Requirements

- Must be run from within a piece worktree
- **Main branch must not be ahead** - Fails if main has commits not in piece
- **No unmerged child pieces** - Fails if the piece has children, unless you pass `--reparent-children` (re-homes them) or `--force` (leaves them orphaned)

### What it does

1. Verifies you're in a piece worktree
2. Runs `before-piece-merge.sh` hook (if exists)
3. Checks main branch isn't ahead (safety check)
4. Switches to main branch in main repository
5. Merges piece branch into main
6. Runs `after-piece-merge.sh` hook (if exists)
7. Reports success/failure

If any hook fails, the operation is aborted.

### Safety check

If main has commits not in the piece, merge fails. Run `mp update` first to incorporate those changes.

---

## mp cleanup

Remove worktrees for merged pieces and prune deleted projects. Aliased as `mp repair`.

**Dry-run by default.** With no `--apply`, cleanup only previews what would be
removed. Pass `--apply` (or `"apply": true`) to actually clean up. In an
interactive terminal you are shown the preview and asked to confirm.

### Usage

```bash
mp cleanup              # Preview what would be cleaned (dry-run)
mp cleanup --apply      # Actually remove merged pieces / prune projects
mp cleanup --dry-run    # Explicit preview (also suppresses the confirm prompt)
echo '{"apply":true}' | mp cleanup
```

### Flags

| Flag            | Description                                       | Default |
| --------------- | ------------------------------------------------- | ------- |
| `--apply`       | Apply the cleanup (default is a dry-run preview)  | `false` |
| `--yes`, `-y`   | Skip the confirmation prompt (implies `--apply`)  | `false` |
| `--dry-run`     | Preview only; never prompt, never change anything | `false` |
| `--main` | Main branch to check merge status against (`--main-branch` deprecated) | `main`  |

`--force` was removed: on cleanup it used to mean "apply", clashing with its
meaning everywhere else (override a safety check). Use `--apply` or `--yes`.

### What it does

1. Scans pieces directory for worktrees
2. Checks if each piece's branch is merged (via git branch, PR, or remote)
3. Previews the merged pieces and stale projects that would be removed
4. With `--apply` (or an interactive confirmation): removes each worktree, kills
   its multiplexer session, and prunes registry entries for deleted projects

---

## mp abandon

Remove an unmerged piece (worktree, multiplexer session, optionally branch).

### Usage

```bash
mp abandon                              # The piece you're standing in
mp abandon my-feature                   # By name
mp abandon my-feature --force           # Discard uncommitted changes
mp abandon foo --delete-branch          # Also delete git branch
```

### Flags

| Flag              | Description                                    | Default |
| ----------------- | ---------------------------------------------- | ------- |
| `--piece`         | Piece to abandon (or pass it positionally)     | current |
| `--name`          | Deprecated alias for `--piece`                 | -       |
| `--force`         | Force removal even with uncommitted changes    | `false` |
| `--delete-branch` | Also delete the git branch                     | `false` |

### What it does

1. Finds the piece by the positional/`--piece` selector, else the piece the caller is standing in
2. Kills the multiplexer session if it exists
3. Removes the git worktree (use `--force` to discard uncommitted changes)
4. Optionally deletes the git branch (`--delete-branch`)

### Output

```json
{
  "piece_name": "my-feature",
  "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/abc123def456/my-feature",
  "branch_name": "my-feature",
  "branch_deleted": true
}
```

---

## mp pr create

Create a pull/merge request for the current piece. Pushes the branch to origin and opens the PR/MR via the configured provider's CLI — `gh` for GitHub or `glab` for GitLab (set `pr_provider` at `mp init`). Run from within a piece worktree.

### Usage

```bash
mp pr create                                  # title/body from piece name
mp pr create --title "Add login" --body "..."
mp pr create --base develop                   # override the base branch
echo '{"title":"Add login","body":"..."}' | mp pr create
mp pr create --schema
```

### Flags

| Flag      | Description                                              | Default                         |
| --------- | ------------------------------------------------------- | ------------------------------- |
| `--title` | PR title                                                | Piece name                      |
| `--body`  | PR description                                           | -                               |
| `--base`  | Base branch to merge into                               | Auto-detect from parent piece   |

For a stacked piece, the base auto-detects to the parent piece's branch so the PR targets the right branch in the stack.

### Output

```json
{ "url": "https://github.com/owner/repo/pull/123", "number": 123 }
```

---

## mp pr ready

Flip the current piece's draft PR/MR to ready-for-review. Reads the PR number
from `.monkeypuzzle/pr-metadata.json` and fires the `before-pr-ready` /
`after-pr-ready` hooks around the provider call. Run from within a piece
worktree.

### Usage

```bash
mp pr ready
echo '{}' | mp pr ready     # stdin accepted for uniformity (no fields)
mp pr ready --schema        # prints {} — ready takes no input
```

| Flag       | Description                          |
| ---------- | ------------------------------------ |
| `--json`   | Output `{"status":"ready"}` even on a terminal |
| `--schema` | Print an example input document and exit |

---

## mp done

Clean up a piece (worktree + multiplexer session) after its branch has been merged. Defaults to the piece you're standing in; name one positionally or with `--piece` to finish it from anywhere in the repo. Verifies the piece is merged first — use [`mp abandon`](#mp-abandon) for unmerged pieces.

### Usage

```bash
mp done
mp done my-feature
mp done --main develop
```

### Flags

| Flag            | Description                                   | Default |
| --------------- | --------------------------------------------- | ------- |
| `--piece`       | Piece to finish (or pass it positionally)     | current |
| `--main` | Main branch to check merge status against (`--main-branch` deprecated) | `main`  |

---

## mp adopt

Convert an existing git branch into a piece worktree. Accepts a local branch name or a remote ref like `origin/foo` (remote refs are fetched and a tracking branch is created). Run from the main repo with no branch to adopt the current branch; from inside a piece worktree `--branch` is required.

Adopt does not require a clean working directory: it always creates a *separate* worktree, so uncommitted changes in the main checkout are left untouched. When the branch being adopted is the one currently checked out in the main worktree, adopt frees it by resetting the main worktree back to its main branch (`main`, or `master`), carrying any uncommitted work-in-progress along into the new piece worktree.

A branch checked out in a worktree mp doesn't manage — e.g. one created by an agent's worktree isolation (Claude Code) or a manual `git worktree add` — is adopted by relocating that whole worktree into the pieces dir (`git worktree move`), uncommitted changes and all. A stale worktree record whose directory has been deleted is pruned automatically before adopting. Two cases still refuse: a branch that is already a piece (use `mp switch`), and a branch held by a locked worktree (`git worktree unlock` it first).

When run interactively from inside the configured multiplexer, adopt creates and switches to the new piece's session (like `mp switch`). For agents/automation it leaves the multiplexer untouched and reports the worktree path in the result JSON.

### Usage

```bash
mp adopt                       # adopt the current branch (from main repo)
mp adopt my-spike              # adopt a local branch
mp adopt --branch origin/foo   # fetch + adopt a remote branch
mp adopt my-spike --parent feature-a   # adopt as a child of another piece
echo '{}' | mp adopt
```

### Flags

| Flag           | Description                                              | Default               |
| -------------- | ------------------------------------------------------- | --------------------- |
| `-b, --branch` | Branch to adopt; local name or remote ref `origin/foo`  | Current branch on main |
| `--name`       | Override piece name                                     | Branch name           |
| `-p, --parent` | Parent piece name                                       | `main`                |

---

## mp stack

Whole-stack operations over pieces stacked via `--parent` (git-town-style). All operations are non-interactive: anything risky aborts cleanly and prints plain-English next steps (e.g. which PR base to change on GitHub).

### mp stack status

Show the stack tree, PR/MR state, and drift vs the forge's PR/MR list.

```bash
mp stack status
mp stack status --from-remote   # rebuild local lineage from open PR/MR bases
mp stack status --apply-bases   # edit PR/MR bases on the forge to match local lineage
```

| Flag            | Description                                          | Default |
| --------------- | ---------------------------------------------------- | ------- |
| `--from-remote` | Rebuild local lineage from open PR/MR bases          | `false` |
| `--apply-bases` | Edit PR/MR bases on the forge to match local lineage | `false` |
| `--main`        | Main branch name                                     | `main`  |

(`--from-github` remains as a hidden, deprecated alias for `--from-remote`.)

### mp stack sync

Propagate main and each parent down through the stack.

**Dry-run by default.** With no `--apply`, sync only previews which pieces would
be synced. Pass `--apply` (or `"apply": true`) to actually sync. In an
interactive terminal you are shown the preview and asked to confirm.

**Sync source.** Sync first updates local main from an upstream ref (a fetch and
fast-forward) before propagating it down the stack. That ref is `--from`,
defaulting to `origin/<main>`. In an interactive terminal you are prompted for it
(enter to accept the default); non-interactive callers use the default. A ref
whose remote isn't configured (e.g. `origin/main` in a local-only repo) makes the
main update a no-op.

```bash
mp stack sync                     # preview which pieces would be synced (dry-run)
mp stack sync --apply             # actually sync the stack
mp stack sync --from upstream/main --apply   # sync main from a different remote
mp stack sync --strategy rebase --apply   # rebase instead of the default merge
mp stack sync --push --apply      # push each branch after syncing
mp stack sync --stack             # limit the preview to the current piece's stack
```

| Flag         | Description                                          | Default       |
| ------------ | ---------------------------------------------------- | ------------- |
| `--apply`    | Apply the sync (default is a dry-run preview)        | `false`       |
| `--dry-run`  | Preview only; never prompt, never change anything    | `false`       |
| `--from`     | Upstream ref to update main from (fetch + fast-forward) | `origin/<main>` |
| `--strategy` | Sync strategy: `merge` or `rebase`                   | `merge`       |
| `--push`     | Push each branch after syncing                       | `false`       |
| `--stack`    | Limit to the current piece's stack (run from a piece) | `false`      |
| `--main`     | Main branch name                                     | `main`        |

### mp stack continue

Resume a conflicted rebase started by `mp stack sync --strategy rebase` (after resolving conflicts).

```bash
mp stack continue
```

### mp stack undo

Restore every piece branch to the snapshot `mp stack sync` took right before its last run. Local only — remote branches are untouched; force-push with lease afterwards if you'd already pushed. Refuses to run if an affected worktree has uncommitted changes.

```bash
mp stack undo
```

### mp stack set-parent

Re-point a piece onto a different parent — metadata only; run `mp stack sync` afterwards to actually restack the branches onto the new lineage. Defaults to the current piece when run from inside one.

```bash
mp stack set-parent --parent other-piece
mp stack set-parent --piece child-feat --parent main   # make it a root piece
```

| Flag       | Description                                   | Default        |
| ---------- | ---------------------------------------------- | -------------- |
| `--piece`  | Piece to re-parent                              | Current piece  |
| `--parent` | New parent piece name, or `main`                | -              |

### mp stack graph

Reconstruct a repository's stacked-PR forest straight from the forge's open PRs' base→head edges — no local clone required. Auth comes from the ambient `GH_TOKEN`/`GITHUB_TOKEN` (or `GITLAB_TOKEN`) environment, so a server can run this as a specific user. This is the same forest the hosted dashboard renders — both go through the shared stackgraph builder.

```bash
mp stack graph --repo owner/name
mp stack graph --repo owner/name --provider gitlab --default-branch develop
```

| Flag               | Description                                    | Default   |
| ------------------ | ----------------------------------------------- | --------- |
| `--repo`           | Repository as `owner/name` (required)           | -         |
| `--default-branch` | Trunk branch                                    | Auto-detected from the forge |
| `--provider`       | Forge provider: `github` or `gitlab`            | `github`  |
| `--limit`          | Max PRs to fetch                                | `200`     |

### mp stack append / prepend

`append` creates a new piece as a child of the current piece; `prepend` inserts a new piece between the current piece and its parent.

```bash
mp stack append --name child-feat
mp stack append --prompt "add caching layer"
mp stack prepend --name base-feat
```

| Flag       | Description                          | Default        |
| ---------- | ------------------------------------ | -------------- |
| `--name`   | Piece name                           | Auto-generated |
| `--prompt` | Piece prompt (recorded in piece metadata; used to name the piece) | -              |

---

## mp project

Manage the global registry of monkeypuzzle projects. A "project" is any git repo initialised with `mp init` (which registers it automatically). Registering projects lets `mp` list pieces across all of them and jump between their multiplexer sessions. Aliases: `projects`, `proj`.

### Usage

```bash
mp project add                       # register the current directory
mp project add /path/to/repo
mp project add wire:code/api         # scp-style: a project on an ssh host
echo '{"path":"/repo"}' | mp project add
echo '{"host":"wire","path":"code/api"}' | mp project add

mp project list                      # human-readable table (alias: ls, status)
mp project list --json               # machine output
mp project list --all                # include hidden rows (box-side clones of placed pieces)

mp project remove my-project         # unregister (alias: rm); repo on disk untouched
mp project remove --target /path/to/repo
```

`mp project list` shows best-effort live state per project (current branch, number of pieces). Remote projects show as `(remote)` with a `host:path` location; their JSON rows carry a `"host"` field. Rows with `"hidden": true` are bookkeeping for placed pieces (`mp create --remote`, see [Remote development](remote-development.md)) — shown as `(hidden)` only with `--all`; their `"linked_from"` is the controller-side repo root. The `HOST:PATH` form resolves the path to an absolute path on the host at add time and requires the repo to already be `mp init`-ed there — see [Remote development](remote-development.md).

---

## mp go

A **repo switcher**: jump to any registered project's worktree from anywhere. With a terminal it opens an interactive fuzzy picker where each repo starts **collapsed** (one row per repo). Press `→` to expand a repo and reveal its pieces and branches; `←` collapses it again. Pressing `Enter` on a collapsed repo jumps straight to its **main worktree**. Typing filters across everything (collapsed or not), and the list scrolls (`↑/↓`, `PgUp/PgDn`), sizing its window to the terminal height. A single registered repo starts expanded.

With `--json` (or no TTY) it prints the **full per-project detail** (`pieces`, `branches`) so automation can build its own pickers.

Bare `mp` opens a fuzzy picker **scoped to the current project** (repo-local) — it shows the pieces and branches of the project you're standing in. When run **outside** a monkeypuzzle project, bare `mp` prints context-aware guidance to stderr and then falls through to the cross-project picker (when you have registered projects to jump to):

- Inside a git repo that hasn't been initialised → suggests `mp init`, then shows the picker.
- Outside any git repo → suggests cd-ing into a repo and running `mp init`, then shows the picker.
- In JSON / non-TTY mode the guidance and the full project detail arrive in one object, with a loud `"in_project": false` plus `reason`/`suggestion` fields.

Use `mp go` (or `mp switch --all`) for the explicit all-projects view from anywhere.

### Usage

```bash
mp               # picker scoped to the current project (guidance if outside one)
mp go            # repo switcher: collapsed repos, → to expand (full JSON when not a TTY)
mp go --json     # force JSON output
mp --json        # JSON for the current project
```

The JSON form includes per-project `pieces` and `branches` arrays so callers can build their own pickers (see [`mp switch`](#mp-switch)). Each piece carries a `branch` field (the branch checked out in its worktree — differs from the name for adopted branches), and the top level carries `in_project` / `current_project` so consumers can hard-scope to the caller's repo. The `branches` array includes both local branches and remote-only refs (e.g. `origin/foo`, marked `"remote": true`) that have no local branch yet — selecting one fetches the remote and adopts it as a piece.

---

## mp remote

Remote-host utilities for the ssh proxy (see [Remote development](remote-development.md)).

### Usage

```bash
mp remote doctor wire     # probe one ssh host
mp remote doctor          # probe every box in the project registry (hidden rows included)
```

Like `mp config`, `doctor` uses positional args — there is no JSON-stdin mode.
It reports, per host: key-based (BatchMode) ssh reachability, the remote
`mp` version vs the local one, and `git`/`tmux`/`gh` presence plus `gh` auth
state. Human summary on stderr, JSON array on stdout; exits non-zero if a host
is unreachable or missing `mp`/`git`. Run it once after setting up a host, and
first whenever a proxied command misbehaves.

---

## mp config

Get and set user-level configuration (stored under `~/.config/monkeypuzzle/`). Uses positional args, not JSON stdin.

### Usage

```bash
mp config get multiplexer
mp config set multiplexer tmux   # tmux, zellij, cmux, or none
```

### Keys

| Key           | Description                                | Values                |
| ------------- | ------------------------------------------ | --------------------- |
| `multiplexer` | Terminal multiplexer for piece sessions    | `tmux`, `zellij`, `cmux`, `none` |

---

## Hooks

Hooks are executable shell scripts in `.monkeypuzzle/hooks/` that run at key points during piece operations.

### Available Hooks

| Hook                     | Trigger                  | Execution                        |
| ------------------------ | ------------------------ | -------------------------------- |
| `on-piece-create.sh`     | After piece creation     | Detached (fire-and-forget); output logged to `.monkeypuzzle/logs/` |
| `before-piece-update.sh` | Before `mp update` / `mp sync` | Blocking |
| `after-piece-update.sh`  | After successful update/sync | Blocking |
| `before-piece-merge.sh`  | Before `mp merge`  | Blocking (non-zero exit aborts the merge) |
| `after-piece-merge.sh`   | After successful merge   | Blocking |
| `before-pr-create.sh` / `after-pr-create.sh` | Around `mp pr create` | Blocking |
| `before-pr-ready.sh` / `after-pr-ready.sh`   | Around `mp pr ready`  | Blocking |
| `is-piece-done.sh`       | Consulted by `mp cleanup` / `mp merge`'s merge-status check | Blocking; exit 0 means "treat as merged" — use to recognise squash-merges a plain branch-ancestry check would miss |
| `agent-blocked.sh`       | Piece's aggregate agent status transitions to `blocked` | Detached |
| `agent-done.sh`          | Piece's aggregate agent status transitions to `done`    | Detached |

### Environment Variables

All hooks receive these environment variables:

| Variable           | Description                     |
| ------------------ | ------------------------------- |
| `MP_PIECE_NAME`    | Name of the piece               |
| `MP_WORKTREE_PATH` | Absolute path to worktree       |
| `MP_REPO_ROOT`     | Absolute path to main repo      |
| `MP_MAIN_BRANCH`   | Main branch name (merge/update) |
| `MP_SESSION_NAME`  | Multiplexer session name (create) |
| `MP_PR_NUMBER`     | PR/MR number (PR hooks)         |
| `MP_PR_URL`        | PR/MR URL (PR hooks)            |
| `MP_PR_BASE_BRANCH`| PR/MR base branch (PR hooks)    |
| `MP_AGENT_ID`      | Reporting agent id (agent hooks) |
| `MP_AGENT_KIND`    | Agent kind, e.g. `claude` (agent hooks) |
| `MP_AGENT_STATUS`  | New piece aggregate status (agent hooks) |
| `MP_AGENT_PANE`    | Multiplexer pane the agent runs in (agent hooks) |

### Behavior

- Hooks must be executable (`chmod +x`)
- Non-zero exit code aborts the operation
- Missing hooks are silently skipped
- Hook output is displayed to the user

### Example

`.monkeypuzzle/hooks/before-piece-merge.sh`:

```bash
#!/bin/bash
cd "$MP_WORKTREE_PATH"
echo "Running pre-merge checks for $MP_PIECE_NAME..."
go test ./... || exit 1
```

---

## mp agent

Track agent processes (Claude Code, codex, …) running inside piece worktrees.
Each piece aggregates its agents' statuses by severity — `blocked` > `working`
> `done` > `idle` — and the aggregate surfaces as `agent_status` /
`agent_counts` in `mp go --json` and `mp list` output.

**Zero-install by default.** `mp agent list` / `summary` recognize agent
processes in a piece session's panes (tmux) and classify their state from the
visible screen: an open permission dialog is `blocked`, a running spinner is
`working`, a resting prompt is `idle`. Nothing is written into the agent's
own configuration. Detection is deliberately strict about `blocked`, so a
phrase in conversation text never raises a false alarm.

**Optional precision.** `mp integration install claude` wires Claude Code's
own hooks to `mp agent report`, which adds what a screen can't show: the
`done` state, stable session ids, coverage for headless agents (no pane), and
the `agent-blocked.sh` / `agent-done.sh` lifecycle hooks on transitions. Both
sources merge: hook records win identity, the screen wins liveness.

```bash
# Called by integration hooks, not usually by hand. Resolves the piece from
# the working directory; outside a piece it is a silent no-op (exit 0).
mp agent report --status blocked --id sess-1 --kind claude

# Claude Code hook mode: derives id + status from the hook payload on stdin
mp agent report --claude-hook --pid $PPID

# Every live agent across the project's pieces, blocked first
mp agent list --json

# Fleet view across all registered projects (implied outside a git repo,
# so status lines work from any cwd)
mp agent list --all --json

# Compact status-line segment, e.g. "🔴1 ⚡2" (empty when no agents)
mp agent summary
```

Status `gone` removes the agent's record (sent on clean exit). Records whose
process has died are reaped lazily on the next report.

```bash
# Check on an agent without switching focus: print its pane contents.
# Accepts an agent id or piece name (most attention-worthy agent wins).
mp agent read my-piece

# Answer a blocked agent / hand it a follow-up, as if typed into its pane
mp agent send my-piece "yes, and add tests"
```

`read` / `send` need a multiplexer with pane support (tmux). They are not
TTY-gated: an orchestrating agent may drive its workers with them.

```bash
# Switch the client straight to an agent's pane — the CLI form of the tmux
# plugin's agent picker and blocked-jump chords.
mp agent focus my-piece            # by piece name (most attention-worthy agent)
mp agent focus sess-1              # by agent id
mp agent focus --blocked           # the most urgent blocked agent, no selector
mp agent focus --blocked --all     # across every registered project
```

If the agent's session is no longer live, `focus` falls back to a plain piece
switch (`mp switch` semantics: attaches an existing worktree, never adopts or
creates). `--blocked` with nothing blocked exits 0 with a warning on stderr
and no stdout output — nothing to report.

## mp wait

Block until agents settle — no agent `working` in the target pieces.

```bash
# Fan out, then wait for the whole flock
mp create --name a --agent claude --prompt "..." --skip-switch
mp create --name b --agent claude --prompt "..." --skip-switch
mp wait && mp agent list

mp wait a b --timeout 30m --interval 5s
```

Exits 0 when settled; the JSON `pieces[].aggregate` distinguishes `blocked`
from `done`. Non-zero on timeout.

## mp integration

```bash
# Merge mp's agent-report hooks into .claude/settings.json at the repo root.
# Idempotent; preserves existing settings. Run in the main repo and commit so
# every piece worktree gets it.
mp integration install claude
```

## AI Agent Integration

Monkeypuzzle is designed for programmatic use:

```bash
# Example-input workflow
mp init --schema | jq '.name = "myproject"' | mp init

# Check status programmatically
STATUS=$(mp status)
IN_PIECE=$(echo "$STATUS" | jq -r '.in_piece')

# Parse piece creation output
OUTPUT=$(mp create)
WORKTREE=$(echo "$OUTPUT" | jq -r '.worktree_path')
```

All commands output JSON to stdout for machine parsing, text to stderr for humans.
