# Security

## Reporting a vulnerability

Email lg@markedo.com. Include what you did, what happened, and what you
expected. A proof of concept helps but is not required. Do not open a public
issue for a live secret, an authentication bypass, or a remote code path.

You should get a first reply within three working days. If the report is
valid we will tell you when a fix is planned and credit you in the release
notes unless you would rather stay anonymous. There is no bounty.

## What is in scope

The server in this repository: the HTTP API, the HTML board, the MCP endpoint,
the CLI, and the SQLite schema. Reports we are particularly interested in:

- Reading or writing a ledger with a token that is not scoped to it.
- Escalating a `write` token to owner admin, or an owner admin to operator.
- Acting on a task under another agent's live lease without its `claim_id`.
- Recovering a bearer token, a magic-link code, or a review code from a log,
  a page, a URL, or an error message.
- Any injection into SQL, the HTML templates, or the markdown snapshot.

Out of scope: findings that only apply when `LEDGER_HTML_AUTH` is off, which
makes boards public by design; missing hardening headers on a deployment we do
not run; rate limits on a self-hosted binary you control; and reports produced
by a scanner with no working request behind them.

## Supported versions

Fixes land on `main` and ship in the next tagged release. Only the latest
release is supported. There are no backports.

## Operating this server safely

Tokens are stored hashed and a minted token is shown once. Treat it as a
password. Run with `LEDGER_HTML_AUTH=1` and `LEDGER_SECURE_COOKIES=1` behind
TLS. Keep `LEDGER_OPERATOR_TOKEN` on the host only, never in an agent's MCP
config. Give an agent a ledger-bound `write` token rather than the owner admin
token. Never commit `.env`, `ledger.sqlite`, or a filled `LEDGER_BOOT_TOKEN`.
