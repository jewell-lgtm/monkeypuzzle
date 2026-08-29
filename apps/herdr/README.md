# monkeypuzzle herdr plugin

A [herdr](https://herdr.dev) plugin for [monkeypuzzle](../../README.md). It is
**not** a port of the tmux plugin: herdr already does natively most of what
that plugin needed pickers and status-line hacks for, and mp reads herdr's
own agent tracking when `multiplexer` is set to `herdr` (see below). This
plugin only adds the verbs where mp holds data herdr doesn't.

## What herdr already covers (nothing to install)

| tmux plugin feature              | herdr-native replacement                  |
| -------------------------------- | ----------------------------------------- |
| switch picker for live sessions  | workspace switcher / sidebar (workspaces are labeled `mp/<project>/<piece>`) |
| agents picker + capture preview  | per-pane agent state in the sidebar       |
| `mp agent summary` status line   | sidebar state icons                       |
| sidecar shell split toggle       | herdr's own splits / overlay panes        |

And with `mp config set multiplexer herdr`, `mp agent list` / `mp wait` /
`mp agent focus --blocked` consume herdr's native agent states (including
`done`, and agents beyond claude/codex) instead of screen-scraping.

## What this plugin adds

| Action (`monkeypuzzle.<id>`) | What it does                                                   |
| ---------------------------- | -------------------------------------------------------------- |
| `open`                       | Popup picker over every piece and project main mp knows about — including pieces with a worktree but **no live workspace yet**, which herdr's switcher can't show — with a git status/log preview. Hands off to `mp switch`. |
| `create`                     | Popup: pick a project, name the piece (or leave blank and describe it as a prompt) → `mp create`. |
| `adopt`                      | Popup picker over adoptable local/remote branches → `mp switch --branch` adopts one as a piece. |
| `blocked`                    | No popup: `mp agent focus --blocked --all` jumps straight to the most urgent blocked agent across every registered project. |

The scripts drive mp through its stateless API and export `MP_MUX_PLUGIN=1`,
which tells mp to perform the herdr workspace focus/create itself (see
"Sessions are interactive-only" in the [workflow guide](../../docs/workflow.md#sessions-are-interactive-only))
— mp stays the single source of truth for workspace naming.

## Requirements

- `mp` on your `PATH` (or set `MP_PLUGIN_BIN`), with `mp config set multiplexer herdr`
- `herdr` (the plugin runs inside it)
- [`fzf`](https://github.com/junegunn/fzf) and [`jq`](https://stedolan.github.io/jq/) — the pickers

## Install

```bash
herdr plugin install jewell-lgtm/monkeypuzzle/apps/herdr
```

For development against a local clone:

```bash
herdr plugin link ~/path/to/monkeypuzzle/apps/herdr
```

## Key bindings

Bind the actions in your herdr config, e.g.:

```toml
[[keys.command]]
key = "prefix+m"
type = "plugin_action"
command = "monkeypuzzle.open"
description = "monkeypuzzle: open piece"

[[keys.command]]
key = "prefix+b"
type = "plugin_action"
command = "monkeypuzzle.blocked"
description = "monkeypuzzle: jump to blocked agent"
```

## Hook coexistence

herdr's own `integration install claude` and mp's `mp integration install
claude` don't conflict: herdr installs a user-level hook that reports state
to herdr; mp merges repo-level hooks into `.claude/settings.json` that report
to mp. With `multiplexer herdr`, mp's hooks are optional — herdr's native
tracking already gives mp every state including `done` — but they remain the
only thing that fires mp's own `agent-blocked.sh` / `agent-done.sh` piece
hooks.

## Development

```bash
make test-herdr        # run the plugin test suite (bash + jq + fzf)
```

The scripts are structured so their `build_*` row-builders can be sourced and
tested in isolation; the pickers have a `MP_PLUGIN_FILTER` seam that drives
them non-interactively for the integration tests. See `test/run.sh`.

Verified against the herdr docs; before the first release, smoke-check the
exact CLI spellings against a live install (`herdr api schema`): the
`plugin pane open` invocation in `scripts/show.sh`, popup `width`/`height`
in the manifest, and whether manifests can declare default key bindings.
