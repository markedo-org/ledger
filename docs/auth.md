# Authentication (HTML)

API and MCP stay on `Authorization: Bearer`. There is no user table, password,
or team directory. A minted bearer token *is* the identity (actor, owner,
optional ledger, write/admin). Do not grow a parallel account system.

## Token login (baseline)

`GET /login` is a form. Paste a bearer token. That sets the same HttpOnly
session cookie GitHub would. Works for self-host, hosted, and local dogfood.

Lock the live view without GitHub:

```
LEDGER_HTML_AUTH=1
```

Until that is set (and GitHub OAuth is off), HTML stays open. Token login still
works; it is just not required.

## GitHub OAuth (optional extra)

The **operator of the server** registers one GitHub OAuth App. People who sign in
just use their existing GitHub account. Not required for HTML if token login is
enough.

- **Markedo hosted:** we register one app against the task-ledger domain.
  The allowlist is host humans (Markedo), not customers. A GitHub allowlist
  sign-in creates an operator session: it can see `/owners` and `/admin`, not a
  tenant's task board. It is not mapped to an owner or a GitHub org.
  Customers sign in with a token we minted (and later a magic link). If we
  later want customer GitHub, bind a login to a token the same way we bind
  email. Do not grow a user table.
- **Self-host:** skip GitHub entirely, or register their own app for *their*
  callback URL. They cannot reuse Markedo's app.

The hosted Markedo app is registered on `markedo-org`. Callback:
`https://task-ledger.com/auth/github/callback`. Allowlist is a comma-separated
list of GitHub logins (for example `octocat`). Client ID and secret stay in
env, never in git.

### GitHub OAuth App (operator)

1. GitHub → Settings → Developer settings → OAuth Apps → New.
2. Homepage URL: the public site of this instance.
3. Authorization callback URL: `https://<domain>/auth/github/callback`.
4. Uncheck "Enable Device Flow". Scope used: `read:user`.
5. Copy Client ID and Client Secret into the server env. Never commit them.

## Environment

```
LEDGER_HTML_AUTH=0
LEDGER_OPERATOR_TOKEN=
LEDGER_GITHUB_CLIENT_ID=
LEDGER_GITHUB_CLIENT_SECRET=
LEDGER_GITHUB_CALLBACK_URL=https://<domain>/auth/github/callback
LEDGER_GITHUB_ALLOWLIST=octocat
LEDGER_SECURE_COOKIES=1
LEDGER_PUBLIC_URL=https://<domain>
LEDGER_SMTP_HOST=
LEDGER_SMTP_PORT=465
LEDGER_SMTP_USER=
LEDGER_SMTP_PASS=
LEDGER_SMTP_FROM=
```

`LEDGER_GITHUB_ALLOWLIST` is a comma-separated list of GitHub logins. If OAuth is
on and the list is empty, GitHub sign-in is denied (fail closed). Token login
still works.

Set `LEDGER_SECURE_COOKIES=1` behind HTTPS.

## Routes

| Path | What |
| --- | --- |
| `GET /owners` | Signed-in home: owners this session may see. Operator and GitHub allowlist see every owner. |
| `GET /:owner` | Ledgers under that owner. Billing link only if `LEDGER_SITE_URL` is set. Owner sessions only: a host session sees an owner's ledgers on `/admin` instead. |
| `GET /:owner/:ledger/settings` | Edit title and DONE retention. Owner-admin session only. |
| `GET /:owner/:ledger` | Live task view. |
| `GET /login` | Token form. Email form if SMTP is set. GitHub link if OAuth is configured. Sets a CSRF cookie. |
| `POST /login` | Exchange a bearer token for a session cookie. Requires the CSRF field. |
| `POST /login/email` | Request a magic link. Requires CSRF. Same copy whether the address is bound or not. |
| `GET /login/email?code=` | Consume a one-time link. Sets the session cookie. |
| `GET /login/review?code=` | Consume a one-time review code. Sets a session cookie with the minting token's role and redirects to the board. |
| `POST /v1/review` | Mint a review URL for the bearer token. Write tokens may. Operator secret may not. |
| `GET /login/github` | Starts GitHub authorize |
| `GET /auth/github/callback` | GitHub code → session |
| `POST /logout` | Drops the session. GET is not accepted. |

Session cookie is HttpOnly, SameSite=Lax, 7 days. Stored hashed, like API tokens.
A token login session is bound to that token's owner, and to its ledger when the
token is ledger-bound. After sign-in, that session lands on `/{owner}` or
`/{owner}/{ledger}`. GitHub allowlist sessions are operator sessions (host-wide,
no owner binding): they may use `/owners` and `/admin`, and land on `/admin`.
They cannot open a tenant board. A project token lands on `/{owner}` or
`/{owner}/{ledger}`. An operator token also lands on `/admin`. Anonymous `GET /` on
hosted still 301s to the brochure.

## Operator token

Host-level, set `LEDGER_OPERATOR_TOKEN`. Distinct from the first owner's boot
token and from owner `admin`. Admin is authority inside one owner; the operator
is not inside any owner. A host must stand tenants up and meter them without
being able to read or delete their work.

It can: create owners (minting that owner's one admin token once, optionally
email-bound), list and read owners, set `max_ledgers`, create ledgers for any
owner, mint write tokens for any owner, and reach HTML `/admin`. GitHub
allowlist sign-in creates the same operator session for HTML.

It cannot: read or write tasks, reset a ledger, change ledger settings, list or
revoke tokens, mint admin for an owner that already exists, or open a tenant
board in the HTML UI. If an owner's admin token is lost, recovery is magic-link
sign-in to the email bound when the owner was created, not a host-level override.

See [`provisioning.md`](provisioning.md). Do not let an owner's admin token
raise its own cap.

```
LEDGER_OPERATOR_TOKEN=
```

## Magic-link email (optional)

Off until SMTP is fully set (`LEDGER_SMTP_HOST`, `LEDGER_SMTP_USER`,
`LEDGER_SMTP_PASS`, and `LEDGER_SMTP_FROM`; `LEDGER_SMTP_FROM` defaults to
`LEDGER_SMTP_USER`). There is still no user table. Bind an email when
you mint a token (`email` on `POST /v1/:owner/tokens`, the admin form, or MCP
`create_token`). One address per token. The link wraps that token into the
same session cookie as paste-to-login. The API token never goes in the mail.

- TTL 15 minutes, single use. A new request replaces any unused link for that
  token.
- Same response whether the address is bound or not: "If that address has a
  token on this ledger, we sent a sign-in link."
- About one request per minute per address.
- Port 465 uses implicit TLS. Other ports use STARTTLS when the server offers
  it.
- `LEDGER_PUBLIC_URL` is the base of the link (default `http://127.0.0.1:8080`).
- The operator env token has no email. Paste it at `/login`.
- `LEDGER_SMTP_FROM` may carry a display name, `Task Ledger <hello@example.com>`.
  The envelope sender is the bare address inside it. A `LEDGER_SMTP_FROM` with
  no address at all leaves mail off rather than failing at every send, which is
  why `LEDGER_SMTP_FROM` defaulting to a `LEDGER_SMTP_USER` like `resend` does
  not count as configured.

Use a sending domain you control, with SPF and DKIM set up at your mail
provider, and an address on that domain. A personal mailbox as the sender lands
in spam and puts that mailbox's password on the server.

The email form is hidden when SMTP is off.
