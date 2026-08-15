---
name: task-ledger
description: >
  Use the shared task-ledger for multi-agent work. Claim, update, and close
  tasks through MCP. Never edit a generated markdown snapshot. Use whenever
  creating, listing, claiming, deferring, verifying, or closing tasks, when
  more than one agent shares a ledger, or when the user mentions task-ledger,
  the live ledger, or T- handles.
---

# task-ledger

The live MCP server is the write path. Use the task-ledger tools already
attached to this session. Do not pick a URL. Do not wrap the `ledger` binary
as stdio MCP. Do not call the HTTP API unless the human gave you a base URL
and said MCP is down.

The harness config (Cursor `.cursor/mcp.json`, Claude Code `.mcp.json`) is
where the origin and bearer token live. This skill does not name a host.
Humans provision with the `ledger` CLI (`init`, `mcp print`, `token mint`).
Do not wrap that binary as stdio MCP.

## If the tools are missing

Stop. Ask the human to attach the Streamable HTTP server. Do not guess
`task-ledger.com` or localhost. Do not invent a token. The README and the
hosted signup page show the JSON shape.

## Which token, which MCP config

Signup issues an **owner admin** token. It is not bound to the first ledger.
It can create ledgers and mint tokens. If that owner has exactly one ledger,
tools default to it (this is the usual signup token). If the owner has
several, pass `ledger` or mint a token bound to one project.

Two setups:

1. An agent that creates ledgers or mints tokens. Give it the owner admin
   token in an MCP server named for admin (`task-ledger-admin`). If that
   same agent also works a project, add a second server named for that
   ledger, with a **ledger-bound write** token. Do not use the admin token
   as the project server.
2. An agent that only works one project. Give it only a ledger-bound write
   token, in a server named for that project. Do not give it the owner
   admin token.

`list_tasks` is a thin index (handle, title, phase, size, claimant).
Default list hides DONE older than the ledger's archive window. Pass
`done=true` for every DONE task, and only those. `get_task` still works
on a hidden handle. Do not delete DONE tasks.

HTML `/login` is for humans. Agents use the bearer token, not the session
cookie.

## TASKS.md in a git repo

If a repo still has a hand-written `TASKS.md`, that is not a ledger snapshot.
Do not treat it as the write path once this server is in use. Do not edit a
file the server generated (`/{owner}/{ledger}.md`). Until the humans migrate,
ask which write path they mean.

## Rules

1. Claim before you work. Heartbeat if you will run past the lease. Pass
   `ttl_seconds` when you already know it will be long (max 24h, default 30m).
2. Intent-shaped operations only. No whole-object PUT.
3. Actor is the token, not a prefix. Series `T-` is a workstream.
4. `idempotency_key` on every create. Agents retry.
5. Closing needs `evidence`. All checks must be ticked (`set_check`).
6. Moving a task later needs `reason`. A fourth silent deferral is refused
   unless you pass `force`.
7. Prefer `next_task` over list-then-claim.
8. `list_tasks` is a thin index. `get_task` before you act.
9. `verified_at` is set when someone verifies, not at create.
10. Do not delete DONE tasks. Use `list_tasks` with `done=true` to see the archive.

## Tools

`list_ledgers`, `create_ledger`, `create_token` (admin), `create_owner` and
`set_max_ledgers` (operator), `list_tasks`, `get_task`, `create_task`,
`claim_task`, `next_task`, `add_note`, `set_check`, `set_phase`,
`close_task`, `verify_task`, `heartbeat_task`, `release_task`.

`create_ledger` as owner admin mints a ledger-bound write token (once) and
returns an `mcp` object named `task-ledger-<slug>`. That is the project
server. Keep the owner admin token in `task-ledger-admin`.

Resource `ledger://live` is a markdown snapshot. Read-only.
