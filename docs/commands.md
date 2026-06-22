# Command Reference

## Input Modes

Every command follows the same interaction contract, in priority order:

| Mode        | When                        | Usage                    |
| ----------- | --------------------------- | ------------------------ |
| Interactive | TTY detected (default)      | Run command with no args |
| Flags       | One or more flags provided  | `mp <cmd> --flag value`  |
| Stdin JSON  | Piped input                 | `echo '{}' \| mp <cmd>`  |
| Schema      | `--schema` flag             | `mp <cmd> --schema`      |

In interactive mode a Bubble Tea wizard walks you through the decisions, using
defaults inferred from detected state; commands with several decisions are
multi-step wizards. **Flags skip the corresponding wizard steps** — pass enough
flags and the command runs straight through with no wizard. Stdin JSON is the
fully programmatic path (JSON in, JSON out): there is no separate "agent mode",
the same CLI is the API.

**Ambiguity rule:** non-interactive invocations (flags or JSON) **fail loudly on
genuine ambiguity** rather than prompting or guessing — there is no human to ask.
The wizard is the only place ambiguity gets resolved. (A pure no-op is not
ambiguous.)

Output goes to stderr (human-readable) while stdout is reserved for JSON (machine-readable).

**Session management is interactive-only.** mp creates, switches, or attaches a
tmux session **only** when driven interactively — a real terminal on stdin
(isatty) **and** `$TMUX` set. Agents and scripts (flags / stdin JSON, output
captured) have no controlling terminal, so `create`/`switch`/`go` skip tmux and
return the worktree path instead (in JSON, or on stdout for `cd $(mp switch …)`).
`$TMUX` is inherited by child processes, so it is not sufficient on its own — the
TTY check is what keeps an agent from creating a stray session or hijacking the
human's terminal via `switch-client`.

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
| `mp update --main-branch` | Git branch names        |
| `mp merge --main-branch`  | Git branch names        |

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
| `--schema`           | Output JSON schema and exit                                       | -              |
| `-y, --yes`          | Overwrite existing config                                         | `false`        |

`--dir` lets you keep all monkeypuzzle state somewhere already ignored by git,
e.g. `mp init --dir .DONOTCOMMIT/monkeypuzzle`. The repo→directory mapping is
recorded in `~/.config/monkeypuzzle/project-dirs.json` (it can't live in
`monkeypuzzle.json`, which is inside the directory being relocated). To relocate
an existing project later, use [`mp move`](#mp-move).

### JSON Schema

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

Cross-project picker. Attach an existing piece (or main worktree), or create a piece on the fly by adopting a stray local branch.

### Usage

```bash
mp switch                                          # Interactive: fuzzy-filter all rows
mp switch --project app                            # Attach app's main worktree
mp switch --project app --piece fix-x              # Attach an existing piece
mp switch --project app --branch spike-token-rotate  # Adopt branch as piece, then attach
echo '{"project":"app","piece":"fix-x"}' | mp switch
mp switch --schema
```

### Flags

| Flag        | Description                                                            |
| ----------- | ---------------------------------------------------------------------- |
| `--project` | Project name or path (required for non-interactive modes)              |
| `--piece`   | Existing piece to attach                                               |
| `--branch`  | Local git branch to adopt as a piece                                   |
| `--schema`  | Print the JSON-stdin schema and exit                                   |

`--piece` and `--branch` are mutually exclusive. Omit both to attach the project's main worktree.

### Interactive picker

With a terminal, opens a fuzzy-filtered list of rows across every registered project:

- **Pieces** — existing worktrees, with a `[tmux]` indicator if a session is live.
- **Branches** — local git branches not currently checked out anywhere (excludes main/master and any branch already adopted as a piece). Selecting one runs `mp adopt`.

Type to filter, ↑/↓ to move, `enter` to select, `esc` to cancel. The picker caps the visible rows at 20; narrow the query to surface anything below the cut.

### What it does

1. Resolves the project from the registry.
2. Based on the chosen selector:
   - **piece** — locates the worktree and attaches its tmux session
   - **branch** — runs `AdoptPiece` with `repo_root` set to the project path, then attaches
3. Attaching only happens when run **interactively from inside tmux** (real TTY
   on stdin *and* `$TMUX` set). When no multiplexer is configured, or when called
   by an agent/script or from a terminal outside tmux, it prints the worktree
   path instead so `cd $(mp switch ...)` works.

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
main-only state. Each piece's tmux session is killed and its worktree removed.

Unlike `mp cleanup` (which only removes _merged_ pieces), flatten removes every
piece regardless of merge status. Branches are kept by default.

### Usage

```bash
mp flatten                       # Interactive confirmation, then remove all pieces
mp flatten --yes                 # Remove all (skip confirmation)
mp flatten --force               # Discard uncommitted changes while removing
mp flatten --delete-branches     # Also delete each piece's git branch
mp flatten --dry-run             # Show what would be removed without changes
echo '{"force":true}' | mp flatten
mp flatten --schema
```

### Flags

| Flag                | Description                                  | Default |
| ------------------- | -------------------------------------------- | ------- |
| `--force`           | Force removal even with uncommitted changes  | `false` |
| `--delete-branches` | Also delete each piece's git branch          | `false` |
| `--dry-run`         | Show what would be removed without changes   | `false` |
| `--yes`             | Skip the interactive confirmation prompt     | `false` |

### What it does

1. Lists all pieces for the repo (works from the main repo or from inside a piece)
2. In an interactive terminal, asks for confirmation (skip with `--yes`/`--force`)
3. For each piece: switches you to the main session if you're inside it, kills the
   tmux session, and removes the worktree (use `--force` to discard uncommitted changes)
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

Show current piece status.

### Usage

```bash
mp status
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

Create a new piece (git worktree + tmux session).

### Usage

```bash
mp create
mp create --name my-feature
mp create --prompt "add dark mode"        # name auto-generated from the prompt
mp create --parent parent-piece           # stack on another piece
mp create --skip-switch  # Don't auto-switch to new piece
```

### Flags

| Flag                  | Description                                       | Default        |
| --------------------- | ------------------------------------------------- | -------------- |
| `--name`              | Custom piece name                                 | Auto-generated |
| `--prompt`            | Create from a prompt (name auto-generated)        | -              |
| `-p, --parent`        | Parent piece name to branch from (stacks the piece) | `main`       |
| `--skip-switch`       | Don't switch to the new piece after creation      | `false`        |
| `--overwrite-session` | Replace existing main repo tmux session           | `false`        |

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

The auto-switch only manages a tmux session when run **interactively from inside
tmux** (a real TTY on stdin *and* `$TMUX` set): it `switch-client`s your existing
tmux client to the piece's session. Run by an agent or script, or from a terminal
outside tmux, it creates no session and prints the worktree path instead — so
`--skip-switch` is only needed to suppress switching in an interactive session.

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
  { "name": "auth-oauth", "worktree_path": "/path", "parent": "feature-auth", "mod_time": "2025-01-04T11:00:00Z" }
]
```

---

## mp update

Merge main branch into current piece.

### Usage

```bash
mp update                  # Merge from 'main'
mp update --main-branch develop  # Merge from 'develop'
```

### Flags

| Flag            | Description          | Default |
| --------------- | -------------------- | ------- |
| `--main-branch` | Branch to merge from | `main`  |

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

## mp merge

Merge piece back to main branch.

### Usage

```bash
mp merge                   # Merge to 'main'
mp merge --main-branch develop  # Merge to 'develop'
```

### Flags

| Flag                   | Description                                                              | Default |
| ---------------------- | ------------------------------------------------------------------------ | ------- |
| `--main-branch`        | Branch to merge into                                                     | `main`  |
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
| `--dry-run`     | Preview only; never prompt, never change anything | `false` |
| `--force`       | Apply without the interactive confirmation (alias for `--apply`) | `false` |
| `--main-branch` | Main branch to check merge status against         | `main`  |

### What it does

1. Scans pieces directory for worktrees
2. Checks if each piece's branch is merged (via git branch, PR, or remote)
3. Previews the merged pieces and stale projects that would be removed
4. With `--apply` (or an interactive confirmation): removes each worktree, kills
   its tmux session, and prunes registry entries for deleted projects

---

## mp abandon

Remove an unmerged piece (worktree, tmux session, optionally branch).

### Usage

```bash
mp abandon                              # Interactive TUI selector
mp abandon --name my-feature            # By name
mp abandon --name my-feature --force    # Discard uncommitted changes
mp abandon --name foo --delete-branch   # Also delete git branch
```

### Flags

| Flag              | Description                                 | Default |
| ----------------- | ------------------------------------------- | ------- |
| `--name`          | Piece name to abandon                       | -       |
| `--force`         | Force removal even with uncommitted changes | `false` |
| `--delete-branch` | Also delete the git branch                  | `false` |

### What it does

1. Finds the piece by name (or shows TUI selector)
2. Kills the tmux session if it exists
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

Create a GitHub pull request for the current piece. Pushes the branch to origin and creates the PR via the `gh` CLI. Run from within a piece worktree.

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

## mp done

Clean up the current piece (worktree + tmux session) after its branch has been merged. Run from within a piece worktree. Verifies the piece is merged first — use [`mp abandon`](#mp-piece-abandon) for unmerged pieces.

### Usage

```bash
mp done
mp done --main-branch develop
```

### Flags

| Flag            | Description                               | Default |
| --------------- | ----------------------------------------- | ------- |
| `--main-branch` | Main branch to check merge status against | `main`  |

---

## mp adopt

Convert an existing git branch into a piece worktree. Accepts a local branch name or a remote ref like `origin/foo` (remote refs are fetched and a tracking branch is created). Run from the main repo with no branch to adopt the current branch; from inside a piece worktree `--branch` is required.

Adopt does not require a clean working directory: it always creates a *separate* worktree, so uncommitted changes in the main checkout are left untouched. When the branch being adopted is the one currently checked out in the main worktree, adopt frees it by resetting the main worktree back to its main branch (`main`, or `master`), carrying any uncommitted work-in-progress along into the new piece worktree.

When run interactively from inside a tmux session, adopt creates and switches to the new piece's session (like `mp switch`). For agents/automation it leaves tmux untouched and reports the worktree path in the result JSON.

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

Show the stack tree, PR state, and drift vs the GitHub PR list.

```bash
mp stack status
mp stack status --from-github   # rebuild local lineage from open PR bases
mp stack status --apply-bases   # edit PR bases on GitHub to match local lineage
```

| Flag            | Description                                          | Default |
| --------------- | ---------------------------------------------------- | ------- |
| `--from-github` | Rebuild local lineage from open PR bases             | `false` |
| `--apply-bases` | Edit PR bases on GitHub to match local lineage       | `false` |
| `--main`        | Main branch name                                     | `main`  |

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

Manage the global registry of monkeypuzzle projects. A "project" is any git repo initialised with `mp init` (which registers it automatically). Registering projects lets `mp` list pieces across all of them and jump between their tmux sessions. Aliases: `projects`, `proj`.

### Usage

```bash
mp project add                       # register the current directory
mp project add /path/to/repo
echo '{"path":"/repo"}' | mp project add

mp project list                      # human-readable table (alias: ls, status)
mp project list --json               # machine output

mp project remove my-project         # unregister (alias: rm); repo on disk untouched
mp project remove --target /path/to/repo
```

`mp project list` shows best-effort live state per project (current branch, number of pieces).

---

## mp go

A **repo switcher**: jump to any registered project's worktree from anywhere. With a terminal it opens an interactive fuzzy picker where each repo starts **collapsed** (one row per repo). Press `→` to expand a repo and reveal its pieces and branches; `←` collapses it again. Pressing `Enter` on a collapsed repo jumps straight to its **main worktree**. Typing filters across everything (collapsed or not), and the list scrolls (`↑/↓`, `PgUp/PgDn`), sizing its window to the terminal height. A single registered repo starts expanded.

With `--json` (or no TTY) it prints the **full per-project detail** (`pieces`, `branches`) so automation can build its own pickers.

Bare `mp` opens a fuzzy picker **scoped to the current project** (repo-local) — it shows the pieces and branches of the project you're standing in. When run **outside** a monkeypuzzle project, bare `mp` does *not* fall back to the cross-project view; instead it prints context-aware guidance:

- Inside a git repo that hasn't been initialised → suggests `mp init`.
- Outside any git repo → suggests cd-ing into a repo and running `mp init`.
- Either way, if you have registered projects it points you at `mp go`.

Use `mp go` for the explicit all-projects view from anywhere.

### Usage

```bash
mp               # picker scoped to the current project (guidance if outside one)
mp go            # repo switcher: collapsed repos, → to expand (full JSON when not a TTY)
mp go --json     # force JSON output
mp --json        # JSON for the current project
```

The JSON form includes per-project `pieces` and `branches` arrays so callers can build their own pickers (see [`mp switch`](#mp-switch)). The `branches` array includes both local branches and remote-only refs (e.g. `origin/foo`, marked `"remote": true`) that have no local branch yet — selecting one fetches the remote and adopts it as a piece.

---

## mp config

Get and set user-level configuration (stored under `~/.config/monkeypuzzle/`). Uses positional args, not JSON stdin.

### Usage

```bash
mp config get multiplexer
mp config set multiplexer tmux   # tmux, zellij, or none
```

### Keys

| Key           | Description                                | Values                |
| ------------- | ------------------------------------------ | --------------------- |
| `multiplexer` | Terminal multiplexer for piece sessions    | `tmux`, `zellij`, `none` |

---

## Hooks

Hooks are executable shell scripts in `.monkeypuzzle/hooks/` that run at key points during piece operations.

### Available Hooks

| Hook                     | Trigger                  | Execution                        |
| ------------------------ | ------------------------ | -------------------------------- |
| `on-piece-create.sh`     | After piece creation     | Detached (fire-and-forget); output logged to `.monkeypuzzle/logs/` |
| `before-piece-update.sh` | Before `mp update` | Blocking |
| `after-piece-update.sh`  | After successful update  | Blocking |
| `before-piece-merge.sh`  | Before `mp merge`  | Blocking (non-zero exit aborts the merge) |
| `after-piece-merge.sh`   | After successful merge   | Blocking |

### Environment Variables

All hooks receive these environment variables:

| Variable           | Description                     |
| ------------------ | ------------------------------- |
| `MP_PIECE_NAME`    | Name of the piece               |
| `MP_WORKTREE_PATH` | Absolute path to worktree       |
| `MP_REPO_ROOT`     | Absolute path to main repo      |
| `MP_MAIN_BRANCH`   | Main branch name (merge/update) |
| `MP_SESSION_NAME`  | Tmux session name (create)      |

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

## AI Agent Integration

Monkeypuzzle is designed for programmatic use:

```bash
# Schema-based workflow
mp init --schema | jq '.name = "myproject"' | mp init

# Check status programmatically
STATUS=$(mp status)
IN_PIECE=$(echo "$STATUS" | jq -r '.in_piece')

# Parse piece creation output
OUTPUT=$(mp create)
WORKTREE=$(echo "$OUTPUT" | jq -r '.worktree_path')
```

All commands output JSON to stdout for machine parsing, text to stderr for humans.
