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

`list_tasks` is a thin index (handle, title, phase, size, tags, claimant).
Default list hides DONE older than the ledger's archive window. Pass
`done=true` for every DONE task, and only those. Pass `tag` to keep tasks
with that slug. `get_task` still works on a hidden handle. Do not delete
DONE tasks.

## Tags

A tag is an optional filter chip. It is not a project, not a folder, and
not a ledger. Do not invent a second "project" field.

- At most three. Lowercase slugs, same charset as owner slugs
  (`^[a-z0-9][a-z0-9-]{0,62}$`). Duplicates and blanks are dropped.
  Empty `set_tags` clears.
- Set them on `create_task`. Replace later with `set_tags`. Filter with
  **one** tag only: `list_tasks` `tag=` or the HTML `?tag=` chips. No
  AND, no OR, no colours.
- Isolation (who may write, which agent, which token) is a
  **ledger-bound token**, never a tag. Extra ledgers are a paid hosted
  feature; do not create one to stand in for a tag.

When to tag:

- **Mixed-purpose board** (several products or hats on one ledger): tag
  on create so a human can filter. Reuse a slug that already appears on
  the board before minting a new one. Examples: `ledger`, `site`,
  `finance`.
- **Dedicated product ledger**: skip a product-name tag. The ledger *is*
  the product. Tag only if that board itself is mixed (for example
  `docs` vs `billing` on that product).

When not to tag:

- Phase, size, claimant, handle, or a file path in a large repo. Those
  already have fields, or they are too fine to filter on.
- A substitute for claiming, for a second series, or for a second board.

If this session's `create_task` has no `tags` field (or `set_tags` /
`review_url` are missing), the MCP schema is stale. Ask the human to
refresh MCP settings and start a **new chat**. Until then, HTTP
`POST /v1/:owner/:ledger/tasks/:handle/tags` with the bearer from
harness config. Do not print the token. Do not skip the tag.

HTML `/login` is for humans. Agents use the bearer token, not the session
cookie.

When the human wants to see the live board in a browser (Cursor Browser or
otherwise), call `review_url` and open the returned URL. Do not paste the
bearer token into `/login`. Do not put the URL in a task note. The link
works once and expires in 15 minutes. A write token is enough.

## TASKS.md in a git repo

If a repo still has a hand-written `TASKS.md`, that is not a ledger snapshot.
Do not treat it as the write path once this server is in use. Do not edit a
file the server generated (`/{owner}/{ledger}.md`). Until the humans migrate,
ask which write path they mean.

## Identity

The bearer token is the actor. Do not send an impersonation header or an
`actor=` field. Two tokens with the same actor name still look like one
agent on the board. Give each seat its own write token.

Cursor chats in one window share one MCP process, one session, and one
token. Extra servers in `.cursor/mcp.json` do not isolate chats.
Isolation there is `claim_id` from `claim_task` / `next_task`, kept in
that chat only.

## Rules

1. Claim before you work. Keep `claim_id` from `claim_task` or
   `next_task` in this chat. Pass it on heartbeat, re-claim, release,
   close, and phase while the lease is live. Do not put it in a note or
   share it. Heartbeat if you will run past the lease. Pass
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
`close_task`, `verify_task`, `heartbeat_task`, `release_task`, `review_url`,
`set_tags`.

`create_ledger` as owner admin mints a ledger-bound write token (once) and
returns an `mcp` object named `task-ledger-<slug>`. That is the project
server. Keep the owner admin token in `task-ledger-admin`.

Resource `ledger://live` is a markdown snapshot. Read-only.
