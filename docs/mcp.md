# MCP

task-ledger exposes a remote MCP server over **Streamable HTTP**.

| | |
| --- | --- |
| Endpoint | `POST /mcp` (same origin as the API) |
| Transport | Streamable HTTP, **stateless**, JSON responses |
| SDK | `github.com/modelcontextprotocol/go-sdk` v1.2.0 (official) |
| Protocol | 2025-06-18 |
| Auth | `Authorization: Bearer` (same tokens as the HTTP API) |

Not stdio. Not a wrapper around a CLI. Not the legacy SSE transport.

The `ledger` process must be listening. Cursor tries Streamable HTTP first, then
falls back to SSE when that fetch fails. `ECONNREFUSED` means the process is
down, not that we serve SSE. Start it with `make run`.

## Cursor / Claude Code config

A project-only agent needs one server, named for that ledger, with a
ledger-bound write token. A provisioner uses a server named for admin, with
the owner admin token. If the same agent does both, keep both servers. See
[`contrib/mcp.json.example`](../contrib/mcp.json.example).

```json
{
  "mcpServers": {
    "task-ledger-admin": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": {
        "Authorization": "Bearer <owner-admin-token>"
      }
    },
    "task-ledger-inbox": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": {
        "Authorization": "Bearer <ledger-write-token>"
      }
    }
  }
}
```

A token bound to one ledger is enough for project work. An owner-scoped
token also defaults when that owner has exactly one ledger (the usual
signup token). With several ledgers, pass `ledger` or mint a bound token.

Install the agent skill with `npx skills add markedo-org/ledger -s task-ledger`.
See `.agents/skills/task-ledger/`.

## Tools

`list_ledgers`, `create_ledger`, `create_token`, `create_owner`,
`set_max_ledgers`, `list_tasks`, `get_task`, `create_task`, `claim_task`,
`next_task`, `add_note`, `set_check`, `set_phase`, `close_task`,
`verify_task`, `heartbeat_task`, `release_task`.

`create_owner` and `set_max_ledgers` need the operator token
(`LEDGER_OPERATOR_TOKEN`). `create_ledger` / `create_token` accept owner
admin or operator.

Prefer `next_task` over list-then-claim. `create_task` requires `idempotency_key`.
`close_task` requires `evidence`. Moving a task later requires `reason`.
`set_phase` accepts `force` to override the fourth-deferral block.

## Resource

`ledger://live` is a markdown snapshot of the token's ledger. Read-only.
