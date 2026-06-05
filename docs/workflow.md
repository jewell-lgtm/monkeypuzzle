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
mp create
```

This creates:

- New worktree at `~/.local/share/monkeypuzzle/pieces/{repo-hash}/piece-YYYYMMDD-HHMMSS`
- New git branch
- Tmux session (if available)

### 3. Work on the feature

Navigate to the piece and make changes:

```bash
# Check where you are
mp status

# Make commits as usual
git add .
git commit -m "feat: add user authentication"
```

### 4. Stay in sync

If main branch has new commits:

```bash
mp update
```

This merges main into your piece, keeping it up to date.

### 5. Complete the feature

When ready to merge back:

```bash
mp merge
```

This:

1. Checks main isn't ahead (safety)
2. Switches to main in the main repo
3. Merges piece branch into main

## Multiple Concurrent Pieces

Work on multiple features simultaneously:

```bash
# From main repo, create first piece
mp create --name feature-a
# Work on feature A...

# From main repo, create second piece
mp create --name feature-b
# Work on feature B...

# Switch between pieces (or jump straight to an open issue / unadopted branch)
mp switch                          # Cross-project fuzzy picker over pieces, issues, and branches
mp switch --project app --piece feature-a
mp switch --project app --issue issues/auth.md   # creates the piece, then attaches
mp switch --project app --branch spike-token     # adopts the branch, then attaches

# Merge feature A when ready
mp switch --project app --piece feature-a
mp merge

# Update feature B with changes from A
mp switch --project app --piece feature-b
mp update
```

## Stacked Pieces

When a piece depends on another (not yet merged) piece, stack them so each builds
on its parent's branch. Create a stacked piece with `--parent`, or use the
`mp stack` subcommands which operate over a whole stack git-town-style.

```bash
# Create a child of the current piece
mp stack append --name api-layer
# ... commit work in api-layer ...

# Create a grandchild stacked on api-layer
mp stack append --name api-ui

# Equivalent low-level form
mp create --name api-ui --parent api-layer

# Insert a piece between the current piece and its parent
mp stack prepend --name shared-types
```

Keep the stack in sync as main and parent branches move:

```bash
# Show the stack tree, PR state, and drift vs the GitHub PR list
mp stack status

# Propagate main and each parent down through the stack
mp stack sync                     # merge strategy (default)
mp stack sync --strategy rebase   # rebase strategy
mp stack sync --push              # push each branch after syncing

# If a rebase sync hits conflicts, resolve them, then:
mp stack continue
```

`mp stack` operations are non-interactive — anything risky aborts cleanly and
prints the next steps (e.g. which PR base to change on GitHub). Open a PR per
piece with `mp pr create`; the base auto-detects to the parent's branch.

## Integration with GitHub PRs

Recommended workflow:

```bash
# Create piece
mp create

# Work on feature, commit changes
git add . && git commit -m "feat: new feature"

# Push the branch and open a PR (run from inside the worktree).
# For a stacked piece the PR base auto-detects to the parent piece's branch.
mp pr create
mp pr create --title "Add feature" --body "Description"

# After the PR is merged, sweep up the worktree and tmux session
mp done
```

`mp pr create` pushes to origin and creates the PR via the `gh` CLI, so you
don't need to run `git push` / `gh pr create` by hand. `mp done` verifies
the branch is merged before cleaning up (use `mp abandon` for unmerged work).

## Tmux Integration

`mp create` creates tmux sessions automatically. Sessions are namespaced by
project so worktrees from different repositories never collide:

- **Main repo session**: `mp/<project>` (created once, reused)
- **Piece session**: `mp/<project>/<piece-name>` (one per piece)

The `<project>` component comes from `project.name` in `.monkeypuzzle/monkeypuzzle.json`
(falling back to the repo directory name). This lets you always switch back to the
main repo via the `Ctrl+b s` session picker.

> **Migration note:** earlier versions used `mp-<repo>` / `mp-piece-<piece>`. The new
> scheme is a rename, not an automatic migration — existing detached sessions keep their
> old names until killed. Rename them with `tmux rename-session -t mp-piece-foo mp/<project>/foo`
> if you want them to match.

Switch between pieces:

```bash
mp switch                              # Fuzzy picker (works with/without tmux)
mp switch --project app --piece foo    # Jump straight to a piece

# Or use tmux directly:
tmux list-sessions           # See all sessions
tmux attach -t mp/<project>/<piece>  # Attach to a specific piece
```

`mp switch` automatically detects if you're in tmux and uses `switch-client` for seamless session switching.

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
mp create --name my-feature

# You're now in session "mp/<project>/my-feature"
# Work on code, then detach:
# Press Ctrl+b, then d

# Later, switch back:
mp switch --project app --piece my-feature

# Or list all piece sessions:
tmux ls | grep "^mp/"
```

**Tips:**

- Sessions persist after detaching - your work stays running
- Use `mp switch` instead of raw tmux commands for easier navigation
- If tmux isn't installed, monkeypuzzle falls back to printing the path

## Hooks

Monkeypuzzle supports hooks to run custom scripts during piece operations. Create executable scripts in `.monkeypuzzle/hooks/` (or `<dir>/hooks/` if the monkeypuzzle directory has been relocated via `mp init --dir` / `mp move`):

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

If `mp merge` fails because main has commits not in your piece:

```bash
mp update   # Merge main into piece first
# Resolve any conflicts
mp merge    # Now safe to merge
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
mp switch               # Interactive fuzzy picker over pieces, issues, and branches
mp list                 # List this repo's pieces as a tree
ls ~/.local/share/monkeypuzzle/pieces/  # Manual listing (shows all repos)
```

### Cleaning up pieces

**Merged pieces** - use cleanup to remove all merged pieces at once:

```bash
mp cleanup              # Remove all merged pieces
mp cleanup --dry-run    # Preview what would be removed
```

**Unmerged pieces** - use abandon to discard work:

```bash
mp abandon --name my-feature            # Remove piece
mp abandon --name my-feature --force    # Discard uncommitted changes
mp abandon --name foo --delete-branch   # Also delete git branch
```

**All pieces** - use flatten to wipe every worktree at once (regardless of merge
status), returning the repo to a flat main-only state:

```bash
mp flatten                    # Confirm, then remove all piece worktrees
mp flatten --dry-run          # Preview what would be removed
mp flatten --force            # Also discard uncommitted changes
mp flatten --delete-branches  # Also delete each piece's branch
```
