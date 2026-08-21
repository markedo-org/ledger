# Deploy

Self-host is the production path: one binary, one SQLite file, nginx in
front. For a laptop and a first owner, start with
[getting-started.md](getting-started.md) and the CLI in [cli.md](cli.md).

## Self-host

1. `make linux-amd64` (or `make build` on the box), or download a release
   binary from [GitHub Releases](https://github.com/markedo-org/ledger/releases).
2. Copy the binary to something like `/opt/ledger/ledger`.
3. Copy [`contrib/ledger.service`](../contrib/ledger.service) to systemd.
   Adjust the listen address if `8080` is free on your box. Markedo's unit
   uses `127.0.0.1:8787` because `8080` is taken there.
4. Env file next to the binary (mode `0640`). Start from [`.env.example`](../.env.example).

Required in production:

```
LEDGER_BOOT_TOKEN=<strong, not lgr_dev>
LEDGER_OPERATOR_TOKEN=<strong>
LEDGER_HTML_AUTH=1
LEDGER_SECURE_COOKIES=1
LEDGER_PUBLIC_URL=https://ledger.example
```

`LEDGER_OPERATOR_TOKEN` is what `/admin` and `POST /v1/owners` use. An owner's
`admin` token cannot raise `max_ledgers`. `0` on `max_ledgers` means unlimited
(self-host). Hosted Markedo never sends `0`.

5. Reverse proxy with TLS. [`contrib/nginx.example.conf`](../contrib/nginx.example.conf)
   is a starting vhost. Point `server_name` at your host.
6. Back up `ledger.sqlite` (and the `-wal` / `-shm` files if the process is
   running). Restore by stopping the unit, replacing the files, starting again.

Optional: SMTP for magic-link sign-in (`docs/auth.md`). GitHub OAuth for host
humans. `LEDGER_ROOT=url` plus `LEDGER_ROOT_URL` if the apex should send
anonymous `GET /` to a brochure.

DONE retention defaults (`LEDGER_ARCHIVE_DONE_AFTER_DAYS=7`,
`LEDGER_PURGE_DONE_AFTER_DAYS=0`). `0` means never. Hosted should leave purge
at `0`. A ledger can override with `ledger ledger set`.

## First boot

A fresh database boots `acme/inbox` with actor `ada` unless you pass
`-boot-owner`, `-boot-ledger`, `-boot-actor`.

Production usually skips `ledger init` and lets the first `ledger serve` create
the database. Start the unit once with the env file in place. If
`LEDGER_BOOT_TOKEN` is set, as recommended above, that value is the boot token
and there is nothing to collect. If it is not set, the server mints one and
writes it to `<dbpath>.boot-token`, mode `0600`, beside the SQLite file. The
journal line names the path rather than the token.

Point the CLI at it from your laptop, or over SSH from the box:

```bash
ledger config set url https://ledger.example
ledger config set token "$(cat /opt/ledger/ledger.sqlite.boot-token)"
ledger config set owner acme    # or your -boot-owner
ledger config set ledger inbox  # or your -boot-ledger
```

Keep the token somewhere safe and delete the file. From there, sign in at
`/login` or run `ledger mcp print` to configure MCP. Changing
`LEDGER_BOOT_TOKEN` afterwards does not rotate the stored token: the boot token
is only read on an empty database.

## Markedo hosted

Markedo AS runs the same binary at `https://task-ledger.com`. Brochure and
signup: `https://www.task-ledger.com`.

`scripts/deploy.sh` is how we ship that instance (VPS Services, systemd
`ledger`, listen `127.0.0.1:8787`). It is not the self-host path. Secrets stay
in the meta-repo `.secrets/` and on the box, never in this tree.

It runs `go test ./...` first and refuses a working tree with uncommitted
changes, because the binary is built from what is on disk and would otherwise
report a version that matches no commit. `--allow-dirty` ships anyway and
stamps the version `+dirty` so the running server says what it is.
`--skip-tests` is for an outage.
