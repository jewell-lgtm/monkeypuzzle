# The mp-server piece registry

mp-server's primary purpose is a durable global registry of pieces from different
machines and developers. Each developer's account is private initially; sharing,
teams, and workspace membership are not implemented. Pieces exist independently
of a forge, PR, or background monitoring service.

`mp tracking` explicitly publishes piece snapshots to that registry. The server
stores them in Postgres and provides a dashboard, HTTP API, and MCP `list_pieces`
tool for the authenticated user's pieces across machines and projects.
No other mp command contacts this API, creates
tracking identities, or requires server configuration. Setting the environment
variables alone does not enable automatic publishing. No hooks are installed.

## CLI

Use an existing mp-server deployment configured as described in
[self-hosting](self-hosting.md). Supply a first-party WorkOS AuthKit access token for the server’s configured
application, mapped to your existing mp-server account. The registry verifies
the signature, expiry, application-specific issuer, client ID, and session ID.
WorkOS Connect resource tokens using the MCP authentication are also accepted. A GitHub PAT or session cookie is not
an mp-server access token. This prototype does not implement interactive login
or token refresh; an expired token returns an error.

```sh
export MP_SERVER_URL=https://mp.example.com
export MP_SERVER_TOKEN='your-oauth-access-token'

# From an mp piece:
mp tracking identity
mp tracking report --state working --note 'Implementing the API'
mp tracking report --state blocked --note 'Waiting for a decision'
mp tracking list
mp tracking report --state done
mp settle
```

`report` replaces the whole snapshot. Omitting `--note` clears the previous note.
States are `todo`, `working`, `blocked`, `review`, and `done`. Marking an item
`done` is a workflow report and keeps it in the list. `mp settle` is the separate
“stop showing this to me” action: it removes the published snapshot without
touching the branch, worktree, PR, session, or local piece. Networked tracking
commands emit JSON; `mp settle` emits JSON when piped or with `--json`.
The server home page shows these pieces, with search and filters for project,
machine and state. Location and persistent identity are available on each row.

`--server URL` overrides `MP_SERVER_URL` for an invocation. There is no default
server. `MP_SERVER_TOKEN` is read only by networked tracking commands, kept in
memory, and never saved in the identity file. Requests time out after 15 seconds
and refuse redirects. Use HTTPS, or a loopback URL through an SSH tunnel.

`identity` only accesses local state and requires no server URL or token.
Both `identity` and publishing default to the current local piece. `settle`
uses the same selector shape as other piece verbs, so it can address an item
after removing its worktree from the main repository:

```sh
mp settle old-feature
mp --project my-project settle old-feature
```

`mp settle --project-root ~/Code/my-project --piece old-feature` remains the
advanced form when the project is not registered. `--project-root` and
`--piece` must be used together on tracking subcommands. They also work for `report`
and `identity`, without requiring the project or worktree to exist; snapshots
published this way omit worktree and parent metadata. Run the command on the
machine that owns the piece (including via `mp --host ... --dir ... tracking`);
server credentials must be configured on that machine.

## Stable identity

The CLI generates and saves machine, project and piece identifiers on first
explicit use, in `tracking/identity.json` under mp's user config directory
(`MP_CONFIG_DIR` can override it). File locking and atomic replacement protect
concurrent first use. Back up this file and do not clone it onto another machine.
Malformed state fails instead of silently generating replacement identities.

The machine ID is independent of hostname. Project IDs are mapped to canonical
main-repository paths; piece IDs are mapped to project plus piece name, so
worktrees and repeated CLI processes share the same identity. Remote DELETE
does not remove local identity. In this prototype, moving a repository or
renaming a piece creates a new identity; reusing a piece name reuses its ID.
Checkouts on separate machines are distinct tracked projects. IDs are scoped
again by authenticated account on the server.

## Server API and persistence

The existing `mp-server serve` command registers these routes and applies the
additive `tracked_items` table migration at startup. No new server secrets or
services are required. The existing WorkOS verifier checks signature, issuer,
audience and expiry, then resolves the token subject to a local user. All
storage operations include that authenticated user ID; a request cannot supply
another user's ID. Snapshots are independent of forge PR synchronization.

The registry runs without Temporal or a sync worker. PR monitoring is secondary
and disabled by default (`PR_SYNC_ENABLED=false`). When explicitly enabled,
the legacy repository/PR view lives at `/repositories` and its MCP tools are
added alongside `list_pieces`. It does not run a sync at login. An unavailable
Temporal service does not prevent registry startup.

All routes require `Authorization: Bearer <access-token>`:

| Method | Route | Result |
| --- | --- | --- |
| PUT | `/api/v1/tracking/items/{machine}/{project}/{piece}` | 200, complete stored item |
| DELETE | Same item route | 204, including when absent |
| GET | `/api/v1/tracking/items` | 200, `{"items": [...]}` for your account |

The three route components are persistent identifiers, each 1–128 ASCII
letters, digits, underscores or hyphens. They are not display names. PUT takes
`application/json`, with a 64 KiB body limit:

```json
{"machine":"laptop","project":"monkeypuzzle","piece":"feature-a","state":"working","note":"Implementing tests"}
```

`machine`, `project`, `piece` and `state` are required strings. Optional strings:
`note`, `worktree_path`, `parent`. Fields are limited to 16000 bytes and may not
contain NUL. Unknown fields, invalid states, multiple JSON values, and query
parameters are rejected. Items include the three IDs plus `created_at` and
`updated_at`. Listing orders by most recent change, then by identity.

Identical PUT (`tracking report`) retries return the same item and preserve both timestamps.
Changed snapshots preserve creation time and advance update time. Repeated
DELETE requests succeed and cannot remove another account's copy of the same
identity. Postgres persists data across server restarts.

This is a last-write-wins snapshot prototype: a delayed PUT after DELETE
recreates an item. There are no tombstones, history, ordering/version protocol,
leases, polling, automatic progress collection, or agent integrations. The
snapshot describes the last explicit report, not necessarily current activity.

## Validation

```sh
go test ./...
go test -race ./internal/trackingclient ./internal/server/trackingapi ./internal/server/store
go test -tags integration ./apps/mp -run '^TestCLI_TrackingOptInAndRoundTrip$'
go test -tags integration ./internal/server/mcp -run '^TestMCP_PrivateRegistryWithoutForge$'
# Optional real PostgreSQL test; use a disposable database:
MP_TEST_DATABASE_URL='postgres://localhost/testdb?sslmode=disable' \
  go test ./internal/server/store -run '^TestTrackingPostgresPersistence$' -count=1
# Full compiled CLI + server + production JWT verification + Postgres restart:
MP_TEST_DATABASE_URL='postgres://localhost/testdb?sslmode=disable' \
  go test -tags integration ./apps/mp-server -run '^TestRegistryServerWithoutTemporal$' -count=1 -v
```

Tests cover user and identity isolation, idempotency, concurrent retries and
identity creation, malformed payloads, authentication, redirects, persistence
through a fresh Postgres connection pool, and real CLI processes. The CLI test
also runs ordinary lifecycle/read commands with server settings present and
checks that they send no server requests and create no tracking identity.
