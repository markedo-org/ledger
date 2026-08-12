---
name: task-ledger
description: >
  Use the task-ledger HTTP API as the write path for shared work. Claim, update,
  and close tasks through the API. Never edit a generated TASKS.md snapshot. Use
  when creating, listing, claiming, deferring, or closing tasks, or when more
  than one agent shares a ledger.
---

# task-ledger

This skill carries **conventions**. Operations are HTTP calls.

## Rules

1. The live system is the write path. Do not edit `TASKS.md` if it was generated
   by task-ledger. That file is a snapshot.
2. Claim a task before you work on it. Heartbeat if the work will run past the
   lease. Request a longer `ttl_seconds` when you already know it will.
3. Use intent-shaped operations. Do not reconstruct and PUT a whole task object.
4. Identify yourself via the bearer token's actor. Series (`T-`) is a workstream,
   not your name.
5. Auth is `Authorization: Bearer <token>`. Never put the token in the URL.
6. Send `idempotency_key` on create. Agents retry.

## This repo (dogfood)

- Base URL: `http://127.0.0.1:8080`
- Owner / ledger: `markedo` / `markedo-meta`
- Token: `$LEDGER_TOKEN`
- Actor: baked into the token (boot default `maria`)

## Calls

```
POST /v1/markedo/markedo-meta/tasks
{"title":"…","body":"…","phase":"NOW","size":"M","idempotency_key":"…"}

POST /v1/markedo/markedo-meta/next
{"prefix":"T"}

POST /v1/markedo/markedo-meta/tasks/T-001/claim
{"ttl_seconds":1800}

POST /v1/markedo/markedo-meta/tasks/T-001/notes
{"body":"…"}

POST /v1/markedo/markedo-meta/tasks/T-001/phase
{"phase":"NEXT","reason":"waiting on review"}

POST /v1/markedo/markedo-meta/tasks/T-001/close
{"evidence":"commit abc123"}
```

Live view: `http://127.0.0.1:8080/markedo/markedo-meta`
Snapshot: `http://127.0.0.1:8080/markedo/markedo-meta.md`

MCP (Streamable HTTP) is not shipped yet. Do not wrap the `ledger` binary in a
local stdio MCP server.
