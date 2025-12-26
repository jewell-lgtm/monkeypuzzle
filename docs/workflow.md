# Stacked Branch Workflow

Monkeypuzzle enables a "stacked branch" workflow using git worktrees, allowing isolated development of atomic changes.

## Core Concepts

### Pieces

A **piece** is an isolated git worktree for developing a single atomic change. Each piece:
- Lives in its own directory (`~/.local/share/monkeypuzzle/pieces/`)
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
mp piece new
```

This creates:
- New worktree at `~/.local/share/monkeypuzzle/pieces/piece-YYYYMMDD-HHMMSS`
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
mp piece new
# Work on feature A...

# From main repo, create second piece
mp piece new
# Work on feature B...

# Merge feature A when ready
cd ~/.local/share/monkeypuzzle/pieces/piece-20241226-100000
mp piece merge

# Update feature B with changes from A
cd ~/.local/share/monkeypuzzle/pieces/piece-20241226-110000
mp piece update
```

## Integration with GitHub PRs

Recommended workflow:

```bash
# Create piece
mp piece new

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

`mp piece new` creates a tmux session automatically:
- Session name: `mp-piece-<piece-name>`
- Working directory: piece worktree path

Switch between pieces using tmux:
```bash
tmux list-sessions          # See all piece sessions
tmux attach -t mp-piece-... # Attach to specific piece
```

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

Pieces are stored in:
- Linux: `~/.local/share/monkeypuzzle/pieces/`
- macOS: `~/Library/Application Support/monkeypuzzle/pieces/`

List all pieces:
```bash
ls ~/.local/share/monkeypuzzle/pieces/
```

### Cleaning up old pieces

After merging, worktrees remain. Clean up manually:
```bash
# From main repo
git worktree remove ~/.local/share/monkeypuzzle/pieces/<piece-name>
```
