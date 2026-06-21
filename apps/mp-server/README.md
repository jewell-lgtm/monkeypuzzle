# mp-server — the dashboard server

The web app behind Monkeypuzzle: sign in with GitHub (via WorkOS), sync your
PRs, and see every stack as a live tree. Also serves the `/mcp` endpoint for
agents. Run it hosted, or self-host it.

Source-available under **FSL-1.1-MIT** (see [LICENSE](./LICENSE)) — not the
repo's default MIT. Build with `make build-server` (→ `bin/mp-server`) or the
[Dockerfile](./Dockerfile).
