# Command Reference

## Input Modes

All commands support multiple input modes:

| Mode        | When                        | Usage                    |
| ----------- | --------------------------- | ------------------------ |
| Interactive | TTY detected                | Run command with no args |
| Flags       | All required flags provided | `mp <cmd> --flag value`  |
| Stdin JSON  | Piped input                 | `echo '{}' \| mp <cmd>`  |
| Schema      | `--schema` flag             | `mp <cmd> --schema`      |

Output goes to stderr (human-readable) while stdout is reserved for JSON (machine-readable).

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
| `mp piece switch --name`        | Available piece names   |
| `mp piece abandon --name`       | Available piece names   |
| `mp piece create --issue`          | Files (for issue paths) |
| `mp init --issue-provider`      | `markdown`              |
| `mp init --pr-provider`         | `github`                |
| `mp piece update --main-branch` | Git branch names        |
| `mp piece merge --main-branch`  | Git branch names        |

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

| Flag               | Description                                                       | Default        |
| ------------------ | ----------------------------------------------------------------- | -------------- |
| `--name`           | Project name                                                      | Directory name |
| `--issue-provider` | Issue provider                                                    | `markdown`     |
| `--pr-provider`    | PR provider                                                       | `github`       |
| `--dir`            | Directory (relative to repo root) for monkeypuzzle state          | `.monkeypuzzle`|
| `--schema`         | Output JSON schema and exit                                       | -              |
| `-y, --yes`        | Overwrite existing config                                         | `false`        |

`--dir` lets you keep all monkeypuzzle state somewhere already ignored by git,
e.g. `mp init --dir .DONOTCOMMIT/monkeypuzzle`. The repo→directory mapping is
recorded in `~/.config/monkeypuzzle/project-dirs.json` (it can't live in
`monkeypuzzle.json`, which is inside the directory being relocated). To relocate
an existing project later, use [`mp move`](#mp-move).

### JSON Schema

```json
{
  "name": "project-name",
  "issue_provider": "markdown",
  "pr_provider": "github",
  "dir": ".monkeypuzzle"
}
```

### Output

Creates the monkeypuzzle directory (default `.monkeypuzzle/`):

```
.monkeypuzzle/
├── monkeypuzzle.json    # Configuration
├── .gitignore           # Ignores pieces/ and per-piece metadata
└── issues/              # Markdown issues (if markdown provider)
```

### Providers

**Issue Providers:**

- `markdown` - Issues as markdown files in `issues/`

**PR Providers:**

- `github` - PR management via `gh` CLI

---

## mp switch

Cross-project picker. Attach an existing piece (or main worktree), or create a piece on the fly by selecting an open todo issue or a stray local branch.

### Usage

```bash
mp switch                                          # Interactive: fuzzy-filter all rows
mp switch --project app                            # Attach app's main worktree
mp switch --project app --piece fix-x              # Attach an existing piece
mp switch --project app --issue issues/auth.md     # Create piece from issue, then attach
mp switch --project app --branch spike-token-rotate  # Adopt branch as piece, then attach
echo '{"project":"app","issue":"issues/auth.md"}' | mp switch
mp switch --schema
```

### Flags

| Flag        | Description                                                            |
| ----------- | ---------------------------------------------------------------------- |
| `--project` | Project name or path (required for non-interactive modes)              |
| `--piece`   | Existing piece to attach                                               |
| `--issue`   | Issue path (e.g. `issues/foo.md`) or title query; creates a piece     |
| `--branch`  | Local git branch to adopt as a piece                                   |
| `--schema`  | Print the JSON-stdin schema and exit                                   |

`--piece`, `--issue` and `--branch` are mutually exclusive. Omit all three to attach the project's main worktree.

### Interactive picker

With a terminal, opens a fuzzy-filtered list of rows across every registered project:

- **Pieces** — existing worktrees, with a `[tmux]` indicator if a session is live.
- **Issues** — open `todo` issues that don't yet have a piece. Selecting one runs `mp piece create --issue`.
- **Branches** — local git branches not currently checked out anywhere (excludes main/master and any branch already adopted as a piece). Selecting one runs `mp piece adopt`.

Type to filter, ↑/↓ to move, `enter` to select, `esc` to cancel. The picker caps the visible rows at 20; narrow the query to surface anything below the cut.

### What it does

1. Resolves the project from the registry.
2. Based on the chosen selector:
   - **piece** — locates the worktree and attaches its tmux session
   - **issue** — runs `CreatePieceFromIssue`, then attaches the new piece's session
   - **branch** — runs `AdoptPiece` with `repo_root` set to the project path, then attaches
3. When no multiplexer is configured, prints the worktree path so `cd $(mp switch ...)` works.

### Non-interactive shape

`mp dash --json` (and the JSON form of `mp switch` when stdout isn't a TTY) now includes per-project `issues` and `branches` arrays so callers can build their own pickers.

---

## mp issue advance / fire / abandon / reopen

Per-project workflows model an issue's lifecycle as named states + events.
`mp piece create` and `mp piece merge` fire the two automatic events
(`branch.created`, `pr.merged`); the verbs below fire everything else.

### Usage

```bash
# Advance to the next progress event (refuses if multiple are available).
mp issue advance --id issues/foo.md
echo '{"id":"issues/foo.md"}' | mp issue advance

# Fire a specific event by name.
mp issue fire --id issues/foo.md --event acceptance.passed
echo '{"id":"issues/foo.md","event":"released"}' | mp issue fire

# Move to the cancel state (workflow's abandoned event).
mp issue abandon --id issues/foo.md
echo '{"id":"issues/foo.md","force":true}' | mp issue abandon

# Move out of cancel: direct write to a named workflow state.
mp issue reopen --id issues/foo.md --to todo
echo '{"id":"issues/foo.md","to":"in-progress"}' | mp issue reopen
```

### Notes

- `advance` filters out the `abandoned` event so it always picks the
  forward-progress step. The cancel axis is reached only via `abandon`.
- A workflow without a `workflow` block in `monkeypuzzle.json` uses the
  built-in default: `todo → in-progress → done`, with `cancelled` reachable
  from any state.
- `mp issue states --provider plane` dumps a Plane project's states
  (id/name/group) so you can populate `workflow.provider_map.plane` without
  curl.

### Workflow definition

A custom workflow lives under the top-level `workflow` key in
`.monkeypuzzle/monkeypuzzle.json`. See `docs/workflow.md` for the schema and a
worked example.

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

## mp piece

Show current piece status.

### Usage

```bash
mp piece
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

## mp piece create

Create a new piece (git worktree + tmux session).

### Usage

```bash
mp piece create
mp piece create --name my-feature
mp piece create --issue issues/my-feature.md
mp piece create --skip-switch  # Don't auto-switch to new piece
```

### Flags

| Flag                  | Description                                   | Default        |
| --------------------- | --------------------------------------------- | -------------- |
| `--name`              | Custom piece name                             | Auto-generated |
| `--issue`             | Create from issue file (sets name from title) | -              |
| `--skip-switch`       | Don't switch to the new piece after creation  | `false`        |
| `--overwrite-session` | Replace existing main repo tmux session       | `false`        |

### What it does

1. Detects current git repository root
2. **Creates main repo tmux session** `mp-<repo-name>` if it doesn't exist
3. Generates piece name: `piece-YYYYMMDD-HHMMSS` (or uses `--name`)
4. Creates git worktree at `~/.local/share/monkeypuzzle/pieces/<piece-name>`
5. Creates tmux session `mp-piece-<piece-name>` (if tmux available)
6. Runs `on-piece-create.sh` hook (if exists)
7. **Switches to the new piece** (unless `--skip-switch` is set)

If the hook fails, the worktree and tmux session are cleaned up automatically.
The auto-switch uses tmux attach/switch-client if available, otherwise prints the path.

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

## mp piece update

Merge main branch into current piece.

### Usage

```bash
mp piece update                  # Merge from 'main'
mp piece update --main-branch develop  # Merge from 'develop'
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

## mp piece merge

Merge piece back to main branch.

### Usage

```bash
mp piece merge                   # Merge to 'main'
mp piece merge --main-branch develop  # Merge to 'develop'
```

### Flags

| Flag            | Description          | Default |
| --------------- | -------------------- | ------- |
| `--main-branch` | Branch to merge into | `main`  |

### Requirements

- Must be run from within a piece worktree
- **Main branch must not be ahead** - Fails if main has commits not in piece

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

If main has commits not in the piece, merge fails. Run `mp piece update` first to incorporate those changes.

---

## mp piece switch

Switch to an existing piece.

### Usage

```bash
mp piece switch                    # Interactive TUI selector
mp piece switch --name my-feature  # Switch by name
echo '{"name":"my-feature"}' | mp piece switch  # JSON stdin
cd $(mp piece switch --name foo)   # Change directory to piece
```

### Flags

| Flag     | Description             | Default |
| -------- | ----------------------- | ------- |
| `--name` | Piece name to switch to | -       |

### What it does

1. Lists available pieces (sorted by modification time, newest first)
2. Shows TUI selector if no name provided
3. Checks if tmux session exists for the piece
4. If in tmux: uses `switch-client` to swap sessions
5. If outside tmux: uses `attach-session` to attach
6. Falls back to printing path if tmux unavailable

### Output

When switching via tmux, outputs JSON:

```json
{
  "piece": {
    "name": "my-feature",
    "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/abc123def456/my-feature",
    "session_name": "mp-piece-my-feature",
    "has_session": true
  },
  "method": "tmux-switch"
}
```

When printing path (no tmux), outputs just the path to stdout for use with `cd $(...)`.

### TUI Selector

Interactive mode shows:

- Piece names sorted by modification time (newest first)
- `[tmux]` indicator for pieces with active sessions
- Navigation: up/down or j/k, enter to select, esc to cancel

---

## mp piece cleanup

Remove worktrees for merged pieces.

### Usage

```bash
mp piece cleanup              # Cleanup merged pieces
mp piece cleanup --dry-run    # Preview what would be cleaned
mp piece cleanup --force      # Skip confirmation
```

### Flags

| Flag            | Description                                | Default |
| --------------- | ------------------------------------------ | ------- |
| `--dry-run`     | Show what would be cleaned without changes | `false` |
| `--force`       | Skip confirmation prompts                  | `false` |
| `--main-branch` | Main branch to check merge status against  | `main`  |

### What it does

1. Scans pieces directory for worktrees
2. Checks if each piece's branch is merged (via git branch, PR, or remote)
3. For merged pieces: removes worktree, kills tmux session, updates issue status
4. Reports what was cleaned

---

## mp piece abandon

Remove an unmerged piece (worktree, tmux session, optionally branch).

### Usage

```bash
mp piece abandon                              # Interactive TUI selector
mp piece abandon --name my-feature            # By name
mp piece abandon --name my-feature --force    # Discard uncommitted changes
mp piece abandon --name foo --delete-branch   # Also delete git branch
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

## Hooks

Hooks are executable shell scripts in `.monkeypuzzle/hooks/` that run at key points during piece operations.

### Available Hooks

| Hook                     | Trigger                  |
| ------------------------ | ------------------------ |
| `on-piece-create.sh`     | After piece creation     |
| `before-piece-update.sh` | Before `mp piece update` |
| `after-piece-update.sh`  | After successful update  |
| `before-piece-merge.sh`  | Before `mp piece merge`  |
| `after-piece-merge.sh`   | After successful merge   |

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
STATUS=$(mp piece)
IN_PIECE=$(echo "$STATUS" | jq -r '.in_piece')

# Parse piece creation output
OUTPUT=$(mp piece create)
WORKTREE=$(echo "$OUTPUT" | jq -r '.worktree_path')
```

All commands output JSON to stdout for machine parsing, text to stderr for humans.
