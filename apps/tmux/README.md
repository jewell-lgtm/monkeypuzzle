# monkeypuzzle tmux plugin

A lightweight tmux UI for [monkeypuzzle](../../README.md): one fuzzy picker for
**every context switch** — jump to any piece or project, adopt a branch as a
piece, or create a new piece — without leaving your current pane layout.

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

| Chord (after prefix) | Action                                                       |
| -------------------- | ------------------------------------------------------------ |
| `m p`                | The one-stop picker: any context switch (see below)          |
| `m a`                | Agents: pick a live agent (blocked first), focus its pane    |
| `m b`                | Jump straight to the first blocked agent — no picker         |
| `m t`                | Toggle a sidecar shell split in the current piece's worktree |
| `m m`                | Cheat sheet: list these bindings                             |

### The `m p` picker

One list covers everything, so `prefix m p` is the muscle memory for any
context switch. Per project it shows, in order: the `(main)` session, every
piece (newest first), a `(+ new piece)` row, and `(branch: …)` rows for local
or remote branches that aren't pieces yet. The preview pane shows each row's
`git status` and recent commits.

| In the picker | Action                                                          |
| ------------- | --------------------------------------------------------------- |
| `enter`       | Switch to the row: piece / main session / adopt branch as piece |
| `enter` on `(+ new piece)` | Create a piece in that project (name prompt)       |
| `ctrl-n`      | New piece in the highlighted project — typed text prefills the name |
| `esc`         | Quit                                                            |

The picker is current-aware, fed by a single `mp go --json` call: the header
shows `current: project/piece`, the row you're in carries a `*` marker, your
current project's block sorts first (remaining projects by most recent piece
activity), and pieces with live agents carry the same status icons as the
agent picker (🔴 blocked, ⚡ working, ✅ done, 💤 idle).

When creating, leaving the name blank lets you describe the piece as a
free-form prompt instead (`mp create --prompt`).

The agent picker and blocked-jump read `mp agent list --json`. Agents are
detected with nothing installed into them: mp recognizes agent processes in
each piece session's panes and reads blocked/working/idle off the screen.
(`mp integration install claude` optionally adds hook-reported precision —
the `done` state and lifecycle hook events.) The sidecar shell is the escape
hatch from keyboard-capturing agent TUIs: one chord to a real shell in the
same worktree, `m t` again from inside it to close it.

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

### Chord indicator

Entering a key table releases prefix mode, so a `#{client_prefix}`-based
prefix indicator goes dark the moment you press `prefix m` — it looks like
the chord was swallowed. Put `#{monkeypuzzle_chord}` anywhere in your
`status-left` / `status-right` and the plugin swaps it at load time for an
indicator that lights up while the chord table waits for its second key:

```tmux
set -g status-right '#{monkeypuzzle_chord}#(cd "#{pane_current_path}" && mp agent summary 2>/dev/null) | %H:%M'
```

It renders as ` mp… ` (reversed) mid-chord and as nothing otherwise; restyle
it with `@monkeypuzzle-indicator` — escape any commas in style directives as
`#,` (e.g. `#[fg=black#,bg=yellow] mp… #[default]`), since the text lands
inside a tmux `#{?…,…,}` conditional. (tmux has no user-defined format
variables, so this is a load-time text substitution — the same trick as
tmux-prefix-highlight. Reload your config after changing the option.)

## Configuration

Set these tmux options before the plugin loads (defaults shown):

```tmux
set -g @monkeypuzzle-key 'm'             # prefix+<key> enters the chord table
set -g @monkeypuzzle-bin 'mp'            # path/name of the mp binary
set -g @monkeypuzzle-popup-width  '80%'
set -g @monkeypuzzle-popup-height '70%'
set -g @monkeypuzzle-indicator '#[reverse] mp… #[noreverse]'  # #{monkeypuzzle_chord} text
```

Reloading your config re-runs the plugin, so bindings (and the indicator)
always match the checkout you load it from — re-source after updating.

## Development

```bash
make test-tmux          # run the plugin test suite (bash + jq + fzf)
```

The scripts are structured so their `build_*` row-builders can be sourced and
tested in isolation, and the flows are drivable without a TTY through env
seams: `MP_PLUGIN_FILTER` stands in for the fzf query, `MP_PLUGIN_KEY` for an
`--expect` accelerator key, and `MP_PLUGIN_INPUT_NAME` / `MP_PLUGIN_INPUT_PROMPT`
for the create flow's prompts (set-but-empty counts as "entered blank"). See
`fzf_pick_expect` / `prompt_input` in `scripts/helpers.sh` and `test/run.sh`.
