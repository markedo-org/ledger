# task-ledger

When several agents share a project file, the last save wins. task-ledger is
one live list on a server. Claim a task for a limited time. Everyone else can
see that, and they wait.

One Go binary, SQLite on disk. HTTP API, a read-only HTML board, and
Streamable HTTP MCP. No user table: a minted bearer token is the identity.

Self-host is the default. A hosted instance is at
[task-ledger.com](https://task-ledger.com) if you do not want to run the
binary. Same code.

Go module: `github.com/markedo-org/ledger`. Binary name: `ledger`.

## Quick start

Go 1.24+.

```bash
git clone https://github.com/markedo-org/ledger.git
cd ledger
make build
LEDGER_BOOT_TOKEN=lgr_dev ./ledger -listen 127.0.0.1:8080 -db ledger.sqlite
```

An empty database creates owner `acme`, ledger `inbox`, and actor `ada`. The
boot token is stored hashed and printed once. Set `LEDGER_BOOT_TOKEN` so you
choose it. Later starts do not print a token.

```bash
export LEDGER_TOKEN=lgr_dev
curl -s -H "Authorization: Bearer $LEDGER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"First task","idempotency_key":"demo-1"}' \
  http://127.0.0.1:8080/v1/acme/inbox/tasks

open http://127.0.0.1:8080/acme/inbox
curl -s http://127.0.0.1:8080/acme/inbox.md
```

`make test` / `make lint`. Default listen is localhost. HTML is a read-only
board. Sign in at `/login` with a bearer token.

## MCP

Streamable HTTP at `/mcp`, same bearer token. Stateless. See
[`docs/mcp.md`](docs/mcp.md).

A project-only agent gets one server, named for that ledger, with a
ledger-bound write token. An agent that creates ledgers or mints tokens gets
a second server named for admin, with the owner admin token. Both in one
file:

```json
{
  "mcpServers": {
    "task-ledger-admin": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer <owner-admin-token>" }
    },
    "task-ledger-inbox": {
      "url": "http://127.0.0.1:8080/mcp",
      "headers": { "Authorization": "Bearer <ledger-write-token>" }
    }
  }
}
```

Paste that into Cursor (`.cursor/mcp.json` or Settings → MCP) or Claude Code
(project `.mcp.json`). Write the URL and token out. Cursor does not interpolate
environment variables. ChatGPT and Claude on the web usually want OAuth. That
is not in this binary yet. A fuller example is
[`contrib/mcp.json.example`](contrib/mcp.json.example).

Then install the agent skill. It teaches the loop (claim, note, close). It
does not choose a host.

```bash
npx skills add markedo-org/ledger -s task-ledger
```

Canonical files: `.agents/skills/task-ledger/`.

## Hosted

Sign up at [www.task-ledger.com/signup](https://www.task-ledger.com/signup).
The first ledger is free. Extra ledgers are paid. The live board is on the
apex, [task-ledger.com](https://task-ledger.com).

## Production

[`docs/deploy.md`](docs/deploy.md) is the self-host path (binary, systemd,
nginx). [`docs/auth.md`](docs/auth.md) covers token login, optional GitHub
OAuth, and magic-link email. [`docs/provisioning.md`](docs/provisioning.md)
covers `max_ledgers` and freeze.

`LEDGER_OPERATOR_TOKEN` enables `/admin` and `POST /v1/owners`.
`LEDGER_HTML_AUTH=1` locks the live view. The API and MCP still need a bearer
token.

## API

Mutating routes need `Authorization: Bearer`. Actor is the token, not a field
on the body. Create requires `idempotency_key` (JSON or header
`Idempotency-Key`).

| Method | Path | What |
| --- | --- | --- |
| GET | `/v1/owners` | List owners (operator) |
| POST | `/v1/owners` | Create owner (operator). Optional `ledger` + `actor` mints an admin token once. |
| GET | `/v1/owners/:owner` | Owner, cap, ledgers, which are frozen |
| PATCH | `/v1/owners/:owner` | Set `max_ledgers` (operator). `0` is unlimited. |
| GET | `/v1/:owner/ledgers` | List ledgers |
| POST | `/v1/:owner/ledgers` | Create ledger (admin; enforces `max_ledgers`) |
| POST | `/v1/:owner/tokens` | Mint a bearer token (admin; plaintext returned once). Optional `email`. |
| POST | `/v1/:owner/:ledger/tasks` | Create |
| GET | `/v1/:owner/:ledger/tasks` | List |
| GET | `/v1/:owner/:ledger/tasks/:handle` | Get `T-001` |
| POST | `/v1/:owner/:ledger/tasks/:handle/claim` | Claim |
| POST | `/v1/:owner/:ledger/tasks/:handle/heartbeat` | Extend lease |
| POST | `/v1/:owner/:ledger/tasks/:handle/release` | Drop claim |
| POST | `/v1/:owner/:ledger/tasks/:handle/phase` | Move phase |
| POST | `/v1/:owner/:ledger/tasks/:handle/notes` | Append note |
| POST | `/v1/:owner/:ledger/tasks/:handle/checks` | Tick or untick a check |
| POST | `/v1/:owner/:ledger/tasks/:handle/close` | Close (`evidence` required) |
| POST | `/v1/:owner/:ledger/tasks/:handle/verify` | Refresh verified date |
| POST | `/v1/:owner/:ledger/next` | Claim the next eligible NOW task |

Handles are `T-001` (one-letter series, default `T`). Default lease is 30
minutes.

## Layout

| Path | What |
| --- | --- |
| `cmd/ledger/` | Server binary |
| `internal/app/` | Domain operations |
| `internal/store/` | SQLite |
| `internal/web/` | HTTP API, HTML |
| `internal/mcpserver/` | Streamable HTTP MCP |
| `docs/design.md` | Settled v1 cut |
| `docs/deploy.md` | Self-host and production |
| `.agents/skills/task-ledger/` | Agent skill |

## License

MIT. See [LICENSE](LICENSE). Report a vulnerability in
[SECURITY.md](SECURITY.md).
