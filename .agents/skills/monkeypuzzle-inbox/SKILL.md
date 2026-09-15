---
name: monkeypuzzle-inbox
description: Reads and reorders the mp inbox — every piece in flight across every project, ranked. Use when asked what to work on next, what is in progress or blocked, what is waiting on review, or to reprioritise, annotate or snooze work. Also the bridge for syncing mp state into an external task system. For the piece/PR/stack workflow inside one repo, use managing-monkeypuzzle instead.
---

# mp inbox

The inbox is one ranked list of every piece across every registered project.
Unlike the rest of `mp`, it does not care which repo you are standing in — it
works from anywhere, including outside a project.

`mp inbox` and the three editing verbs emit JSON to stdout when piped or given
`--json`; on a terminal the human table goes to **stderr**, so stdout stays
parseable either way. Two exceptions: `mp inbox next`/`prev` print a bare path
unless you pass `--json`, and `mp history --json` emits JSON *lines*, one
object per line, not a single document.

## Read it

```bash
mp inbox --json                    # {"rows": [...]}
mp inbox --json --sort urgency     # urgency first instead of your manual rank
mp inbox --refresh --json          # re-fetch PR state instead of using the cache
```

Each row:

| Field | Meaning |
| --- | --- |
| `key` | `project/piece` — the selector you pass to the verbs below |
| `id` | The piece's durable identifier. Unlike `key` it survives renames |
| `rank` | Position in the list, 1-based |
| `urgency` | `blocked` > `review` > `working` > `idle` > `merged` |
| `agent_status` | Aggregate of the piece's agents: `blocked`, `working`, `done`, `idle` — or `""` when no agent has reported, which is the common case |
| `agent_counts` | Per-status counts behind that aggregate, or `null` when there are none |
| `pr` | `{number, url, state, draft}` when the branch has one |
| `note` | Free-form text you attached |
| `snoozed` | Snooze already evaluated against now — no timestamp maths needed |
| `snoozed_until` | When it comes back |
| `merged`, `branch`, `parent`, `worktree_path`, `host`, `updated_at` | |

Order is your manual rank first, urgency breaking ties; `--sort urgency`
inverts that. Snoozed rows always sort last.

`urgency` is derived, not stored: `blocked` means an agent is waiting on a
human, `review` means an agent finished or a non-draft PR is open, `merged`
means the PR landed and the piece is ready to clean up.

## Change it

Each verb takes the same selector and emits JSON. `project/piece` is exact; a
bare piece name prefers the project you are standing in and otherwise has to be
unique, or the call fails naming the candidates.

```bash
mp inbox move api/fix-auth --top
mp inbox move api/fix-auth --up 2        # also --down, --bottom, --before X, --after X
mp inbox note api/fix-auth "waiting on review"
mp inbox note api/fix-auth --clear
mp inbox snooze api/fix-auth --for 2d    # also --until RFC3339, --clear
```

All three accept stdin JSON instead of flags; `--schema` prints the shape:

```bash
echo '{"piece":"api/fix-auth","up":2}'              | mp inbox move --json
echo '{"piece":"api/fix-auth","note":"needs specs"}' | mp inbox note --json
echo '{"piece":"api/fix-auth","for":"2d"}'           | mp inbox snooze --json
```

## Move through it

```bash
mp inbox next        # the piece after the one you are in; wraps, skips snoozed
mp inbox prev
mp switch --project api --piece fix-auth
mp create --name fix-auth --skip-switch --json
```

`next`/`prev` switch the multiplexer only for an interactive caller; otherwise
they just report where they would go.

## Agents

```bash
mp agent list --json --all     # every live agent across every project
```

`mp wait --timeout 5m` blocks until no agent is working, but unlike everything
else here it is **not** cross-project: it fails outside a git repo and only
covers the pieces of the repo you are standing in. Run it per project, and do
not report "everything has settled" from one repo's answer.

## What changed since last time

`mp history` is an append-only log, which is how you answer "what finished
while I was away" without polling the inbox.

```bash
mp history --json --event piece.merged --since 24h
mp history --json --event 'pr.*' --project api -n 20
```

Events carry `piece_id` alongside `project`/`piece`, so a consumer can follow a
piece across a rename. Useful events: `piece.created`, `piece.updated`,
`piece.merged`, `piece.done`, `piece.abandoned`, `pr.created`, `pr.ready`,
`agent.blocked`, `agent.done`, `inbox.moved`, `inbox.noted`, `inbox.snoozed`.

## Referring to a piece from outside mp

**Key on `id`, not `key`.** `project/piece` is what a human types; it changes
when either name changes. `id` is minted once and never changes.

mp stores nothing about the other system — there is no field for a foreign
identifier, deliberately. The mapping lives on your side: record mp's `id`
against your own record, and reconcile by reading `mp inbox --json` and
`mp history --json`.

`note` is free-form text shown in every picker a human uses. Treat it as theirs
to read, not as a machine field to stuff structured data into.

Two fields are always present but often empty: `agent_status` is `""` and
`agent_counts` is `null` for any piece with no live agent. Switching on the
four status values without handling `""` breaks on most rows.

A piece created before ids existed reports no `id`. Reads never mint one, since
writing metadata dirties the worktree and would make `mp cleanup` refuse the
piece. Materialise one explicitly:

```bash
mp --project api piece show --piece fix-auth --ensure-id --json | jq -r .id
```

`--piece` takes a bare piece name and resolves it against the repo you are
standing in; it rejects a `project/piece` key outright. The leading `--project`
is what points it at another project, so split the `key` on `/` rather than
passing it whole.

## Cautions

- **Never reorder or snooze without being asked.** Rank is the human's own
  judgement about their work; rewriting it silently destroys the one thing the
  inbox is for. Report the order, suggest a change, let them decide.
- A snooze hides a row from `next`/`prev` until it expires. Say so when you
  set one, and say for how long.
- Don't infer that an empty inbox means no work — it means no pieces, which is
  also what an unregistered project looks like. Check `mp project list`.
- The forge is consulted for PR state and the result is cached briefly. A
  failure there warns and leaves `pr` empty rather than failing the list, so
  absent `pr` means "unknown", not "no PR".
