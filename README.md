# task-ledger

A task ledger for many agents. Markedo AS. MIT.

Agents share one live list without overwriting each other. Humans get a page that
reads like `TASKS.md`. The live system is the write path. A markdown snapshot is
optional and read-only.

- Binary and slug: `ledger`
- Module: `github.com/markedo-org/ledger`
- Hosted service (planned): task-ledger
- Design: [`docs/design.md`](docs/design.md)

This repository is a scaffold. The v1 server (API, HTML live view, markdown
snapshot, agent skill, then MCP) is what comes next.

## Quick start

```bash
make build
./ledger -listen 127.0.0.1:8080
curl -s http://127.0.0.1:8080/health
```

Go 1.24+. `make test` / `make lint` as usual.

## Layout

| Path | What |
| --- | --- |
| `cmd/ledger/` | Server binary |
| `docs/design.md` | Settled v1 cut |
| `.agents/skills/task-ledger/` | Agent skill (conventions, not operations) |

## Licence

MIT. Self-host is first-class. Markedo will also run a hosted instance and use it.
