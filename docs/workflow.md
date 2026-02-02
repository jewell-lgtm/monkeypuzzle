# Stacked Branch Workflow

Monkeypuzzle enables a "stacked branch" workflow using git worktrees, allowing isolated development of atomic changes.

## Core Concepts

### Pieces

A **piece** is an isolated git worktree for developing a single atomic change. Each piece:

- Lives in its own directory (repo-scoped: `~/.local/share/monkeypuzzle/pieces/{repo-hash}/`)
- Has its own branch
- Can be worked on independently
- Gets merged back to main when complete

### Why worktrees?

Git worktrees allow multiple working directories from the same repository:

- Switch between features without stashing
- Run tests in one piece while coding in another
- Isolate experimental changes
- Parallel development of independent features

## Basic Workflow

### 1. Initialize project

```bash
cd myproject
mp init
```

### 2. Start a new feature

```bash
mp piece create
```

This creates:

- New worktree at `~/.local/share/monkeypuzzle/pieces/{repo-hash}/piece-YYYYMMDD-HHMMSS`
- New git branch
- Tmux session (if available)

### 3. Work on the feature

Navigate to the piece and make changes:

```bash
# Check where you are
mp piece

# Make commits as usual
git add .
git commit -m "feat: add user authentication"
```

### 4. Stay in sync

If main branch has new commits:

```bash
mp piece update
```

This merges main into your piece, keeping it up to date.

### 5. Complete the feature

When ready to merge back:

```bash
mp piece merge
```

This:

1. Checks main isn't ahead (safety)
2. Switches to main in the main repo
3. Merges piece branch into main

## Multiple Concurrent Pieces

Work on multiple features simultaneously:

```bash
# From main repo, create first piece
mp piece create --name feature-a
# Work on feature A...

# From main repo, create second piece
mp piece create --name feature-b
# Work on feature B...

# Switch between pieces
mp piece switch                    # TUI selector
mp piece switch --name feature-a   # By name

# Merge feature A when ready
mp piece switch --name feature-a
mp piece merge

# Update feature B with changes from A
mp piece switch --name feature-b
mp piece update
```

## Integration with GitHub PRs

Recommended workflow:

```bash
# Create piece
mp piece create

# Work on feature, commit changes
git add . && git commit -m "feat: new feature"

# Push branch and create PR
git push -u origin HEAD
gh pr create

# After PR review and approval
mp piece merge
git push origin main
```

## Tmux Integration

`mp piece create` creates tmux sessions automatically:

- **Main repo session**: `mp-<repo-name>` (created once, reused)
- **Piece session**: `mp-piece-<piece-name>` (one per piece)

This lets you always switch back to the main repo via `Ctrl+b s` session picker.

Switch between pieces:

```bash
mp piece switch              # TUI selector (works with/without tmux)
mp piece switch --name foo   # By name

# Or use tmux directly:
tmux list-sessions           # See all piece sessions
tmux attach -t mp-piece-...  # Attach to specific piece
```

`mp piece switch` automatically detects if you're in tmux and uses `switch-client` for seamless session switching.

### Tmux for Beginners

**What is tmux?** A terminal multiplexer - run multiple terminal sessions in one window, detach/reattach sessions, and keep processes running when you disconnect.

**Install:**

```bash
# macOS
brew install tmux

# Ubuntu/Debian
sudo apt install tmux

# Fedora
sudo dnf install tmux
```

**Essential commands:**

| Command                       | Description       |
| ----------------------------- | ----------------- |
| `tmux`                        | Start new session |
| `tmux ls`                     | List sessions     |
| `tmux attach -t <name>`       | Attach to session |
| `tmux kill-session -t <name>` | Kill session      |

**Inside tmux (prefix is `Ctrl+b`):**

| Keys                    | Action                         |
| ----------------------- | ------------------------------ |
| `Ctrl+b d`              | Detach (leave session running) |
| `Ctrl+b c`              | New window                     |
| `Ctrl+b n` / `Ctrl+b p` | Next/previous window           |
| `Ctrl+b 0-9`            | Switch to window by number     |
| `Ctrl+b %`              | Split vertically               |
| `Ctrl+b "`              | Split horizontally             |
| `Ctrl+b ←↑↓→`           | Move between panes             |
| `Ctrl+b x`              | Kill current pane              |

**With monkeypuzzle:**

```bash
# Create piece (auto-creates tmux session)
mp piece create --name my-feature

# You're now in session "mp-piece-my-feature"
# Work on code, then detach:
# Press Ctrl+b, then d

# Later, switch back:
mp piece switch --name my-feature

# Or list all piece sessions:
tmux ls | grep mp-piece
```

**Tips:**

- Sessions persist after detaching - your work stays running
- Use `mp piece switch` instead of raw tmux commands for easier navigation
- If tmux isn't installed, monkeypuzzle falls back to printing the path

## Hooks

Monkeypuzzle supports hooks to run custom scripts during piece operations. Create executable scripts in `.monkeypuzzle/hooks/`:

### Pre-merge validation

Run tests before allowing merge to main:

```bash
# .monkeypuzzle/hooks/before-piece-merge.sh
#!/bin/bash
cd "$MP_WORKTREE_PATH"
echo "Running tests..."
go test ./... || exit 1
echo "Linting..."
go vet ./... || exit 1
```

### Post-create setup

Run setup after creating a new piece:

```bash
# .monkeypuzzle/hooks/on-piece-create.sh
#!/bin/bash
cd "$MP_WORKTREE_PATH"
echo "Installing dependencies..."
go mod download
```

### Notifications

Send notifications after merges:

```bash
# .monkeypuzzle/hooks/after-piece-merge.sh
#!/bin/bash
echo "Piece $MP_PIECE_NAME merged to $MP_MAIN_BRANCH" | slack-notify
```

See [docs/commands.md](commands.md) for full hooks reference.

---

## Troubleshooting

### "Main branch is ahead"

If `mp piece merge` fails because main has commits not in your piece:

```bash
mp piece update   # Merge main into piece first
# Resolve any conflicts
mp piece merge    # Now safe to merge
```

### Finding pieces

Pieces are stored in repo-scoped directories:

- Linux: `~/.local/share/monkeypuzzle/pieces/{repo-hash}/`
- macOS: `~/Library/Application Support/monkeypuzzle/pieces/{repo-hash}/`

Each repository has its own pieces directory (identified by a hash of the repo root path), which:

- Isolates pieces per repository
- Prevents naming conflicts between repos
- Makes it easier to manage pieces per project

List and switch between pieces:

```bash
mp piece switch               # Interactive TUI with all pieces (for current repo)
ls ~/.local/share/monkeypuzzle/pieces/  # Manual listing (shows all repos)
```

### Cleaning up pieces

**Merged pieces** - use cleanup to remove all merged pieces at once:

```bash
mp piece cleanup              # Remove all merged pieces
mp piece cleanup --dry-run    # Preview what would be removed
```

**Unmerged pieces** - use abandon to discard work:

```bash
mp piece abandon --name my-feature            # Remove piece
mp piece abandon --name my-feature --force    # Discard uncommitted changes
mp piece abandon --name foo --delete-branch   # Also delete git branch
```
