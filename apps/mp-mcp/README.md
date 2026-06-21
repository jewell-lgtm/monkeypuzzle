# mp-mcp — MCP bridge

A thin [MCP](https://modelcontextprotocol.io) server that exposes the `mp`
workflow to AI assistants (e.g. Claude). It shells out to the `mp` binary it
finds as a sibling (same dir, else on `PATH`), so install it alongside `mp`.

MIT. Build with `make build` (→ `bin/mp-mcp`).
