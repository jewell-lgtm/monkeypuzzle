---
title: "Getting Started"
order: 0
---
<!-- Generated from docs/getting-started.md by scripts/sync-docs.mjs — edit the source, then run `pnpm sync-docs`. -->
## Prerequisites

- **Git**
- **`gh`** (GitHub) or **`glab`** (GitLab), authenticated, for `mp pr create`
- **Go 1.24+**, only if you build from source

A multiplexer (tmux, zellij, cmux, herdr) is optional; see
[Integrations](/docs/integrations/#multiplexers).

## Installation

### Homebrew

```bash
brew install jewell-lgtm/tap/monkeypuzzle
```

### Via go install

```bash
go install github.com/jewell-lgtm/monkeypuzzle/apps/mp@latest
```

### From source

```bash
git clone https://github.com/jewell-lgtm/monkeypuzzle.git
cd monkeypuzzle
make build                  # → bin/mp
sudo mv bin/mp /usr/local/bin/   # or add bin/ to PATH
```

## Set up your shell

mp keeps a small user config. Create it once; `none` means no multiplexer,
which is the plain-terminal setup this guide uses:

```bash
mp config set multiplexer none
```

(Run any other `mp` command on a terminal first and a setup wizard asks the
same question.)

Then load the shell wrapper, so that `mp create` and `mp switch` move your
shell into the piece's worktree instead of only printing its path:

```bash
echo 'eval "$(mp shell-init zsh)"' >> ~/.zshrc   # bash: ~/.bashrc
exec zsh
```

For fish, add `mp shell-init fish | source` to `~/.config/fish/config.fish`.

Check the result:

```bash
mp doctor
```

`mp doctor` reports the config, multiplexer, shell wrapper, opener and, inside
a repo, whether it's an mp project and whether `gh`/`glab` is authenticated.

## Initialize a project

In your repo:

```bash
cd path/to/your/repo
mp init
```

The wizard asks for a project name (defaults to the directory name) and a PR
provider (`github` or `gitlab`). It creates `.monkeypuzzle/` with the project
config and a `.gitignore` for the piece worktrees, and registers the project
so `mp go` and `mp inbox` can find it.

For scripts or CI, skip the wizard:

```bash
mp init --name myproject --pr-provider github
echo '{"name":"myproject","pr_provider":"github"}' | mp init
mp init --schema | jq '.name = "custom-name"' | mp init
```

## Your first piece

```bash
mp create --name my-feature   # branch + worktree off main; your shell moves into it

# ... make your changes, commit ...

mp pr create --draft          # push the branch, open a draft PR/MR
mp pr ready                   # flip it to ready for review
mp merge                      # merge into main (or merge the PR on the forge)
mp done                       # remove the worktree now it's merged
```

`mp done` refuses a piece that isn't merged; `mp done --force` removes the
worktree anyway and keeps the branch. When the worktree you're standing in is
removed, the shell wrapper takes you back to the main repo.

Each step fires the matching lifecycle hook if you've dropped one in
`.monkeypuzzle/hooks/`; see [Hooks](/docs/workflow/#hooks).

## Getting around

```bash
mp                            # picker over this repo's pieces and branches
mp go                         # picker across every registered project
mp switch my-feature          # straight to a piece or branch by name
mp open my-feature            # open the worktree in your editor
```

`mp open` needs an opener. Set one once:

```bash
mp config set open_command 'code {path}'   # or cursor, zed, idea, …
```

Without the shell wrapper, `cd "$(mp switch my-feature)"` does the same as
`mp switch my-feature`.

## Next steps

- [Workflow guide](/docs/workflow/): [stacking](/docs/workflow/#stacking) pieces, gates, hook recipes
- [Commands reference](/docs/commands/): every flag and JSON shape
- [Integrations](/docs/integrations/): editors, tmux and other multiplexers, coding agents
- [Remote development](/docs/remote-development/): drive a project on another machine, or place single pieces on a box with `mp create --remote`
