# Integrations

The piece workflow needs git, a forge CLI (`gh` or `glab`) and a terminal.
Everything on this page is optional: ways to connect mp to the editor, shell,
multiplexer, coding agents and machines you already use. Run `mp doctor` to
see which of them are set up on this machine.

## Editor and terminal: `mp open`

`mp open` hands a piece's worktree to the tool you work in: an editor, an IDE,
or a new terminal window.

```bash
mp open                  # the piece you're standing in (or the main worktree)
mp open add-login        # a piece, or a branch (adopted as a piece first)
mp open add-login --with 'zed {path}'
```

The target resolves like [`mp switch`](commands.md#mp-switch), except that
`mp open` never creates a piece.

The opener is a command template. mp uses the first one it finds:

1. `--with '<template>'` on the command line
2. the `$MP_OPEN` environment variable
3. the `open_command` config key (`mp config set open_command '<template>'`)

A template can use `{path}`, `{piece}`, `{project}` and `{branch}`. Each value
is shell-quoted before substitution. A template with no placeholders gets the
path appended, so `code` means `code {path}`. The command runs through `sh -c`
with the worktree as its working directory.

With no opener configured, `mp open` prints the worktree path and these
suggestions:

```bash
mp config set open_command 'code {path}'           # VS Code
mp config set open_command 'cursor {path}'         # Cursor
mp config set open_command 'zed {path}'            # Zed
mp config set open_command 'idea {path}'           # IntelliJ
mp config set open_command 'open -a iTerm {path}'  # a new iTerm window
mp config set open_command 'wezterm start --cwd {path}'
```

`mp create` and `mp switch` take `--open` to open the worktree right after
landing in it, and `--with` to pick the opener for that call:

```bash
mp create --name add-login --open
mp switch add-login --open --with 'cursor {path}'
```

## Shell

### Follow mp into the worktree: `mp shell-init`

mp never changes your shell's directory on its own. Without a multiplexer
session to attach, `mp create` and `mp switch` print the worktree path and
leave you where you are. `mp shell-init` prints a small `mp` shell function
that closes the loop:

```bash
eval "$(mp shell-init zsh)"       # ~/.zshrc
eval "$(mp shell-init bash)"      # ~/.bashrc
mp shell-init fish | source       # ~/.config/fish/config.fish
```

The function runs mp with `MP_CWD_FILE` pointing at a temp file. Verbs that
end with "you should now be in this directory" write it there, and the
function `cd`s into it after mp exits. Those verbs are `switch`, `go`,
`create`, `adopt`, `inbox next`/`prev`, `agent focus`, and `done`, `abandon`
and `cleanup` when they remove the worktree you're standing in (you land in
the main repo). mp's stdout and exit code are unchanged, so pipes and scripts
behave the same.

Without the wrapper, `cd` yourself:

```bash
cd "$(mp switch add-login)"
```

### Tab completion

`mp completion bash|zsh|fish|powershell` prints a completion script. Piece
names complete on `mp switch`, `mp open`, `mp done` and the other verbs that
take a piece. Setup per shell is in the
[command reference](commands.md#shell-completion).

## Multiplexers

mp can give each piece its own multiplexer session. The default is no
multiplexer:

```bash
mp config get multiplexer
mp config set multiplexer tmux    # tmux, zellij, cmux, herdr, or none (default)
```

The first `mp` command you run on a terminal, before any config exists, asks
which one you use.

With a multiplexer configured, each piece gets a session named
`mp/<project>/<piece>` (`<project>` is `project.name` from
`.monkeypuzzle/monkeypuzzle.json`, which defaults to the repo directory name),
so pieces from different repos never collide. `mp create`, `mp switch` and
`mp go` create the session if needed and move your client to it. Long-running
processes survive switching: each piece's session keeps its own dev server,
log tail or REPL. `mp done`, `mp abandon`, `mp cleanup` and `mp flatten` kill
the session along with the worktree.

### Sessions are interactive-only

mp manages a session only when you drive it interactively from inside the
configured multiplexer: a real terminal on stdin **and** the multiplexer's
in-session variable set (`$TMUX` for tmux, `$ZELLIJ` for zellij,
`$CMUX_WORKSPACE_ID` for cmux, `$HERDR_ENV` for herdr).

Driven any other way (by an agent or script through flags or stdin JSON with
output captured, or from a terminal outside the multiplexer) mp creates
**no** session and returns the worktree path instead: in the result JSON, or
on stdout so `cd "$(mp switch …)"` works. The in-session variable alone isn't
enough, because child processes inherit it: an agent launched from inside your
tmux still has `$TMUX` set. The TTY check is what keeps an agent from creating
a stray session or switching your terminal out from under you.

The one exception is the companion plugins: their popups have no TTY of their
own, so they export `MP_MUX_PLUGIN=1` (tmux's older spelling:
`MP_TMUX_PLUGIN=1`) in place of the TTY check, and mp does the session work
for them.

### tmux

```bash
mp config set multiplexer tmux
mp switch add-login                        # switch-client if you're inside tmux
tmux ls | grep "^mp/"                      # all mp sessions
tmux attach -t mp/<project>/<piece>        # raw tmux attach
```

The companion plugin in [`apps/tmux`](../apps/tmux/README.md) binds a
`prefix m` chord table: an `fzf` popup for switching between pieces and
branches (`prefix m p`), a paste-a-branch jump scoped to the current repo
(`prefix m g`), piece creation (`prefix m c`), the inbox (`prefix m i`), agent
focus (`prefix m a` / `m b`), and more. It reads state with `mp go --json` and
`mp inbox --json` and hands the session work back to mp.

#### Tmux 101

If you've never used tmux, the essentials:

| Command | Description |
| --- | --- |
| `tmux` | start a new session |
| `tmux ls` | list sessions |
| `tmux attach -t <name>` | attach to session |
| `Ctrl+b d` | detach (session keeps running) |
| `Ctrl+b s` | session picker |
| `Ctrl+b c` | new window |
| `Ctrl+b %` / `Ctrl+b "` | split pane |

Sessions persist after detaching: your dev server keeps running and your
terminal state survives reattach.

### herdr

With `mp config set multiplexer herdr`, pieces open as herdr workspaces
labeled `mp/<project>/<piece>`, and mp reads herdr's own agent tracking
instead of reading the screen. The [herdr plugin](../apps/herdr/README.md)
adds pickers for what herdr can't show natively: pieces with no live
workspace, adoptable branches, and the inbox.

### zellij and cmux

Both are supported for sessions (`mp config set multiplexer zellij` or
`cmux`). Agent pane commands (`mp agent read`/`send`) need tmux or herdr.

## Coding agents

mp has no separate agent mode. Every command takes flags, stdin JSON, or
`--schema`, and prints JSON on stdout, so an agent drives the same CLI you do
(see [Input modes](commands.md#input-modes)). Non-interactive calls fail
loudly on genuine ambiguity rather than guessing.

```bash
mp create --schema
echo '{"name":"my-feature","skip_switch":true}' | mp create
```

- **Claude Code skill.** `mp init` writes
  `.claude/skills/managing-monkeypuzzle/SKILL.md` so Claude Code knows the CLI;
  `mp claude skill` regenerates it.
- **Agent status.** `mp integration install claude` merges hooks into the
  repo's `.claude/settings.json` that report each agent's state to mp
  (`blocked`, `working`, `done`, `idle`). With tmux or herdr configured, mp
  also detects agents in each piece's panes with nothing installed. Without a
  multiplexer, the hooks are the only way mp sees agents.
- **Watching agents.** `mp agent list`, `mp agent summary` (a status-line
  segment), `mp agent focus --blocked` (jump to the agent waiting on you), and
  `mp wait` (block until no agent is working). Agent state also feeds the
  [inbox](workflow.md#the-inbox) and the `agent-blocked.sh` / `agent-done.sh`
  hooks.
- **MCP.** [`mp-mcp`](../apps/mp-mcp/README.md) exposes the workflow as MCP
  tools for assistants that speak MCP.

Reference: [Agent commands](commands.md#agent-commands).

## Remote boxes

mp can drive a project that lives on another machine over ssh
(`mp --project <name> <cmd>`), or place a single piece of a local project on a
box (`mp create --remote=<box>`). Hooks, worktrees and forge auth all resolve
on the box. See [Remote development](remote-development.md).

## Dashboard

The Monkeypuzzle server syncs your GitHub and GitLab repos and draws each
stack of PRs as a live tree in the browser, with an MCP endpoint for agents.
The CLI doesn't need it. To run your own, see [Self-hosting](self-hosting.md).
