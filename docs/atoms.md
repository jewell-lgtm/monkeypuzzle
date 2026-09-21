# Atoms and workflows

mp has a small set of nouns. These are its **atoms**: independently
addressable objects with a clear owner and lifecycle. Commands such as
`mp create`, `mp switch`, and `mp pr create` are workflows that stitch several
atoms together.

mp is not a second Git porcelain and it is not a terminal multiplexer. Git refs,
linked checkouts, and terminal sessions are implementation resources. They only
enter this model through mp-owned identity, lineage, and lifecycle state.

The noun form is the regular, discoverable command surface. Singular and
plural spellings are equivalent where the plural reads naturally:

| Atom | Meaning | Command surface |
| --- | --- | --- |
| **project** | An initialized Git repository known to mp | `mp project` / `mp projects` |
| **branch** | A branch layer recorded in an mp piece stack | `mp branch` / `mp branches` |
| **worktree** | The local storage occupied by an mp piece checkout | `mp worktree` / `mp worktrees` |
| **piece** | A worktree and mp-owned lifecycle envelope for a unit of work | `mp piece` / `mp pieces` |
| **stack** | Ordered base→head relationships between branches | `mp stack` / `mp stacks` |
| **PR** | Forge state attached to a branch | `mp pr` / `mp prs` |
| **agent** | A live agent process reported from a piece | `mp agent` / `mp agents` |
| **history event** | An immutable record of a completed mp transition | `mp history` / `mp events` |
| **skill** | An agent skill document mp ships for this CLI | `mp skill` / `mp skills` |

`show` addresses one object, `list` addresses a collection, `create` adds an
object, and `delete` removes one when that operation is safe for the atom.
Established terms remain aliases: for example `piece show` is also
`piece status`, `stack show` is also `stack status`, and `list` also accepts
`ls` inside the noun namespaces.

## Branch

A branch atom is a layer recorded in an mp piece's stack metadata. Git owns the
underlying ref; mp owns the layer's base, piece, PR association, and lifecycle.
An arbitrary local or remote-tracking ref is not an mp atom until it is adopted
as a piece.

```bash
mp branch                         # list managed branch layers
mp branches list                  # the same JSON/table model
mp branch show                    # current branch
mp branch show feat/auth          # one managed layer
mp branch create feat/auth        # append and check out a layer in this piece
mp branch create --prompt "Add auth API"
mp branch delete feat/auth        # remove the current managed stack tip
mp branch delete --force          # allow removal when the tip records a PR
```

`branch create` is the noun spelling of `stack append`; both update Git and mp
metadata as one transition. `branch delete` only removes the checked-out tip.
It refuses an initial piece branch (use `piece done` or `piece abandon`), a
non-tip layer, dirty work, or a recorded PR unless `--force` is explicit.
Unmanaged refs remain adoption candidates for `mp piece adopt` / `mp switch`.

All branch read commands emit JSON when stdout is not a terminal or with
`--json`. Mutations also accept stdin JSON; `--schema` prints an editable
example.

## Worktree

A worktree atom is the storage occupied by an mp piece checkout. The inventory
also exposes the main checkout and unmanaged Git worktrees as adoption
candidates, but mp does not take ownership of them. Each row includes size and
mp lifecycle, agent, and PR signals.

```bash
mp worktrees                       # TTY: management picker; pipe: JSON list
mp worktree list
mp worktree show [path|branch|piece]
mp worktree delete <selector>      # keep the branch; refuse dirty worktrees
mp worktree delete <selector> --delete-branch
mp worktree delete <selector> --force
```

Deleting a piece worktree delegates to the piece-abandon workflow so child
lineage and metadata cannot be orphaned. Deleting an unmanaged worktree is
refused: adopt it as a piece or use Git to manage it. The main worktree and
the worktree containing the running `mp` process cannot be deleted.

mp intentionally does not create unmanaged worktrees. `mp piece create` creates
a branch, worktree, and lifecycle envelope together; `mp piece adopt` turns an
existing branch/worktree into that managed form. Use Git directly only when an
unmanaged checkout is genuinely what you want.

On a terminal, bare `mp worktrees` first selects a checkout and then offers
mp-coded actions: inspect, finish a merged piece, abandon an active piece, or
adopt an unmanaged checkout. The first/default action is inspect-only. Without
both a stdin and stdout TTY, bare `mp worktrees` never mutates: it returns the
same JSON object as `mp worktree list`.

For the “ten worktrees are filling my laptop” workflow, the size column finds
the expensive pieces and the lifecycle columns explain what culling means.
Finishing or abandoning a piece removes its managed checkout. A piece that
still matters keeps its local worktree, or is created as a placed piece on
another configured box; mp does not silently demote it to an unmanaged Git ref.

## Piece

A piece is mp's execution and lifecycle boundary. A local piece owns:

- a durable `id`, minted at create or adopt, which outlives every renameable
  thing about the piece;
- one Git worktree;
- an initial branch, plus any branches appended to its in-worktree stack;
- metadata including its parent, prompt, merge marker, stack entries, agents,
  and optional placement host;
- optional terminal-integration state (an implementation detail, not an atom);
- the lifecycle hook context rooted at its stable worktree path.

```bash
mp piece                          # show the current piece
mp piece show [piece]
mp pieces list [--all]
mp piece create --name auth
mp piece adopt feat/existing
mp piece sync
mp piece done [piece]
mp piece abandon [piece]
```

The long-standing flat forms (`mp status`, `mp list`, `mp create`, `mp adopt`,
`mp sync`, `mp done`, and so on) remain supported. They are convenient workflow
entry points, not a second data model.

A placed piece has the same identity and metadata contract, but its worktree
lives on its placement host and commands that accept a piece selector proxy to
that host.

### Referring to a piece from outside mp

`project/piece` is the selector a human types, but it is not an identity: it
changes when either name changes. The `id` is the stable one, so it is what an
external system should record. mp assigns it and never interprets it — there is
no place in mp to store a foreign system's identifier, and that is deliberate.

It appears on `mp piece show`, on `mp create --json`, on `mp list --all` when
you need every piece across registered projects, and on the hook-driven
`mp history` events.

A piece created before ids existed has none until something writes its metadata,
and reads stay reads: minting an id dirties the worktree, which is enough to make
`mp cleanup` refuse the piece. `mp piece show --ensure-id` mints one explicitly.

## Stack

A stack atom is an ordered `base → head` relation. mp has two compatible
levels of stack topology:

1. **Inside one piece**, `mp stack append` creates another branch in the same
   worktree and records it after the current tip. The worktree checks out the
   new tip; `mp pr create` uses the recorded base for that branch.
2. **Between pieces**, every piece records a parent piece (or trunk). This forms
   a forest rooted at trunk. `mp piece create --parent`, `mp stack prepend`, and
   `mp stack set-parent` edit that lineage.

The invariants are the same at both levels: a head has exactly one base, cycles
are invalid, and parents precede children during synchronization. A piece gives
parallel work a separate worktree; an in-piece branch gives sequential work a
lighter-weight layer in the same worktree.

```bash
mp stack show                     # alias: status; tree, PR state, drift
mp stacks list                    # same read model
mp stack append auth-api          # branch above this piece's current tip
mp stack prepend auth-schema      # piece between this piece and its parent
mp stack set-parent --parent core # change inter-piece lineage metadata
mp stack sync                     # preview
mp stack sync --apply             # propagate parents in dependency order
mp stack undo                     # restore the pre-sync branch snapshot
```

## PR

A PR atom is the forge association recorded on an mp-managed branch layer.
Listing this atom is deliberately local and predictable: it does not sweep in
unrelated pull requests from the repository or require a live forge call.

```bash
mp pr                             # recorded PRs in this project
mp prs list --json
mp pr show [number|branch]         # current branch when omitted
mp pr create                       # create on the forge and record the result
mp pr ready
```

Live forge reconciliation belongs to stack workflows, which can combine the
recorded association with remote state.

## Skill

A skill atom is a document that teaches an agent this CLI. mp ships the
documents and materialises them; it does not track what an agent then does with
one. The format is portable, so the canonical copy is written to
`.agents/skills/<name>/SKILL.md` and `.claude/skills/<name>` is a relative
symlink to it — Claude Code does not read `.agents/skills/`. The repo makes the
same split between `AGENTS.md` and its `CLAUDE.md` symlink.

```bash
mp skill                          # list (also: mp skill list, mp skills)
mp skill show managing-monkeypuzzle
mp skill create                   # write the default skill into this repo
```

mp ships `managing-monkeypuzzle`, the piece/PR/stack workflow inside one repo
(and cross-project commands such as `mp go` and `mp list --all` when installed
with `mp skill create --user`).

`create` is idempotent and reports `created`, `updated`, or `unchanged`, so it
doubles as the refresh path after upgrading mp. A skill you wrote yourself at
`.claude/skills/<name>` is never replaced — mp reports it and leaves it alone.

Skills are documents, not state: `mp skill` works before the first-run config
wizard, since teaching an agent about mp is a reasonable first move.

## Working across projects

Commands above default to the repo you are standing in. These span every
registered project and work from anywhere:

```bash
mp go                              # switch across projects (also: mp switch --all)
mp list --all                      # every piece in flight
mp agent list --all                # live agents across projects
mp history                         # lifecycle events on this machine
```

## Workflows are compositions

The flat commands optimize common transitions. Their atomic effects are:

| Workflow | Atoms read or changed |
| --- | --- |
| `mp create` | creates **branch** + **piece**; may create a session; fires the piece-create hook |
| `mp adopt` | reads an existing **branch**; creates the **piece** worktree and metadata |
| `mp switch` / `mp go` | resolves **project**, **piece**, and **branch**; may adopt or create a piece; switches session/path |
| `mp stack append` | creates and checks out a **branch**; appends a **stack** edge in the current piece |
| `mp stack prepend` | creates a **branch** + **piece**; inserts an inter-piece **stack** edge |
| `mp pr create` | reads current **branch** and **stack** base; creates a forge **PR**; records it and fires hooks |
| `mp sync` | reads the current **piece** parent; updates its branch from that parent |
| `mp stack sync` | reads the **stack** graph; updates branches parent-first; snapshots refs for undo |
| `mp merge` | merges the current **piece** branch into its stack parent; records merge state and fires hooks |
| `mp done` | verifies branch merge state; removes **piece** worktree/session; reparents child stack edges |
| `mp abandon` | removes an unmerged **piece**; optionally deletes its **branch**; reparents children |
| `mp cleanup` | scans **projects** and **pieces**; removes merged worktrees/sessions and stale project rows |
| `mp worktree delete` | removes a managed **piece** through its abandon lifecycle |

This composition rule is also the extension rule: an atom owns its local
invariants; workflows coordinate atoms and fire lifecycle hooks only after the
relevant boundary has been crossed.

## Compatibility vocabulary

Both forms below are stable:

| Noun form | Flat workflow form |
| --- | --- |
| `mp piece show` | `mp status` |
| `mp piece list` | `mp list` |
| `mp piece create` | `mp create` |
| `mp piece adopt` | `mp adopt` |
| `mp piece sync` | `mp sync` |
| `mp piece merge` | `mp merge` |
| `mp piece done` | `mp done` |
| `mp piece abandon` | `mp abandon` |
| `mp stack show` / `mp stack list` | `mp stack status` |

Collection aliases extend to the remaining atoms: `mp prs`, `mp agents`, and
`mp events`. Project CRUD accepts both its established verbs and the regular
noun vocabulary: `project add`/`create`, `remove`/`delete`, and `list`/`show`.

Prefer noun forms when teaching or exploring mp. Prefer flat forms when writing
the end-to-end workflow as a readable sequence.

## TTY and non-TTY defaults

Bare noun commands are intentionally read-only unless they advertise a
management picker. Scripts never inherit an interactive decision:

| Bare command | Terminal | No TTY / piped |
| --- | --- | --- |
| `mp branch(es)` | managed branch-layer table | `{"branches":[…]}` |
| `mp worktree(s)` | inspect/adopt/finish/abandon picker | `{"worktrees":[…]}`; never mutates |
| `mp piece(s)` | current piece status | current piece status JSON |
| `mp stack(s)` | stack tree and forge drift | stack status JSON |
| `mp project(s)` | registered-project table | project list JSON |
| `mp agent(s)` | live-agent table | agent list JSON |
| `mp history` / `events` | recent-event table | JSON Lines |
| `mp pr(s)` | recorded-PR table | `{"prs":[…]}`; never mutates |

Mutation subcommands require their inputs from arguments, flags, or stdin JSON.
They do not open a picker without a TTY. Destructive worktree and branch actions
retain mp lifecycle gates and Git safety checks unless `--force` is explicit.
