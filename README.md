# task-ledger

A task ledger for many agents. Markedo AS. MIT.

Agents share one live list without overwriting each other. Humans get a page that
reads like `TASKS.md`. The live system is the write path. A markdown snapshot is
optional and read-only.

- Binary and slug: `ledger`
- Module: `github.com/markedo-org/ledger`
- Design: [`docs/design.md`](docs/design.md)

## Quick start

```bash
make build
LEDGER_BOOT_TOKEN=lgr_dev ./ledger -listen 127.0.0.1:8080 -db ledger.sqlite \
  -boot-owner markedo -boot-ledger markedo-meta -boot-actor maria
```

On an empty database the boot token is stored hashed and printed once. Set
`LEDGER_BOOT_TOKEN` so you choose it. Subsequent starts do not print a token.

```bash
export LEDGER_TOKEN=lgr_dev
curl -s -H "Authorization: Bearer $LEDGER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"First task","idempotency_key":"demo-1"}' \
  http://127.0.0.1:8080/v1/markedo/markedo-meta/tasks

open http://127.0.0.1:8080/markedo/markedo-meta
curl -s http://127.0.0.1:8080/markedo/markedo-meta.md
```

Go 1.24+. `make test` / `make lint`. Default listen is localhost. HTML is
read-only and unauthenticated in v1 (bind to loopback). The API requires a
bearer token.

## API

All mutating routes need `Authorization: Bearer`. Actor is the token, not a
field on the body. Idempotency: JSON `idempotency_key` or header `Idempotency-Key`.

| Method | Path | What |
| --- | --- | --- |
| POST | `/v1/:owner/:ledger/tasks` | Create (`title`, optional `body`, `phase`, `size`, `prefix`, `checks`, `ref`) |
| GET | `/v1/:owner/:ledger/tasks` | List |
| GET | `/v1/:owner/:ledger/tasks/:handle` | Get `T-001` |
| POST | `/v1/:owner/:ledger/tasks/:handle/claim` | Claim (`ttl_seconds`, optional `steal`+`reason`) |
| POST | `/v1/:owner/:ledger/tasks/:handle/heartbeat` | Extend lease |
| POST | `/v1/:owner/:ledger/tasks/:handle/release` | Drop claim |
| POST | `/v1/:owner/:ledger/tasks/:handle/phase` | Move phase (`phase`, `reason` if deferring) |
| POST | `/v1/:owner/:ledger/tasks/:handle/notes` | Append note |
| POST | `/v1/:owner/:ledger/tasks/:handle/close` | Close (`evidence` required) |
| POST | `/v1/:owner/:ledger/tasks/:handle/verify` | Refresh verified date |
| POST | `/v1/:owner/:ledger/next` | Claim the next eligible NOW task |

Handles are `T-001` (one-letter series, default `T`). Default lease is 30
minutes; pass `ttl_seconds` to ask for longer, up to 24 hours.

## MCP

Streamable HTTP at `/mcp`, same bearer token. Stateless. Official Go SDK v1.2.0,
protocol 2025-06-18. See [`docs/mcp.md`](docs/mcp.md).

```json
{
  "mcpServers": {
    "task-ledger": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer lgr_dev" }
    }
  }
}
```

## Layout

| Path | What |
| --- | --- |
| `cmd/ledger/` | Server binary |
| `internal/app/` | Domain operations |
| `internal/store/` | SQLite |
| `internal/web/` | HTTP API, HTML |
| `internal/mcpserver/` | Streamable HTTP MCP |
| `docs/design.md` | Settled v1 cut |
| `docs/mcp.md` | MCP endpoint, auth, tools |
| `docs/design.md` | Settled v1 cut |
| `.agents/skills/task-ledger/` | Agent skill |

## Licence

MIT. Self-host is first-class. Markedo will also run a hosted instance and use it.
