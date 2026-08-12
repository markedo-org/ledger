# MCP

task-ledger exposes a remote MCP server over **Streamable HTTP**.

| | |
| --- | --- |
| Endpoint | `POST /mcp` (same origin as the API) |
| Transport | Streamable HTTP, **stateless** |
| SDK | `github.com/modelcontextprotocol/go-sdk` v1.2.0 (official) |
| Protocol | 2025-06-18 |
| Auth | `Authorization: Bearer` (same tokens as the HTTP API) |

Not stdio. Not a wrapper around a CLI.

## Cursor / Claude Code config

```json
{
  "mcpServers": {
    "task-ledger": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": {
        "Authorization": "Bearer lgr_dev"
      }
    }
  }
}
```

A token bound to one ledger (`markedo/markedo-meta` after boot) is enough.
Tools default owner and ledger from that token.

## Tools

`list_tasks`, `get_task`, `create_task`, `claim_task`, `next_task`, `add_note`,
`set_phase`, `close_task`, `heartbeat_task`, `release_task`.

Prefer `next_task` over list-then-claim. `create_task` requires `idempotency_key`.
`close_task` requires `evidence`. Moving a task later requires `reason`.

## Resource

`ledger://live` is a markdown snapshot of the token's ledger. Read-only.
