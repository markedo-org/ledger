# Getting started

This is the local path: one binary on your machine, a SQLite file, agents
talking to `127.0.0.1`. No nginx. No operator token. You pick the owner,
ledger, and actor names.

Self-host (a VPS, systemd, TLS) is [deploy.md](deploy.md) after this works.
Hosted (we run the box) is [www.task-ledger.com/signup](https://www.task-ledger.com/signup).

## Install

Go 1.24+. Until GitHub Releases exist, build or `go install`.

macOS and Linux:

```bash
go install github.com/markedo-org/ledger/cmd/ledger@latest
```

Make sure `$(go env GOPATH)/bin` is on your `PATH`. Or clone and build:

```bash
git clone https://github.com/markedo-org/ledger.git
cd ledger
go build -o ledger ./cmd/ledger
```

Windows (PowerShell):

```powershell
go install github.com/markedo-org/ledger/cmd/ledger@latest
```

Or:

```powershell
git clone https://github.com/markedo-org/ledger.git
cd ledger
go build -o ledger.exe ./cmd/ledger
```

`ledger version` should print a version line.

## Init

Pick names you will keep. The owner is usually you or the org. The ledger is
usually the repo or project. The actor is the name claims will show.

```bash
ledger init --owner acme --ledger inbox --actor ada
```

That creates `ledger.sqlite` in the current directory, writes
`~/.ledger/config` (mode `0600`), prints the owner admin token once, and
prints an MCP snippet. On Windows the config is `%USERPROFILE%\.ledger\config`.

A second `init` on the same database refuses. Use `ledger serve`.

## Serve

```bash
ledger serve
```

Same as running `ledger` with no subcommand, which is what a systemd unit
already does. Default listen is `127.0.0.1:8080`. Open
`http://127.0.0.1:8080/acme/inbox` (your owner and ledger). Sign in at
`/login` with the token `init` printed.

## Attach an agent

`ledger mcp print` reprints the snippet from `~/.ledger/config`. Paste it
into Cursor (`.cursor/mcp.json` or Settings → MCP) or Claude Code (project
`.mcp.json`). Write the values out. Cursor does not interpolate environment
variables.

```bash
ledger mcp print --write-cursor
```

merges into `./.cursor/mcp.json` and leaves other servers alone.

Then:

```bash
npx skills add markedo-org/ledger -s task-ledger
```

The skill does not choose a host. The MCP config does.

The token from `init` is owner admin. It can create more ledgers and mint
tokens. It already works on your first board. For an agent that should only
see that project:

```bash
ledger token mint --actor bot --ledger inbox --role write
```

Put that token in a second MCP server named for the project. See
[cli.md](cli.md).

## Create a task

With the server up and the token in the environment for curl:

```bash
export LEDGER_TOKEN=…   # the token init printed
curl -s -H "Authorization: Bearer $LEDGER_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"title":"First task","idempotency_key":"demo-1"}' \
  http://127.0.0.1:8080/v1/acme/inbox/tasks
```

Or ask the attached agent to create one. Prefer `next_task` once there is
work on the board.

## More owners and ledgers

Locally you already have the first owner. Extra ledgers:

```bash
ledger ledger create --slug jobs
```

That mints a project write token and prints an MCP object. Extra owners on
a self-hosted box need the operator token (`LEDGER_OPERATOR_TOKEN` on the
server, a profile on the CLI):

```bash
ledger config set --profile op url http://127.0.0.1:8080
ledger config set --profile op token "$LEDGER_OPERATOR_TOKEN"
ledger owner create --profile op --slug other --ledger inbox --actor ada
```

## Next: a server you operate

Same binary. `ledger init` on the box, or `ledger owner create` against it
with the operator token. Then [deploy.md](deploy.md) for systemd, nginx,
TLS, and backups.

## Next: hosted

If you do not want to run the binary, sign up at
[www.task-ledger.com/signup](https://www.task-ledger.com/signup). Point
`ledger config` at `https://task-ledger.com` and the token you were shown.
The CLI commands are the same.
