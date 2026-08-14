# Remote development over ssh

mp can drive a project that lives on another machine. The model is a **proxy**:
your local `mp` forwards the whole command over ssh to an `mp` binary on the
remote host, which runs it exactly where the repo and worktrees live. Worktree
paths, lifecycle hooks, tmux sessions, and forge auth (`gh`/`glab`) all resolve
on the remote machine — nothing is emulated locally.

This is built for the fleet workflow: a laptop dispatching pieces to a beefy
box where coding agents run. The remote surface is byte-identical to the local
one — flags, stdin JSON, `--schema`, JSON out — so anything that can drive `mp`
locally can drive a remote host, including [mp-mcp](../apps/mp-mcp/README.md).

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

## Known limits (v1)

- `mp go`/`mp switch` don't yet open remote tmux sessions for you; attach with
  `ssh -t <host> tmux new -A -s <session> -c <worktree>`.
- The dashboard lists remote projects but doesn't enumerate their pieces;
  `mp --project <name> list` proxies in for the live tree.
- One host per project — no mirroring of a repo across hosts.
- Version skew is tolerated (the remote's flags and example-input defaults win — the proxy
  forwards argv verbatim without parsing it); `mp remote doctor` flags a
  mismatch.
