# monkeypuzzle tmux plugin

A lightweight tmux UI for [monkeypuzzle](../../README.md): pop up a fuzzy picker
to **switch between pieces** across every registered project, or **create a new
piece**, without leaving your current pane layout.

It is a thin layer over the `mp` CLI. It reads state with `mp go --json` and
renders its own [fzf](https://github.com/junegunn/fzf) picker; the switch/create
actions call `mp` through its stateless API. The plugin exports
`MP_TMUX_PLUGIN=1`, which tells `mp` to perform the tmux `switch-client` /
session-create itself (see the "No tmux sessions for agents" note in the repo
`AGENTS.md`) — so `mp` stays the single source of truth for session naming.

## Requirements

- `mp` on your `PATH` (or set `@monkeypuzzle-bin`)
- `tmux` (the plugin runs inside it)
- [`fzf`](https://github.com/junegunn/fzf) — the picker
- [`jq`](https://stedolan.github.io/jq/) — parses `mp ... --json`

## Install

### With [TPM](https://github.com/tmux-plugins/tpm)

The plugin lives in the `apps/tmux` subdirectory of the monkeypuzzle repo, so
point TPM at that subpath:

```tmux
set -g @plugin 'jewell-lgtm/monkeypuzzle'
set -g @monkeypuzzle-subdir 'apps/tmux'   # if your TPM supports subdirectories
```

If your TPM build cannot load a plugin from a subdirectory, use the manual
method below against a local clone.

### Manual

Clone the repo (or use your existing checkout) and source the entry script from
`~/.tmux.conf`:

```tmux
run-shell ~/path/to/monkeypuzzle/apps/tmux/monkeypuzzle.tmux
```

Reload tmux (`tmux source-file ~/.tmux.conf`) and the key-bindings are live.

## Usage

Every action is a two-key chord: `prefix m`, then one of the keys below (the
plugin claims a single key in the prefix table and puts everything in a
`monkeypuzzle` key table).

| Chord (after prefix) | Action                                                        |
| -------------------- | ------------------------------------------------------------- |
| `m p`                | Switch: pick a piece, branch, or a project's main session     |
| `m g`                | Go to a branch: paste a name — switch, adopt, or create it    |
| `m c`                | Create: pick a project, name the piece, create + switch       |
| `m a`                | Agents: pick a live agent (blocked first), focus its pane     |
| `m b`                | Jump straight to the first blocked agent — no picker          |
| `m t`                | Toggle a sidecar shell split in the current piece's worktree  |
| `m m`                | Cheat sheet: list these bindings                              |

The switch picker shows `project/piece` rows (plus each project's adoptable
branches) with a preview pane of each piece's `git status` and recent commits.
The branch jump (`m g`) is repo-aware: it scopes to the project of the current
pane's directory and takes whatever you paste — an existing piece attaches, an
existing local or remote branch is adopted as a piece, and a brand-new name
creates a piece whose branch is the pasted name verbatim (all one
`mp switch --create` call; outside a project it first asks which project). The
create picker pre-selects the project of the current pane's directory; leaving
the name blank lets you describe the piece as a free-form prompt instead
(`mp create --prompt`).

The agent picker (`m a`) reads `mp agent list --json` to build its rows, then
hands the selected agent's id to `mp agent focus <id>` — one mp invocation
does all the resolving and pane-switching. `m b` is the same `mp agent focus
--blocked` call with no picker; since a run-shell binding has no popup to
show anything in, it relays any stderr the call produces to a `tmux
display-message` — "no blocked agents" when there's nothing to do, or the
error verbatim if the call fails outright.
Agents are detected with nothing installed into them: mp recognizes agent
processes in each piece session's panes and reads blocked/working/idle off
the screen. (`mp integration install claude` optionally adds hook-reported
precision — the `done` state and lifecycle hook events.) The sidecar shell is
the escape hatch from keyboard-capturing agent TUIs: one chord to a real
shell in the same worktree, `m t` again from inside it to close it.

## Status line

For ambient awareness without any always-on UI, add the agent summary to your
status line:

```tmux
set -g status-right '#(cd "#{pane_current_path}" && mp agent summary 2>/dev/null) | %H:%M'
```

(The `cd` matters: tmux runs `#()` from the server's own cwd, which is
usually not your project.)

It renders like `🔴1 ⚡2` (blocked first) and prints nothing when no agents
are live.

## Configuration

Set these tmux options before the plugin loads (defaults shown):

```tmux
set -g @monkeypuzzle-key 'm'             # prefix+<key> enters the chord table
set -g @monkeypuzzle-bin 'mp'            # path/name of the mp binary
set -g @monkeypuzzle-popup-width  '80%'
set -g @monkeypuzzle-popup-height '70%'
```

## Development

```bash
make test-tmux          # run the plugin test suite (bash + jq + fzf)
```

The scripts are structured so their `build_*` row-builders can be sourced and
tested in isolation; the switch flow has a `MP_PLUGIN_FILTER` seam that drives
the picker non-interactively for the integration test. See `test/run.sh`.
