# mp-server — the global piece registry

mp-server is the central registry of pieces published from different machines
and developers. Accounts are private initially: each developer can see only
their own published pieces, across their machines and projects. The CLI remains
fully local unless the developer explicitly invokes `mp tracking`.

The home page lists pieces with project, machine, progress, notes, and last
reported change. Search and filters work without background services. The
[registry API](../../docs/server-tracking.md) and MCP `list_pieces` tool expose
the same account-scoped records. Postgres owns the durable registry; WorkOS
OAuth authenticates API clients and browser sessions use the existing login.

PR monitoring is secondary and disabled by default. `mp-server serve` runs
without Temporal or a worker. To enable the optional `/repositories` view and
PR MCP tools, set `PR_SYNC_ENABLED=true` and run Temporal plus `mp-server worker`.
Login does not trigger a forge sync; use the secondary PR-monitoring view's
explicit Sync action. If Temporal is unavailable at startup, the registry still
starts with monitoring disabled until restart.

Docker Compose defaults to server + Postgres. Enable PR monitoring with
`PR_SYNC_ENABLED=true docker compose --profile pr-monitoring up --build`.
Helm defaults likewise omit the worker and Temporal; opt in with
`prSync.enabled=true` and either `temporal.enabled=true` or an external Temporal.

Source-available under **FSL-1.1-MIT** (see [LICENSE](./LICENSE)) — not the
repo's default MIT. Build with `make build-server` (→ `bin/mp-server`) or the
[Dockerfile](./Dockerfile).
