---
name: task-ledger
description: >
  Use the shared task-ledger for multi-agent work. Claim, update, and close
  tasks through MCP (or the HTTP API). Never edit a generated TASKS.md snapshot.
  Use whenever creating, listing, claiming, deferring, verifying, or closing
  tasks, when more than one agent shares a ledger, or when the user mentions
  task-ledger, the live ledger, or T- handles.
---

# task-ledger

Conventions for agents attached to a task-ledger server. The live system is
the write path. Prefer MCP tools. Use HTTP if MCP is not attached.

## Configure

Read from the environment (do not invent URLs or tokens):

| Variable | Meaning | Example |
| --- | --- | --- |
| `LEDGER_URL` | Origin of the server | `https://ledger.example` or `http://127.0.0.1:8080` |
| `LEDGER_TOKEN` | Bearer token (`lgr_…`) | minted once, never in git |
| `LEDGER_OWNER` | Owner slug | `acme` |
| `LEDGER_LEDGER` | Ledger slug | `inbox` |

MCP (Streamable HTTP, same token). Not stdio. Do not wrap the `ledger` binary.

```json
{
  "mcpServers": {
    "task-ledger": {
      "url": "LEDGER_URL/mcp",
      "headers": { "Authorization": "Bearer LEDGER_TOKEN" }
    }
  }
}
```

Cursor does not interpolate env in `mcp.json`. Put the literal URL and token
there. Never put the token in a query string.

A token bound to one ledger is enough. Tools default owner and ledger from it.
HTML `/login` is for humans; agents use the bearer token, not the session cookie.

## Rules

1. Do not edit `TASKS.md` if task-ledger generated it. That file is a snapshot.
2. Claim before you work. Heartbeat if you will run past the lease. Pass
   `ttl_seconds` when you already know it will be long (max 24h, default 30m).
3. Intent-shaped operations only. No whole-object PUT.
4. Actor is the token, not a prefix. Series `T-` is a workstream.
5. `idempotency_key` on every create. Agents retry.
6. Closing needs `evidence`. All checks must be ticked (`set_check`).
7. Moving a task later needs `reason`. A fourth silent deferral is refused
   unless you pass `force`.
8. Prefer `next_task` over list-then-claim.

## MCP tools

`list_ledgers`, `create_ledger`, `create_token` (admin), `list_tasks`,
`get_task`, `create_task`, `claim_task`, `next_task`, `add_note`, `set_check`,
`set_phase`, `close_task`, `verify_task`, `heartbeat_task`, `release_task`.

Resource `ledger://live` is a markdown snapshot. Read-only.

## HTTP fallback

Base: `$LEDGER_URL`. Header: `Authorization: Bearer $LEDGER_TOKEN`.

```
POST /v1/$LEDGER_OWNER/ledgers
POST /v1/$LEDGER_OWNER/tokens
GET  /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/next
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks/T-001/claim
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks/T-001/notes
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks/T-001/checks
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks/T-001/phase
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks/T-001/close
POST /v1/$LEDGER_OWNER/$LEDGER_LEDGER/tasks/T-001/verify
```

Live view: `$LEDGER_URL/$LEDGER_OWNER/$LEDGER_LEDGER`
Snapshot: `$LEDGER_URL/$LEDGER_OWNER/$LEDGER_LEDGER.md`
