# CLI

The `ledger` binary is the server and the provision tool. Agents still use
MCP. This CLI is for humans: first owner, extra ledgers, tokens, MCP
snippets.

## Serve

No subcommand, or `ledger serve`, starts the HTTP/MCP process. Flags are
unchanged: `-listen`, `-db`, `-boot-owner`, `-boot-ledger`, `-boot-actor`,
`-version`. Existing systemd units keep working.

`ledger init` is the only command that writes SQLite itself. Everything
else talks HTTP to a running server.

## Config

File: `~/.ledger/config` (Windows: `%USERPROFILE%\.ledger\config`). Override
with `LEDGER_CONFIG`. Mode `0600`. AWS-style profiles.

```
[default]
url = http://127.0.0.1:8080
token = lgr_…
owner = acme
ledger = inbox

[hosted]
url = https://task-ledger.com
token = lgr_…
owner = acme
```

```bash
ledger config path
ledger config show [--profile name]
ledger config set [--profile name] url|token|owner|ledger <value>
```

`--profile` or `LEDGER_PROFILE` selects a profile. `LEDGER_URL` and
`LEDGER_TOKEN` override the file (useful in CI). Do not commit the file.

## Commands

| Command | What |
| --- | --- |
| `ledger init --owner --ledger --actor` | Empty DB: boot those names, write config, print token and MCP |
| `ledger serve` | Run the server |
| `ledger mcp print` | Print Cursor/Claude MCP JSON from the profile |
| `ledger mcp print --write-cursor` | Merge into `./.cursor/mcp.json` |
| `ledger skill` | Print `npx skills add markedo-org/ledger -s task-ledger` (does not run it) |
| `ledger owner create` | Operator. Optional `--ledger` and `--actor` mint owner admin |
| `ledger owner list` | Operator |
| `ledger owner set-max --max-ledgers N` | Operator. `0` is unlimited |
| `ledger ledger create --slug` | Owner admin. Mints a bound write token and returns `mcp` |
| `ledger ledger list` | Owner admin or operator |
| `ledger token mint --actor [--ledger] [--role write]` | Owner admin |

`init --write-cursor` also merges the admin MCP server into
`./.cursor/mcp.json`.
