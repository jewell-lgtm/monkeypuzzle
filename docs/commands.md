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

- `markdown` - Issues as markdown files in `.monkeypuzzle/issues/`

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
```

### Flags

| Flag     | Description       | Default        |
| -------- | ----------------- | -------------- |
| `--name` | Custom piece name | Auto-generated |

### What it does

1. Detects current git repository root
2. Generates piece name: `piece-YYYYMMDD-HHMMSS` (or uses `--name`)
3. Creates git worktree at `~/.local/share/monkeypuzzle/pieces/<piece-name>`
4. Creates symlink `.monkeypuzzle-source` to source monkeypuzzle config
5. Creates tmux session `mp-piece-<piece-name>` (if tmux available)
6. Runs `on-piece-create.sh` hook (if exists)

If the hook fails, the worktree and tmux session are cleaned up automatically.

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
