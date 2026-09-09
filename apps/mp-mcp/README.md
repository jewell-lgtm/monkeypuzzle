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
| `mp_inbox`        | `mp inbox --json [--sort rank\|urgency] [--refresh]` — the global cross-repo inbox, ordered by manual rank then urgency |
| `mp_inbox_move`   | `mp inbox move --json` (stdin: `piece` + one of `top`/`bottom`/`up`/`down`/`before`/`after`) |
| `mp_inbox_note`   | `mp inbox note --json` (stdin: `piece`, `note` or `clear`) |
| `mp_inbox_snooze` | `mp inbox snooze --json` (stdin: `piece`, `for`/`until`/`clear`) |
| `mp_inbox_next`   | `mp inbox next --json [--sort …]` — returns the switch result; no multiplexer switch for a non-TTY caller |
| `mp_inbox_prev`   | `mp inbox prev --json [--sort …]` — same as above, stepping backwards |

Every tool also accepts `cwd`, the directory `mp` runs in.
