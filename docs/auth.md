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
  The allowlist is host humans (Markedo), not customers. A GitHub session
  can see every live view. It is not mapped to an owner or a GitHub org.
  Customers sign in with a token we minted (and later a magic link). If we
  later want customer GitHub, bind a login to a token the same way we bind
  email. Do not grow a user table.
- **Self-host:** skip GitHub entirely, or register their own app for *their*
  callback URL. They cannot reuse Markedo's app.

The hosted Markedo app is registered on `markedo-org`. Callback:
`https://task-ledger.com/auth/github/callback`. Allowlist starts at
`lgforsberg`. Client ID and secret stay in env, never in git.

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
LEDGER_GITHUB_ALLOWLIST=you
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
| `GET /owners` | Signed-in home: owners this session may see. GitHub allowlist sees all. |
| `GET /:owner` | Ledgers under that owner. |
| `GET /:owner/:ledger` | Live task view. |
| `GET /login` | Token form. Email form if SMTP is set. GitHub link if OAuth is configured. Sets a CSRF cookie. |
| `POST /login` | Exchange a bearer token for a session cookie. Requires the CSRF field. |
| `POST /login/email` | Request a magic link. Requires CSRF. Same copy whether the address is bound or not. |
| `GET /login/email?code=` | Consume a one-time link. Sets the session cookie. |
| `GET /login/github` | Starts GitHub authorize |
| `GET /auth/github/callback` | GitHub code → session |
| `POST /logout` | Drops the session. GET is not accepted. |

Session cookie is HttpOnly, SameSite=Lax, 7 days. Stored hashed, like API tokens.
A token login session is bound to that token's owner, and to its ledger when the
token is ledger-bound. After sign-in, that session lands on `/{owner}` or
`/{owner}/{ledger}`. GitHub allowlist sessions are host-wide (no owner binding) and may use
`/admin`. They land on `/admin`. A project token lands on `/{owner}` or
`/{owner}/{ledger}`. An operator token also lands on `/admin`. Anonymous `GET /` on
hosted still 301s to the brochure.

## Operator token

Host-level, set `LEDGER_OPERATOR_TOKEN`. Distinct from the first owner's boot
token and from owner `admin`. Creates owners, sets `max_ledgers`, and is what
a website (ours or a self-hoster's) uses to provision. HTML: sign in at
`/login` with that token, then `/admin`. See
[`provisioning.md`](provisioning.md). Do not let an owner's admin token
raise its own cap.

```
LEDGER_OPERATOR_TOKEN=
```

## Magic-link email (optional)

Off until SMTP is fully set (`LEDGER_SMTP_HOST`, `USER`, `PASS`, and `FROM`;
`FROM` defaults to `USER`). There is still no user table. Bind an email when
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
- Do not put a personal mailbox on the hosted service. Hosted SMTP is a later
  choice.

The email form is hidden when SMTP is off.
