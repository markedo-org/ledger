# Agents

You are working in **task-ledger** (`ledger`): a Go server that is a shared task
ledger for many agents, with a human-readable live view. Module path:
`github.com/markedo-org/ledger`.

This file is the router. Read linked files when the task needs them.

## First steps

1. Read `docs/design.md` (the v1 cut). Do not reopen it unless the cut is wrong.
2. `make build` / `make test` (Go 1.24+).
3. Secrets stay out of git (`config.yaml`, `.env`, `*.db`).

## Where things live

| Need | Look here |
| --- | --- |
| Settled design | `docs/design.md` |
| MCP | `docs/mcp.md` |
| Agent skill (conventions) | `.agents/skills/task-ledger/` |
| Server entry | `cmd/ledger/` |

## Rules that always apply

1. **The HTTP API is the system.** HTML, the skill, MCP, and a future CLI are
   clients of the domain layer. Do not put business logic in transport handlers.
2. **No whole-object writes.** Intent-shaped operations only.
3. **MCP is Streamable HTTP at `/mcp`.** Same bearer token as the API. Not stdio,
   never a facade over a CLI.
4. **Markdown is a snapshot, never a write path.** Never parse `TASKS.md` back in.
5. **Do not invent product facts.** Behaviour comes from `docs/design.md` and the
   code.

## Useful commands

```bash
make build
make test
make lint
LEDGER_BOOT_TOKEN=lgr_dev ./ledger -listen 127.0.0.1:8080 -db ledger.sqlite
./ledger -version
```
