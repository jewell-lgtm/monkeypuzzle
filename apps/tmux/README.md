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

| Key (after prefix) | Action                                                  |
| ------------------ | ------------------------------------------------------- |
| `p`                | Switch: pick a piece (or a project's main session)      |
| `P`                | Create: pick a project, name the piece, create + switch |

The switch picker shows `project/piece` rows with a preview pane of each piece's
`git status` and recent commits. The create picker pre-selects the project of the
current pane's directory; leaving the name blank lets you describe the piece as a
free-form prompt instead (`mp create --prompt`).

## Configuration

Set these tmux options before the plugin loads (defaults shown):

```tmux
set -g @monkeypuzzle-switch-key 'p'
set -g @monkeypuzzle-create-key 'P'
set -g @monkeypuzzle-bin        'mp'     # path/name of the mp binary
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
