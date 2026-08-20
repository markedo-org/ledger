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
down, not that we serve SSE.

From a release binary or `go install`: `ledger serve` (flags: `-listen`,
`-db`, `-boot-owner`, `-boot-ledger`, `-boot-actor`). From a clone:
`make run` builds and listens on `127.0.0.1:8080` with `ledger.sqlite`.

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

`ledger mcp print` emits this JSON from `~/.ledger/config`. See
[cli.md](cli.md).

Install the agent skill with `npx skills add markedo-org/ledger -s task-ledger`.
See `.agents/skills/task-ledger/`.

## Tools

`list_ledgers`, `create_ledger`, `create_token`, `list_tokens`, `revoke_token`,
`reset_ledger`,
`create_owner`, `set_max_ledgers`, `list_tasks`, `get_task`, `create_task`,
`claim_task`, `next_task`, `add_note`, `set_check`, `set_phase`,
`close_task`, `verify_task`, `heartbeat_task`, `release_task`, `review_url`,
`set_tags`. `set_phase` accepts optional `force` to override a fourth deferral.

`create_owner` and `set_max_ledgers` need the operator token
(`LEDGER_OPERATOR_TOKEN`). `create_ledger` and `create_token` accept owner admin
or operator; the operator may mint role write but not admin for an owner that
already exists. `list_tokens`, `revoke_token`, and `reset_ledger` are owner admin
only. A ledger-bound write token is refused for token list and revoke. `reset_ledger` requires `confirm` equal to
`owner/ledger`. It wipes every task and restarts the series at T-001.
Write tokens are refused. Tokens and the ledger row stay.

`create_token` returns plaintext and id once. Prefer `next_task` over list-then-claim. `create_task` requires `idempotency_key`.
`close_task` requires `evidence`. Moving a task later requires `reason`.

`claim_task` and `next_task` return `claim_id` once. Keep it in that chat.
Pass it on heartbeat, same-actor re-claim, release, close, phase, checks, and
tags while the lease is live. `set_check` and `set_tags` require it the same
way as `set_phase` and `close_task`. `get_task`, `list_tasks`, and HTML never include it.
Leases created before this field stay on the old same-actor refresh until
they expire.

If the `claim_id` is gone, because the chat was compacted or the session died,
you are not locked out. `claim_task` with `steal: true` and a `reason` takes
the lease back and mints a new `claim_id`, on your own actor as well as someone
else's, and the steal is written to the event log either way. An owner admin
token can also `release_task` any live lease without the `claim_id`.

`review_url` returns a one-time URL (`GET /login/review?code=`). Open it in
a browser for the human. Do not paste the bearer token. Do not put the URL
in a task note. Write tokens may mint. The operator secret cannot.

The session carries the same role as the token that minted it. That is what
makes the link useful: it is how an agent hands its own human a way into their
own board, and an owner should not have to go looking for another credential to
change their own retention when their agent is holding the admin token already.

Choose the power by choosing the token. Mint from a write token when the reader
should only look, and from the admin token when they are the owner and may need
to change something.

`create_task` accepts optional `tags` (at most three lowercase slugs,
same charset as owner slugs). `set_tags` replaces them; an empty list
clears. `list_tasks` accepts one `tag` to keep tasks with that slug.
A tag is a filter chip, not a ledger and not a nested project. Isolation
is a ledger-bound token. See the task-ledger skill for when to tag.

`list_tasks` hides DONE older than `archive_done_after_days` (process default
7, or the ledger override). Pass `done: true` for every DONE task, and only
those. `get_task` still loads a hidden handle. There is no purge tool. The
server may delete DONE after `purge_done_after_days` (default 0, never).

`list_tokens` returns id and metadata only, never the secret or its hash.
`revoke_token` takes a token id. Revocation is irreversible. It invalidates
the bearer everywhere, deletes HTML sessions signed in with that token, and
clears any outstanding magic link or review link bound to it. The row stays
for audit. Revoking twice is a no-op. A token cannot revoke itself: mint the
replacement first, update every config that held the old token, then revoke
the old id with the new token.

To rotate a token: list to find the id you are replacing; mint the replacement
(and if the old token had an email bound, revoke the old one before you can
reuse that address); put the new token in every config that held the old one;
revoke the old id with the new token.

## Resource

`ledger://live` is a markdown snapshot of the token's ledger. Read-only.
