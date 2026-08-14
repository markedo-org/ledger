# task-ledger

A shared task list for people and AI agents. MIT. Markedo AS.

When several agents work the same project, a shared file becomes a fight. The
last save wins. task-ledger is one live list on a server. If you are on a task,
you claim it for a limited time. Everyone else can see that, and they wait.

- One Go binary, SQLite on disk, nginx in front
- HTTP API, read-only HTML live view, Streamable HTTP MCP
- No user table. A minted bearer token is the identity
- Self-host is first-class. Markedo also runs a hosted instance at
  [task-ledger.com](https://task-ledger.com)
  ([www.task-ledger.com](https://www.task-ledger.com))

Module: `github.com/markedo-org/ledger`. Binary and slug: `ledger`.

## Quick start

Go 1.24+.

```bash
git clone https://github.com/markedo-org/ledger.git
cd ledger
make build
LEDGER_BOOT_TOKEN=lgr_dev ./ledger -listen 127.0.0.1:8080 -db ledger.sqlite
```

On an empty database the process creates owner `acme`, ledger `inbox`, and
actor `ada`, stores the boot token hashed, and prints it once. Set
`LEDGER_BOOT_TOKEN` so you choose it. Later starts do not print a token.

```bash
export LEDGER_TOKEN=lgr_dev
curl -s -H "Authorization: Bearer $LEDGER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"First task","idempotency_key":"demo-1"}' \
  http://127.0.0.1:8080/v1/acme/inbox/tasks

open http://127.0.0.1:8080/acme/inbox
curl -s http://127.0.0.1:8080/acme/inbox.md
```

`make test` / `make lint`. Default listen is localhost. HTML is read-only.
Sign in at `/login` with a bearer token. Optional SMTP enables a magic link
for tokens that have an email bound at mint time. `LEDGER_OPERATOR_TOKEN`
enables `/admin` and `POST /v1/owners`. GitHub OAuth is optional.
`LEDGER_HTML_AUTH=1` locks the live view. The API still needs a bearer token.

See [`docs/auth.md`](docs/auth.md). Production: [`docs/deploy.md`](docs/deploy.md).
Meter and freeze: [`docs/provisioning.md`](docs/provisioning.md).

## Hosted

Markedo AS runs [task-ledger.com](https://task-ledger.com). The first ledger is
free. Extra ledgers are paid. Sign up at
[www.task-ledger.com/signup](https://www.task-ledger.com/signup).
Self-host if you want the list on a machine you operate. Same binary.

## Agent skill

```bash
npx skills add markedo-org/ledger -s task-ledger
```

Canonical files: `.agents/skills/task-ledger/`. Set `LEDGER_URL` and
`LEDGER_TOKEN` in the consumer. Cursor `mcp.json` needs those values written
out. It does not interpolate environment variables.

## MCP

Streamable HTTP at `/mcp`, same bearer token. Stateless. Official Go SDK,
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

Paste that into Cursor (`.cursor/mcp.json` or Settings → MCP) or Claude Code
(project `.mcp.json`). ChatGPT and Claude on the web usually want OAuth. That
is not in this binary yet.

## API

All mutating routes need `Authorization: Bearer`. Actor is the token, not a
field on the body. Create requires `idempotency_key` (JSON or header
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

## Licence

MIT. See [SECURITY.md](SECURITY.md) to report a vulnerability.
