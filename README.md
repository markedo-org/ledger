# task-ledger

When several agents share a project file, the last save wins. task-ledger is
one live list on a server. Claim a task for a limited time. Everyone else can
see that, and they wait.

One Go binary, SQLite on disk. HTTP API, a read-only HTML board, Streamable
HTTP MCP, and a provision CLI. No user table: a minted bearer token is the
identity.

Three ways to run it:

- **Local.** Your laptop, many agents, `127.0.0.1`. Start here:
  [docs/getting-started.md](docs/getting-started.md).
- **Self-host.** The same binary on a machine you operate. systemd and
  nginx: [docs/deploy.md](docs/deploy.md).
- **Hosted.** We run the box.
  [www.task-ledger.com](https://www.task-ledger.com). First ledger free.

Go module: `github.com/markedo-org/ledger`. Binary name: `ledger`.

## Quick start (local)

Go 1.24+. macOS, Linux, or Windows.

```bash
go install github.com/markedo-org/ledger/cmd/ledger@latest
ledger init --owner acme --ledger inbox --actor ada
ledger serve
```

Binaries: [Releases](https://github.com/markedo-org/ledger/releases).
`init` creates the database with *your* names, writes `~/.ledger/config`,
and prints the owner admin token once. If the current directory has
`.cursor`, it also merges MCP there. Open
`http://127.0.0.1:8080/acme/inbox`. `ledger mcp print` reprints the agent
snippet. Full walkthrough: [docs/getting-started.md](docs/getting-started.md).
CLI reference: [docs/cli.md](docs/cli.md).

`make test` / `make lint` / `make smoke` from a clone. `ledger` with no subcommand still
serves (same flags as before: `-listen`, `-db`).

## MCP

Streamable HTTP at `/mcp`, same bearer token. Stateless. See
[`docs/mcp.md`](docs/mcp.md).

A project-only agent gets one server, named for that ledger, with a
ledger-bound write token. An agent that creates ledgers or mints tokens gets
a second server named for admin, with the owner admin token. Both in one
file. `ledger mcp print` emits that JSON from your profile.

Then install the agent skill. It teaches the loop (claim, note, close). It
does not choose a host.

```bash
npx skills add markedo-org/ledger -s task-ledger
```

Canonical files: `.agents/skills/task-ledger/`.

## Hosted

Sign up at [www.task-ledger.com/signup](https://www.task-ledger.com/signup).
The first ledger is free. Extra ledgers are paid. The live board is on the
apex, [task-ledger.com](https://task-ledger.com). Point `ledger config` at
that origin and the token you were shown. The CLI commands are the same.

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
| POST | `/v1/:owner/ledgers` | Create ledger (owner admin or operator; enforces `max_ledgers`) |
| PATCH | `/v1/:owner/ledgers/:ledger` | Title and DONE retention (owner admin) |
| POST | `/v1/:owner/:ledger/reset` | Wipe tasks and restart at T-001 (owner admin). `confirm` must be `owner/ledger`. |
| GET | `/v1/:owner/tokens` | List tokens (owner admin; id and metadata only, no secret). |
| POST | `/v1/:owner/tokens` | Mint a bearer token (owner admin; operator may mint role write only). Plaintext and id returned once. Optional `email`. |
| DELETE | `/v1/:owner/tokens/:id` | Revoke token (owner admin; irreversible). |
| POST | `/v1/review` | Mint a one-time browser review URL (write token; operator refused) |
| POST | `/v1/:owner/:ledger/tasks` | Create |
| GET | `/v1/:owner/:ledger/tasks` | List. Query `done=1` or `done=true` for DONE only; `tag=` filters by tag slug |
| GET | `/v1/:owner/:ledger/tasks/:handle` | Get `T-001` |
| POST | `/v1/:owner/:ledger/tasks/:handle/claim` | Claim |
| POST | `/v1/:owner/:ledger/tasks/:handle/heartbeat` | Extend lease |
| POST | `/v1/:owner/:ledger/tasks/:handle/release` | Drop claim |
| POST | `/v1/:owner/:ledger/tasks/:handle/phase` | Move phase |
| POST | `/v1/:owner/:ledger/tasks/:handle/notes` | Append note |
| POST | `/v1/:owner/:ledger/tasks/:handle/checks` | Tick or untick a check, or several with `ns` |
| POST | `/v1/:owner/:ledger/tasks/:handle/tags` | Replace tags |
| POST | `/v1/:owner/:ledger/tasks/:handle/close` | Close (`evidence` required) |
| POST | `/v1/:owner/:ledger/tasks/:handle/verify` | Refresh verified date, recording your actor as the verifier |
| POST | `/v1/:owner/:ledger/next` | Claim the next eligible NOW task |

Handles are `T-001` (one-letter series, default `T`). Default lease is 30
minutes.

## Layout

| Path | What |
| --- | --- |
| `cmd/ledger/` | Server and CLI binary |
| `internal/cli/` | `init`, config, mcp print, owner/ledger/token |
| `internal/app/` | Domain operations |
| `internal/store/` | SQLite |
| `internal/web/` | HTTP API, HTML |
| `internal/mcpserver/` | Streamable HTTP MCP |
| `docs/getting-started.md` | Local tutorial |
| `docs/cli.md` | CLI reference |
| `docs/versioning.md` | 0.x tags and what 1.0 means |
| `docs/deploy.md` | Self-host and production |
| `.agents/skills/task-ledger/` | Agent skill |

## License

MIT. See [LICENSE](LICENSE). Report a vulnerability in
[SECURITY.md](SECURITY.md).
