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

| Flag | Completes To |
|------|-------------|
| `mp piece switch --name` | Available piece names |
| `mp piece abandon --name` | Available piece names |
| `mp piece new --issue` | Files (for issue paths) |
| `mp init --issue-provider` | `markdown` |
| `mp init --pr-provider` | `github` |
| `mp piece update --main-branch` | Git branch names |
| `mp piece merge --main-branch` | Git branch names |

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

| Flag               | Description                 | Default        |
| ------------------ | --------------------------- | -------------- |
| `--name`           | Project name                | Directory name |
| `--issue-provider` | Issue provider              | `markdown`     |
| `--pr-provider`    | PR provider                 | `github`       |
| `--schema`         | Output JSON schema and exit | -              |
| `-y, --yes`        | Overwrite existing config   | `false`        |

### JSON Schema

```json
{
  "name": "project-name",
  "issue_provider": "markdown",
  "pr_provider": "github"
}
```

### Output

Creates `.monkeypuzzle/` directory:

```
.monkeypuzzle/
├── monkeypuzzle.json    # Configuration
└── issues/              # Markdown issues (if markdown provider)
```

### Providers

**Issue Providers:**

- `markdown` - Issues as markdown files in `issues/`

**PR Providers:**

- `github` - PR management via `gh` CLI

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
  "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/piece-20241226-143022",
  "repo_root": "/home/user/projects/myproject"
}
```

Human-readable message to stderr.

---

## mp piece new

Create a new piece (git worktree + tmux session).

### Usage

```bash
mp piece new
mp piece new --name my-feature
mp piece new --issue issues/my-feature.md
mp piece new --skip-switch  # Don't auto-switch to new piece
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
5. Creates symlink `.monkeypuzzle-source` to source monkeypuzzle config
6. Creates tmux session `mp-piece-<piece-name>` (if tmux available)
7. Runs `on-piece-create.sh` hook (if exists)
8. **Switches to the new piece** (unless `--skip-switch` is set)

If the hook fails, the worktree and tmux session are cleaned up automatically.
The auto-switch uses tmux attach/switch-client if available, otherwise prints the path.

### Output

JSON to stdout:

```json
{
  "name": "piece-20241226-143022",
  "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/piece-20241226-143022",
  "session_name": "mp-piece-piece-20241226-143022"
}
```

### Piece storage

Pieces stored in XDG data directory:

- Linux: `~/.local/share/monkeypuzzle/pieces/`
- macOS: `~/Library/Application Support/monkeypuzzle/pieces/`
- `$XDG_DATA_HOME/monkeypuzzle/pieces/` if set

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

| Flag     | Description            | Default |
| -------- | ---------------------- | ------- |
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
    "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/my-feature",
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

| Flag            | Description                                   | Default |
| --------------- | --------------------------------------------- | ------- |
| `--dry-run`     | Show what would be cleaned without changes    | `false` |
| `--force`       | Skip confirmation prompts                     | `false` |
| `--main-branch` | Main branch to check merge status against     | `main`  |

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

| Flag              | Description                               | Default |
| ----------------- | ----------------------------------------- | ------- |
| `--name`          | Piece name to abandon                     | -       |
| `--force`         | Force removal even with uncommitted changes | `false` |
| `--delete-branch` | Also delete the git branch                | `false` |

### What it does

1. Finds the piece by name (or shows TUI selector)
2. Kills the tmux session if it exists
3. Removes the git worktree (use `--force` to discard uncommitted changes)
4. Optionally deletes the git branch (`--delete-branch`)

### Output

```json
{
  "piece_name": "my-feature",
  "worktree_path": "/home/user/.local/share/monkeypuzzle/pieces/my-feature",
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
OUTPUT=$(mp piece new)
WORKTREE=$(echo "$OUTPUT" | jq -r '.worktree_path')
```

All commands output JSON to stdout for machine parsing, text to stderr for humans.
