---
name: task-ledger
description: >
  Use the task-ledger server as the write path for shared work. Claim, update,
  and close tasks through the HTTP API (and MCP once it exists). Never edit a
  generated TASKS.md snapshot. Use when creating, listing, claiming, deferring,
  or closing tasks, or when more than one agent shares a ledger.
---

# task-ledger

This skill carries **conventions**. Operations live on the server.

## Rules

1. The live system is the write path. Do not edit `TASKS.md` if it was generated
   by task-ledger. That file is a snapshot.
2. Claim a task before you work on it. Heartbeat if the work will run past the
   lease. Request a longer TTL when you already know it will.
3. Use intent-shaped operations (`set-phase`, `add-note`, `defer`, `close` with
   evidence). Do not reconstruct and PUT a whole task object.
4. Identify yourself as an actor on every mutation. Series (`T-`) is a
   workstream, not your name.
5. Auth is `Authorization: Bearer <token>`. Never put the token in the URL.

## This repo

When this skill is copied into a consumer, set the owner, ledger, and base URL
below (or via `LEDGER_URL` / `LEDGER_TOKEN` / `LEDGER_ACTOR`).

- Base URL: `http://127.0.0.1:8080`
- Owner / ledger: *(set me)*
- Actor: `$LEDGER_ACTOR`

## Attach

MCP (Streamable HTTP) is the copy-paste attach path once the server exposes it.
Until then, call the HTTP API. Do not wrap the `ledger` binary in a local stdio
MCP server.
