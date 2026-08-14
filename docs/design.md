# task-ledger v1 cut

Settled 2026-08-12. Longer reasoning lives in the Markedo meta-repo at
`docs/ideas/agent-task-server.md`. This file is what to build.

## What it is

A server. Same binary self-hosted or hosted. Four agents point at one URL.
Owner slug, then ledgers as projects (`markedo/dispatch`). Default series `T`,
handles `T-001`…, one-letter prefix, at most five series. Actor is who did it
and is not the series.

## Interfaces

1. HTTP API (the system).
2. HTML live view, read-only in v1.
3. Skill: conventions (do not edit the snapshot, claim before you work, bearer
   token, this repo's owner/ledger).
4. MCP, Streamable HTTP at `/mcp`, over the same domain layer. Copy-paste attach
   path. Not stdio. Not a CLI wrapper. See `docs/mcp.md`.
5. CLI later, as an HTTP client.

URLs:

```
/:owner/:ledger
/:owner/:ledger/:task
/:owner/:ledger.md
```

Auth: browser session from a bearer token (the identity) or optional GitHub
OAuth. A bearer token is enough to dogfood and to self-host. API and MCP use
`Authorization: Bearer`. No API keys in query strings. No user table.

## Semantics that cannot wait

- Two identities: uuid plus human handle, allocated in the insert transaction.
  Numbers never reused. Idempotency key on create.
- Intent-shaped operations. Append-only notes. No `PUT` of a whole task.
- Claims are leases (default 30 minutes, agent may request longer). Reaper.
  Pull (`next`), do not push. `steal` is logged.
- Event log from the first write. Materialised task table is the truth.
- Schema copied from Markedo `TASKS.md`: phases NOW/NEXT/LATER/GATED/PARKED/DONE,
  gates as flags, size S|M|L, explicit rank, sub-checkboxes as child items
  without IDs.
- Policy: evidence on close, reason on deferral, no fourth silent deferral.
  Stale verification warns and allows override.
- SQLite in WAL, one process. `max_ledgers` on the owner is the meter.
  Stripe is not in this binary. See [`provisioning.md`](provisioning.md).

## Meter and provisioning (next slice)

Ledger owns the meter. The hosted site owns money. Same binary, same port.
Self-hosters call the same operator routes we will call from
`www.task-ledger.com`.

- Host-level operator token (not owner `admin`): create owners, create
  ledgers, set `max_ledgers` (floor 1). Optional HTML admin page later.
- Overflow freeze: when cap is below ledger count, newest ledgers are
  read-only; oldest `max_ledgers` stay writable. Same-second creates follow
  insert order. Derive, do not store a flag.
- Stripe, webhooks, and the provisioning client come after that API exists.
  Working hosted prices live in `provisioning.md`.

## Out of v1

- CLI.
- Folder-watch sync of `TASKS.md` (on-demand markdown export is in).
- Public read-only ledger (a boolean; v1.1).
- Browser write form.
- Postgres, change streams, project-management features.

## Human write path in v1

Ask an agent. Do not make the markdown snapshot writable to paper over a missing
form.

## Hosting

Markedo hosted: apex `https://task-ledger.com` is the app. Brochure and signup
are `https://www.task-ledger.com`. `GET /` with no extra path is configurable
(`login`, `url`, or `file`). Hosted uses `url` → the brochure. The app does
not live under `app.`. Live view, API, and MCP stay on the apex. Self-host
is the same binary; see `docs/deploy.md`.
