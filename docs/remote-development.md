# Remote development over ssh

Vocabulary: the **controller** is the machine running `mp` (your laptop); a
**box** is the ssh destination it talks to. mp's `--host` flag and the
`"host"` field in registry rows and JSON name a box.

## Two placement models

mp can work on another machine in two ways, both built on the same **proxy**:
your local `mp` forwards a command over ssh to an `mp` binary on the box,
which runs it exactly where the repo and worktrees live. Worktree paths,
lifecycle hooks, tmux sessions, and forge auth (`gh`/`glab`) all resolve on
the box — nothing is emulated on the controller.

| Model | What lives on the box | How you address it |
| --- | --- | --- |
| **Remote project** — the whole project is on the box | the repo and every piece | `mp project add box:path`, then `mp --project <name> <cmd>` (or `--host`/`--dir`) — see [Setup](#setup-once-per-host) |
| **Placed piece** — one piece of a local project is on the box | one clone per (project, box) under `~/.local/share/mp/<project>`, with that piece as a normal worktree | `mp create --remote=<box> --name <piece>`; the project, `mp list`, and the tmux plugin stay on the controller — see [Placing a piece on a box](#placing-a-piece-on-a-box) |

Both are built for the fleet workflow: a laptop dispatching pieces to beefy
boxes where coding agents run. The remote surface is byte-identical to the
local one — flags, stdin JSON, `--schema`, JSON out — so anything that can
drive `mp` locally can drive a box, including
[mp-mcp](../apps/mp-mcp/README.md).

## Setup (once per host)

1. Install `mp` on the host, on `PATH` or in `~/.local/bin` (the proxy adds
   `~/.local/bin` to `PATH` itself — plain `ssh` sessions don't read your
   shell profile). Set `MP_REMOTE_BIN=/path/to/mp` locally to override.
2. Make sure key-based ssh works without prompting: the proxy runs with
   `BatchMode=yes` and a 5s connect timeout so agents can never hang on a
   password prompt.
3. Clone the repo and initialise it there — you can do both halves through the
   proxy already:

```bash
ssh wire git clone git@github.com:acme/api code/api
mp --host wire --dir code/api init
mp --host wire config set multiplexer none    # first-run config, once per host
```

4. Register it, scp-style — the path is resolved to an absolute path on the
   host at add time, so relative paths are fine here (and only here):

```bash
mp project add wire:code/api
```

5. Check the host:

```bash
mp remote doctor wire
# wire:
#   ✓ ssh (BatchMode)
#   ✓ mp v0.9.2 = local
#   ✓ git  ✓ tmux  ✓ gh (auth: ✓)
```

`mp remote doctor` with no argument checks every host in your registry. If a
proxied command misbehaves, run it first: unreachable ssh, a missing or stale
remote `mp`, and unauthenticated `gh` (PRs are created *on the host*, so `gh
auth login` must have happened there) are the three usual causes.

## Dispatch

`mp --project <name> <cmd>` routes any command to a registered project —
proxied over ssh when the entry has a host, run from its path when local.
Same command either way. **Proxy flags go between `mp` and the verb** — that
keeps them from colliding with flags the verbs own (`mp init --dir`,
`mp switch --project`), and means the forwarded argv is never parsed locally:

```bash
mp --project api create --prompt "add rate limiting"
echo '{"prompt":"add rate limiting"}' | mp --project api create
mp --project api list
mp --project api stack sync
```

If the same repo is registered both locally and on a host, the bare name is
ambiguous and mp refuses to guess a machine — pass the path or `host:path`
(`mp --project wire:/home/u/api list`); `mp project add` warns when it
creates such a collision.

For anything not registered, `--host`/`--dir` is the raw escape hatch, and the
`MP_HOST`/`MP_DIR` environment variables are their equivalents (mp prints a
banner to stderr when `MP_HOST` reroutes a command, since an exported stray
would otherwise silently proxy everything):

```bash
mp --host wire --dir /home/u/api list
mp --host wire --dir /home/u/api/.monkeypuzzle/pieces/add-rate-limiting pr create
```

Precedence: `--host` flag > project's registry host > local. mp never proxies
based on your cwd alone. `--dir` takes an absolute path, or one relative to
the ssh login home (it ends up inside single quotes on the remote shell, so
`~` never expands).

## What runs where

| Concern | Where it happens |
| --- | --- |
| git, worktrees, branches | remote host |
| lifecycle hooks (`.monkeypuzzle/hooks/`) | remote host, remote env |
| PR creation (`gh` / `glab`) | remote host — auth there |
| tmux sessions | remote host (create/attach yourself: `ssh -t wire tmux new -A -s <session>`) |
| interactivity | remote: a pty is allocated only when your local stdin+stdout are a real terminal |

Piped/JSON invocations get no pty, so JSON output stays byte-clean; when the
local caller supplies no stdin at all, the proxy feeds the remote `{}` (stdin
JSON mode's "all defaults") because ssh always presents a pipe on the far end.

The `worktree_path` in JSON responses from a proxied command is a path **on the
host**. Registry rows for remote projects carry a `"host"` field (`mp project
list --json`, `mp go --json`) so scripts and the tmux plugin can tell them
apart.

## Placing a piece on a box

`mp create --remote=<box> --name <piece>` (or `"remote"` in stdin JSON) puts
**one piece** of the project you are standing in onto a long-lived box. mp
owns the placement — which box a piece lives on — and the box contract
(`mp remote doctor` passes). Provisioning, teardown, toolchains, credentials
and `gh auth` on the box are not mp's: prepare the box by hand (below) or
with whatever system owns it.

```bash
cd ~/code/api                 # local project (the controller side)
mp create --remote=wire --name fix-auth
# Connecting wire for api (clone + mp init under ~/.local/share/mp)...
# ✓ Placed fix-auth on wire (/home/u/.local/share/mp/api/.monkeypuzzle/pieces/fix-auth)
mp list                       # fix-auth shows up with host "wire"
```

What happens, in order:

1. **Validate** on the controller: the box name is a sane ssh destination;
   the piece name is free across local worktrees *and* placed pieces (one
   namespace); `--parent` is `main` or a piece already placed on the same
   box (a local parent or one on another box is refused — stacks never span
   boxes).
2. **Link** — `.monkeypuzzle/placements.json` gets
   `{"fix-auth": {"box": "wire", "pending": true}}` *before* anything touches
   the box, so a crash can never leave a box-side worktree nobody can find.
3. **Connect** (first piece of this project on this box only): if the
   controller has `.monkeypuzzle/hooks/on-box-connect.sh`, it runs — on the
   controller, blocking — and owns this step (see [Hooks](#hooks)).
   Otherwise, over ssh, the repo's `origin` is cloned to
   `$HOME/.local/share/mp/<project>` (skipped if the directory already has a
   `.git`), `mp init --name <project>` runs there (skipped if already a
   project), and the controller's `.monkeypuzzle/hooks/` is rsynced across —
   hooks only exist on the box if shipped. Either way the path is then
   resolved **on the box** (`readlink -f`), so `$HOME` never has to expand on
   the controller. The box is recorded as a hidden registry row
   `<project>@<box>` (`mp project list --all`), which is what "connected"
   means — later pieces skip this step, and a failed connect (hook or
   built-in) is retried next time.
4. **Doctor** the clone: ssh, `mp` on the box, and `init=yes` for that path.
5. **Create** by proxy: `mp --host wire --dir <clone> create --name fix-auth
   [--parent …] [--prompt …] [--branch …] [--agent …] --skip-switch --json` —
   an ordinary create on the box, hooks and all. The proxy exports
   `MP_PLACEMENT_HOST=wire MP_REMOTE=1` into that call so box-side hooks
   know they are serving a placement.
6. **Placed**: the link flips to `pending: false` with the box-side
   `remote_path`.

Any failure after step 2 removes the pending link; the registry row stays
once the box passed doctor (it *is* connected). Named errors: `piece already
exists`, `parent must be main or a piece on the same box`, `box unreachable
over ssh` (ssh exit 255), `box connect failed` (clone/init/rsync or
`on-box-connect.sh` stderr), `box clone is not an mp project`, `mp is not
installed on the box`.

The origin URL is forwarded to the box verbatim by the built-in connect —
use ssh remotes rather than URLs with embedded credentials.

### Working on a placed piece

Piece verbs that take a piece selector — `mp status <piece>`, `mp done
<piece>`, `mp abandon <piece>` (positional or `--piece`) — look the name up
in `placements.json` first. A placed piece is proxied to its box as
`mp --host <box> --dir <remote_path> <verb> …` with the selector stripped
(the box-side mp runs inside the worktree), stdout/exit code passed through.
Verbs that end a piece (`done`, `abandon`) drop the link once the box has
succeeded; when that was the project's last piece on the box, the hidden
registry row goes too. Verbs that only run from inside a piece (`pr create`,
`pr ready`, `update`, `merge`, `sync`) have no selector: address the box-side
worktree directly, which is what the hint after `mp create --remote` prints:

```bash
mp --host wire --dir /home/u/.local/share/mp/api/.monkeypuzzle/pieces/fix-auth pr create --draft
```

A link whose create never finished is **pending**: the verbs refuse it
(`piece placement is still pending`), `mp remote doctor` lists it under
`pending_links`, and `mp cleanup` drops it. `mp cleanup` also asks each box
whether its side of every link still exists (`test -d <remote_path>`) and
drops **stale** links (piece gone on the box, e.g. merged from inside the box
worktree); unreachable boxes keep their links. Its JSON carries the verdicts
under `links`.

### Hooks

Two hook moments sit at the controller/box boundary; everything else is the
ordinary lifecycle, just running on the box.

**`on-box-connect.sh` — controller-side, blocking, once per (project, box).**
Fires from step 3 above the first time a project places a piece on a box,
and replaces the built-in connect entirely: clone, `mp init`, toolchain,
credentials, shipping hooks — whatever the box needs is the hook's job. mp
only resolves the clone path afterwards (`cd <path> && readlink -f .`) and
runs the doctor probe; non-zero exit is `box connect failed` with the hook's
stderr, the pending link is removed and nothing is registered, so the next
`mp create --remote` runs it again. Its env:

| Variable | Value |
| --- | --- |
| `MP_BOX` | the ssh destination (`wire`) |
| `MP_REMOTE_PATH` | where the clone must end up: `$HOME/.local/share/mp/<project>`, **unexpanded** — pass it to the box, don't expand it here |
| `MP_REPO_URL` | the controller's `origin` URL |
| `MP_PROJECT` | project name (`mp init --name` must match) |
| `MP_HOOKS_DIR` | the controller's `.monkeypuzzle/hooks/`, if you want them on the box too |

It runs with the controller repo as cwd. A worked recipe (clone + toolchain
+ `gh auth`) is in the [workflow guide](workflow.md#per-box-setup).

**Box-side hooks of a placed piece** — `on-piece-create.sh`,
`before-pr-create.sh`, all of them — run on the box as usual, from the clone
the hook or built-in connect made, and see two extra variables:
`MP_PLACEMENT_HOST=<box>` (the box's name as the controller addresses it)
and `MP_REMOTE=1`. The controller's proxy exports both into every placement
call (`create --remote`, and verbs routed to a placed piece); plain
`mp --host <box> …` proxying does not, and `MP_HOST` is never set on the box
(it is the reroute variable and would make the box proxy onward). An older
`mp` on the box simply ignores the two variables — `mp remote doctor` already
flags the version skew.

### Preparing a box by hand

The built-in connect needs only ssh + git + `mp` on the box. To control the
clone yourself (a different remote, credentials, a pre-warmed checkout), do
its work up front and mp will find it:

```bash
ssh wire
mkdir -p ~/.local/share/mp && cd ~/.local/share/mp
git clone git@github.com:acme/api api && cd api
mp init --name api            # must match the controller's project name
mp config set multiplexer none
gh auth login                 # PRs are created on the box
```

Back on the controller, `mp remote doctor wire` should be all green; the
first `mp create --remote=wire` then finds the clone and only ships hooks.
To do the same thing repeatably, put it in `on-box-connect.sh` instead
([Hooks](#hooks)).

## Known limits (v1)

- `mp go`/`mp switch` don't yet open remote tmux sessions for you; attach with
  `ssh -t <host> tmux new -A -s <session> -c <worktree>`.
- The dashboard lists remote projects but doesn't enumerate their pieces;
  `mp --project <name> list` proxies in for the live tree.
- One host per remote project — no mirroring of a repo across hosts. A
  placed piece lives on exactly one box and cannot be moved.
- Placed pieces: `mp list` shows them with their last known `state`; live
  refresh and `mp go` into a box session are not there yet — attach with
  `ssh -t <box> tmux new -A -s <session> -c <remote_path>`.
- Version skew is tolerated (the remote's flags and example-input defaults win — the proxy
  forwards argv verbatim without parsing it); `mp remote doctor` flags a
  mismatch.
