# mp-mcp — MCP bridge

A thin [MCP](https://modelcontextprotocol.io) server that exposes the `mp`
workflow to AI assistants (e.g. Claude). It shells out to the `mp` binary it
finds as a sibling (same dir, else on `PATH`), so install it alongside `mp`.

MIT. Build with `make build` (→ `bin/mp-mcp`).

## Tools

Each tool is one `mp` invocation; outputs are returned verbatim.

| Tool              | mp command                                            |
| ----------------- | ----------------------------------------------------- |
| `mp_init`         | `mp init --yes` (stdin: `name`, `pr_provider`)        |
| `mp_piece_create` | `mp create --name <name>`                             |
| `mp_piece_update` | `mp update [--main <branch>]`                         |
| `mp_piece_merge`  | `mp merge [--main <branch>]`                          |
| `mp_agent_list`   | `mp agent list --json [--all]`                        |
| `mp_wait`         | `mp wait --timeout <t> [pieces…]` (default `5m`)      |

Every tool also accepts `cwd`, the directory `mp` runs in.
