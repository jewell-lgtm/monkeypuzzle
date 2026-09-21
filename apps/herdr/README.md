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
| `open`                       | Popup picker over every piece and project main mp knows about — including pieces with a worktree but **no live workspace yet**, which herdr's switcher can't show — with a git status/log preview. Hands off to `mp switch`. Typing a name no row matches (or `ctrl-o` over one that does) creates that piece instead; `ctrl-d` finishes the highlighted piece and `ctrl-x` abandons it. |
| `create`                     | Popup: pick a project, name the piece (or leave blank and describe it as a prompt) → `mp create`. `ctrl-o` in the `open` picker with nothing typed lands here. |
| `adopt`                      | Popup picker over adoptable local/remote branches → `mp switch --branch` adopts one as a piece. |
| `blocked`                    | No popup: `mp agent focus --blocked --all` jumps straight to the most urgent blocked agent across every registered project. |

The `open` picker is also how you start a piece: whatever you type is a
name as much as a filter, so `alpha/new-thing` — or a bare `new-thing` from
inside a project — that matches no row mints it on Enter, and `ctrl-o`
(`alt-enter`) does the same over a name that does match. It is one
`mp switch --create` call, so an existing piece attaches and an existing
branch is adopted rather than failing. A query that is a filter and not a name
— fzf's `^` `$` `!` `'` operators, two terms, anything `git check-ref-format`
refuses — is turned down instead of handed to git, and a bare `alpha/` asks
for the name through the create picker, in alpha.

It is also how you end one, so culling merged and dead work needs no separate
trip: `ctrl-d` finishes the highlighted piece (`mp done`) and `ctrl-x`
abandons it (`mp abandon`), then the list reloads. Each asks first —
`[y/N/f=force]`, where `f` goes straight past mp's gate — and when the plain
run is refused (the piece isn't merged, the worktree is dirty) it shows mp's
own reason and offers that one escalation rather than dropping you back to a
shell to retype it. Abandon always keeps the branch; use `mp abandon
--delete-branch` for the rest. On a project main row the keys say there is no
piece there and do nothing.

The scripts drive mp through its stateless API and export `MP_MUX_PLUGIN=1`,
which tells mp to perform the herdr workspace focus/create itself (see
"Sessions are interactive-only" in [Integrations](../../docs/integrations.md#sessions-are-interactive-only))
— mp stays the single source of truth for workspace naming.

## Requirements

- `mp` on your `PATH` (or set `MP_PLUGIN_BIN`), with `mp config set multiplexer herdr`
- `herdr` (the plugin runs inside it)
- [`fzf`](https://github.com/junegunn/fzf) ≥ 0.71 and [`jq`](https://stedolan.github.io/jq/) — the pickers

### Command lookup and startup errors

The plugin keeps herdr's existing `PATH` order, then adds `~/.local/bin`,
`/opt/homebrew/bin`, and `/usr/local/bin`. This also works when herdr starts
from a desktop app with only system directories in its environment.
For another install location, add it to the environment used to launch
herdr, or set `MP_PLUGIN_BIN` to the absolute path of `mp` there.

Missing-command errors name the command and show the searched `PATH`.
Failed picker panes keep their error visible until you press Enter;
cancelling a picker still closes it immediately. Direct actions report
errors in `herdr plugin log list`.

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
key = "prefix+u"
type = "plugin_action"
command = "monkeypuzzle.blocked"
description = "monkeypuzzle: jump to blocked agent"
```

The examples preserve herdr’s sidebar (`prefix+b`) and tab navigation
(`prefix+c`, `prefix+n`). Use `prefix+a` for `monkeypuzzle.create` and
`prefix+u` for blocked agents.

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
them non-interactively for the integration tests (`MP_PLUGIN_KEY` names the
key that closed the picker). See `test/run.sh`.

The adapter targets the Herdr 0.9 CLI: list commands already emit JSON,
wrapped in `result`, with `workspace_id`, `pane_id`, and `agent_status` fields.
Agent kinds pass through without a provider allowlist. The list API supplies
no process ID, so mp reports zero rather than inventing one.

Run `MP_TEST_HERDR=1 go test ./internal/adapters -run HerdrLive -v` for an
isolated live-server compatibility check. It requires herdr on PATH and
permission to create local sockets and terminal processes.
