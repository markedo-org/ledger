# Changelog

## 0.15.11

Tag badges sit vertically centered against the two-line title and meta
stack. The handle stays aligned with the title.

## 0.15.10

Tag chips and the board tag filter are outlined badges: 1px mute border,
full rounding, no fill colour.

## 0.15.9

Optional tags on tasks: at most three lowercase slugs. Set on create,
replace with `set_tags`. `list_tasks` and the HTML board filter by one
tag. Chips sit on the right of each row. A tag is a filter, not a ledger.

## 0.15.8

Ledger settings is its own form: sentence-case labels, grouped fields,
day inputs with a unit, and a darker field well so the page reads in
dark theme. Login `form.signin` is unchanged.

## 0.15.7

`review_url` (MCP) and `POST /v1/review` mint a one-time, 15-minute code.
`GET /login/review?code=` sets the session cookie and redirects to the
board. The bearer token never goes in the query string. Operator secret
cannot mint. Skill: open that URL for the human, do not paste the token.

## 0.15.6

Owner page: capacity as in-use / cap / open, billing only when
`LEDGER_SITE_URL` is set, and Settings to edit a ledger title (and
retention). Same binary for self-host; omit the env var and there is no
billing link. `PATCH /v1/:owner/ledgers/:ledger` accepts `title`.

## 0.15.5

Claim returns a `claim_id`. The same actor in another session cannot
refresh, heartbeat, release, close, or phase that live lease without it.
`get_task`, `list_tasks`, and HTML omit it. Steal from another actor
mints a new id. Leases minted before this release keep the old
same-actor behaviour until they expire.

## 0.15.4

Owner page prints a ledger slug once when the title matches the slug, so a
long name no longer reads as `abusemanagerabusemanager`. Optional
`LEDGER_SITE_URL` adds a billing link for unused capacity.

## 0.15.3

Creating an owner with a first ledger mints two tokens: owner admin, and a
ledger-bound write token for that ledger. HTTP and MCP return `write_token`
and `write_mcp` beside the admin `token` and `mcp`.

## 0.15.2

Owner and ledger slugs may start with a digit (`2027`, `1acme`). They still
must be lowercase letters, digits, or hyphens, and must not start with a
hyphen. Actor names still start with a letter.

## 0.15.1

Admin provision page: each action is a framed block with a display title
and a short explanation. Labels stay short. The type and palette are
unchanged.

## 0.15.0

DONE tasks stay stored. The default list and HTML board hide DONE older than
`archive_done_after_days` (7). `0` means never hide. HTML Archive and
`list_tasks` `done=true` show every DONE task, and only those. `get_task`
still works. `purge_done_after_days` (0) may delete on reap; there is no
MCP purge tool. Process defaults:
`LEDGER_ARCHIVE_DONE_AFTER_DAYS`, `LEDGER_PURGE_DONE_AFTER_DAYS`. Per-ledger
override: `ledger ledger set` / `PATCH /v1/:owner/ledgers/:ledger`.

## 0.14.2

`init` and `mcp print` write `.cursor/mcp.json` when the working directory
already has a `.cursor` folder, or when `--project-dir` points at a repo.
`--write-cursor` still forces a write. `--no-write-cursor` skips it. GitHub
Release binaries build on `v*` tags. `make smoke` is the v1.0 local-loop
protocol.

## 0.14.1

`ledger skill` prints the `npx skills add` command. `init` prints it too.
Neither runs it. [docs/versioning.md](docs/versioning.md) is the 0.x / 1.0
rule.

## 0.14.0

The binary is also a provision CLI. `ledger init` boots an empty database
with your owner, ledger, and actor, writes `~/.ledger/config`, and prints
MCP JSON. `ledger mcp print`, `owner`, `ledger`, and `token` talk HTTP to a
running server (local, self-host, or hosted). Bare `ledger` / `ledger serve`
is unchanged. Getting started covers macOS, Linux, and Windows.

## 0.13.1

Owner-scoped tokens default MCP tools to the owner's only ledger. Create no
longer stamps `verified_at`. Skill is MCP-first and does not name a host;
example tokens removed. Signup and create-ledger copy tell the admin-versus
project token split. Creating a ledger as owner admin mints a bound write
token and returns a project MCP config. Restarting an already-initialised database no longer
requires the default boot owner (`acme`) to exist. Deploy resets a
crash-looped systemd unit before restart.

## 0.13.0

README and deploy docs are written for a public self-host first. Markedo
hosted remains a second path, not the only one.

Optional magic-link email. Bind an address when minting a token. SMTP off
until host, user, and password are set. The link wraps that token into the
same session as paste-to-login. No user table. The API token is not mailed.
Login is a focused sign-in: GitHub first when configured, token as the
project door, no architecture lecture. After sign-in, hosted no longer
sends you to the brochure: GitHub allowlist lands on `/admin`, a token on
its owner or ledger. `/admin` no longer redirect-loops when a signed-in
host human opens it.

## 0.12.0

Operator provisioning on the same process: `LEDGER_OPERATOR_TOKEN` creates
owners, sets `max_ledgers`, and mints the first admin token. Newest ledgers
over the cap become read-only (oldest stay writable; `0` is unlimited). HTML
`/admin` is a form over those routes. Stripe stays out of this binary.

## 0.11.0

HTML live view uses the brochure type (Archivo Narrow, IBM Plex Sans, IBM Plex
Mono) and a cooler ink/paper. Claimed rows use teal. The masthead carries the
badge. Hosted deploy: `scripts/deploy.sh` (linux/amd64 to VPS Services,
systemd, nginx, certbot). Listen on `127.0.0.1:8787`. Brochure deploy lives in
the Markedo meta-repo.

## 0.10.0

`GET /` is configurable: `login` (default, 302 `/login`), `url` (301 to
`LEDGER_ROOT_URL`), or `file` (serve `LEDGER_ROOT_FILE`). Hosted Markedo uses
url → `https://www.task-ledger.com`. Apex stays the app; www is marketing.

## 0.9.0

Create requires `idempotency_key` (HTTP and MCP). MCP `set_phase` accepts
`force`, matching HTTP, so agents can override the fourth-deferral block.

## 0.8.0

HTML sessions from a bearer token are bound to that token's owner, and to its
ledger when the token is ledger-bound. GitHub allowlist sessions stay operator
wide. Token login requires a CSRF field. Logout is POST.

## 0.7.0

HTML live view: the whole board row is a link. Row meta shows notes, checks,
evidence, and claims. Task page uses a real check list and keeps notes under
their own heading. Appearance follows the OS, with a small control to flip.

## 0.6.1

Installable agent skill at `.agents/skills/task-ledger/` (`npx skills add
markedo-org/ledger -s task-ledger`). MCP-first, env-driven. Duplicate
`.claude/skills` copy removed.

## 0.6.0

Token login is the HTML baseline (`POST /login`). `LEDGER_HTML_AUTH=1` locks the
live view without GitHub. GitHub remains an optional extra. No user table.
Magic-link email is documented as later (SMTP, off by default, wraps a token).

## 0.5.0

GitHub OAuth for the HTML live view: session cookie, `/login`, `/logout`,
`/auth/github/callback`, allowlist. Off until env vars are set. Owner index at
`/{owner}`. Deploy unit file is in `contrib/` and is not applied.

## 0.4.0

Admin API to create ledgers and mint tokens after bootstrap. `max_ledgers` is
enforced. A token with no ledger binding can act across the owner's ledgers.
The boot token may be bound to one ledger and still provision, if it is admin.

## 0.3.0

Tick and untick checks (`POST .../checks`, MCP `set_check`) so close is not a
trap. MCP `verify_task`. Streamable HTTP returns JSON (not SSE-framed POST
bodies) and empty `list_tasks` returns `[]`. MCP is mounted on the stdlib mux,
not wrapped by Gin.

## 0.2.0

Streamable HTTP MCP at `/mcp` (official Go SDK v1.2.0, stateless, bearer auth).
Hosted app will run on VPS Services; marketing site hosting still open.

## 0.1.0

First vertical slice: SQLite store, intent-shaped HTTP API, read-only HTML live
view, markdown snapshot, boot token, claim leases with a reaper.

## 0.0.0

Scaffold: `ledger` binary with `GET /health`, MIT, design cut, agent skill stub.
