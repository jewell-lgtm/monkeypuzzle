# Licensing

Monkeypuzzle is licensed per-component. Different directories carry different
licenses, summarized below. Where a subdirectory contains its own `LICENSE`
file, that file governs everything beneath it and overrides the repository
default.

## Default: MIT

The repository's root [`LICENSE`](./LICENSE) (MIT) applies to all code **except**
the directories listed under "Source-available" below. This covers the open
source components:

- `apps/mp` — the `mp` CLI
- `apps/mp-mcp` — the `mp` MCP server
- `apps/tmux` — the tmux plugin
- `internal/*` shared packages (except `internal/server`)
- `pkg/*`

## Source-available: FSL-1.1-MIT

The server is **source-available**, not open source. Self-hosting,
modification, internal use, non-commercial education/research, and professional
services are permitted; offering a competing commercial product or service is
not. Each copy converts to MIT two years after its release (see the license
text for details).

Governed by [Functional Source License 1.1 (MIT future)](https://fsl.software):

- `apps/mp-server` — see [`apps/mp-server/LICENSE`](./apps/mp-server/LICENSE)
- `internal/server` — see [`internal/server/LICENSE`](./internal/server/LICENSE)

## Trademark

Software licenses do **not** grant any rights to the "Monkey Puzzle" name or
logo. See [`TRADEMARK.md`](./TRADEMARK.md).
