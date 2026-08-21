# Changelog

## 0.24.0

`set_check` takes several boxes at once, and the MCP descriptions say what the
server actually enforces.

Ticking the last four boxes on a task cost four calls, each one reloading the
task and re-checking the lease, and an agent that stopped halfway left the task
in a state nobody had asked for. `set_check` now accepts `ns`, a list of
indexes, applied in one transaction as one version bump. Every index is
resolved before anything is written, so a wrong one ticks nothing. `n` still
works.

`claim_id` is enforced whenever a lease is live, but it sits at the end of the
tool descriptions as an aside, and an agent writing a call from the schema
skipped it and got an error it could not have predicted. The condition now
leads the description on `set_check`, `set_tags`, `set_phase`, and `close_task`.

`ledger config set` reads a value from stdin with `-` or from a file with
`@path`, so a bearer token no longer has to be typed as an argument, where the
shell keeps it in history and the process list shows it while it runs.

There is still no MCP tool for ledger settings, and `docs/mcp.md` now says why
rather than leaving the gap to be read as an oversight. Retention is the one
setting whose effect is invisible in the reply to the call that set it.

## 0.23.0

A task now records who closed it and who verified it.

Verify is the product's quality gate and it wrote nothing to the task about who
passed it, so an agent that closed its own work and verified it a second later
was indistinguishable from a second pair of eyes. The actor was in the event
log, which nothing surfaces. Tasks gain `closed_by` and `verified_by`, both
returned by the API and `get_task`, and the board shows them, saying plainly
when the verifier is the actor who closed it.

Self-verification stays allowed. A ledger with one agent on it would otherwise
never reach verified, and refusing it would only push people to swap actor
names. Making it visible is the honest fix.

`get_task` also returns `depends_on`, which the HTTP API has always returned.
Leaving it out meant an agent working through MCP could not see that a task was
blocked at all.

Both columns arrive by migration on existing databases, rehearsed by a test
that writes a task, removes the columns, and reopens the database the way a
deploy does.

## 0.22.1

Reverts the review link change in 0.22.0. A review session carries the role of
the token that minted it again, which is how it worked up to 0.21.1.

0.22.0 treated the link as something handed to an outside reviewer. It is
mainly how an agent gives its own human a way into their own board, and that
human is usually the owner. Read-only meant an owner whose agent is holding
their admin token had to go and find another credential, or wait for a magic
link by email, to change their own retention. That is friction bought with very
little, since the agent holding the token is already inside the same trust
boundary as the chat the link is pasted into.

Choose the power by choosing which token mints the link: write to look, admin
when the reader is the owner. `read` is no longer a role.

Kept from 0.22.0: the owner page decides the Settings link per ledger, so it is
not offered to sessions the handler will refuse, and it is not hidden from an
admin bound to a single ledger.

## 0.22.0

A review link now opens a read-only session, whatever token minted it.

It did what its description said, which was the problem: the browser inherited
the minting token's role, so a link made from an admin token let whoever opened
it rewrite retention, the setting that decides when finished work is deleted.
The link exists so an agent can show a human the board without handing over a
bearer token, and it is meant to be pasted into a chat, where it is logged,
forwarded and screen-shared. Reviewing is looking, so the word and the grant
now agree. Handing over real control means giving someone a token of their own.

Read is a session role only. Nothing mints a read token, and the API is
unchanged.

Also, the owner page decides the Settings link per ledger instead of asking the
owner-wide question once and reusing the answer. It used to offer Settings to
any session bound to that ledger, including write sessions the handler then
refused with a 403.

## 0.21.1

The sign-in rate limit counted attempts against an address the caller chooses.

Gin trusts every proxy unless told otherwise, so it read the leftmost
`X-Forwarded-For` entry, and nothing writes that entry but the client. Sending
a different value each time bought a fresh allowance each time, which left
bearer tokens, magic codes and review codes with nothing counting guesses
against them. Putting someone else's address there was worse: it spent their
allowance and locked them out of their own sign-in from an address they do not
control.

Loopback is now the only peer trusted by default, which is nginx and the binary
on one host. `LEDGER_TRUSTED_PROXIES` takes comma-separated CIDRs for a proxy
that runs elsewhere or a CDN in front. A request from anywhere else is counted
against the address the listener actually saw, so a directly exposed binary is
safe without configuration.

## 0.21.0

Breaking. The operator token is a provisioning credential now, not a master key.

`LEDGER_OPERATOR_TOKEN` is held by whoever runs the server. It used to satisfy
every ownership and admin check in the codebase, so running the host meant
being an admin of everyone on it: read any board, write into it, claim or break
anyone's lease, empty a ledger, revoke a tenant's tokens. That is a great deal
more than provisioning needs, and none of it was visible to the tenant.

Admin is authority inside one owner. The operator is not inside any owner. It
can still create owners, list and read them, set `max_ledgers`, create ledgers,
and mint `write` tokens for any owner, which is everything a host needs to sell
and meter the service. It can no longer read or write tasks, override a lease,
reset a ledger, change ledger settings, or list and revoke an owner's tokens.

It also cannot mint an `admin` token for an owner that already exists. Without
that last one the rest would be theatre: an operator able to mint itself admin
over any tenant still holds root, it just takes one more call. An owner is
issued its one admin token when the owner is created. If that token is lost,
the way back is magic-link sign-in to the email bound to it, not a host
override, so bind an email at signup.

The HTML side moved with it. A GitHub allowlist sign-in creates an operator
session, and the board renders through a public read gated only on the session,
so leaving that open would have handed the host every customer's tasks in a
browser while the API refused them. An operator session now reaches `/owners`
and `/admin` and stops there.

If you self-host alone, you are both the operator and the owner admin, and
nothing about your day changes except that destructive acts want the owner
token. If you host for others, they can now be told what you cannot see.

## 0.20.2

Documentation only. The docs described shipped work as future work, which left
a reader unable to tell what the product does today.

Operator provisioning, the `/admin` page and overflow freeze are written as
things that exist, because they do. `max_ledgers` is documented as the code
behaves: `0` means unlimited when you set it, `0` on create becomes 1, and a
negative value is refused. The old text said the floor was 1, so an operator
following it would never have found the unlimited setting.

Self-host gained the step it was missing. Starting `serve` on an empty database
without `LEDGER_BOOT_TOKEN` writes the token to a file beside the database, and
nothing walked the reader from that file to a working CLI or MCP. `docs/mcp.md`
no longer tells someone who installed a binary to start the server with a
`make` target they do not have.

The SMTP variables are named in full rather than as `HOST` and `PASS`, the
example GitHub allowlist is a placeholder rather than a maintainer's handle,
and the example systemd unit sets `UMask=0027`, without which the database is
created world readable on the host.

## 0.20.1

The deploy script now keeps the install directory to the service user, `0750`,
and sets `UMask=0027` on the unit. The database is customer data and was being
created world readable on the host.

Its two checks for an existing install ask through `sudo`, because a directory
closed that way answers "missing" to anyone else. Without that the script
refuses to deploy over a healthy install, and the bootstrap path would have
overwritten an existing `ledger.env`.

## 0.20.0

The app now sends its own security headers, including a content policy. They
used to come from the example nginx config, so anyone running the binary
without our proxy in front got none of them, and the content policy did not
exist anywhere.

The policy is strict: no inline script, no eval, nothing framed, nothing
loading from anywhere but this host and the font service. It is built at start
from the templates themselves, so the hash that allows the theme script follows
the script if it is edited. A hash written down as a constant would go stale
quietly, and the page would look right to whoever changed it while losing its
theme for everyone arriving fresh.

The example nginx configs no longer repeat the three headers the app now sends,
and carry `Strict-Transport-Security` instead, commented out in the generic one.
HSTS is the proxy's business: the app also serves plain http on localhost,
where claiming TLS would be wrong.

## 0.19.0

A claim is now conditional on the state it was decided from, so two agents can
no longer come away holding the same lease. Claiming read the task, judged it
free and then wrote, and those three steps were not one step: agents reading
the same unclaimed task all passed the check and all wrote, and every one of
them was handed a claim_id and a success. The loser found out only when its
next write was refused, after the duplicate work was done. A test with eight
agents on one task used to report eight winners.

The guard is the `version` column, which the store already knew how to check
and nothing passed. A claim now carries the version it read and a stale one
comes back a conflict.

`next_task` no longer gives up when it loses that race. Losing a candidate is
not a reason to answer "no eligible task" while others are waiting, so it moves
on to the next one. Two agents asking at the same moment get one task each.

Ticking a check or changing tags now bumps the task version and `updated_at`.
Both changed the task while leaving it looking untouched, so a read taken
before either still looked current.

## 0.18.1

The mail `FROM` may now carry a display name, so a sign-in link arrives from
"Task Ledger" rather than a bare address. It used to be handed to `MAIL FROM`
verbatim, and an envelope sender is not allowed a display name, so anything but
a bare address was rejected by the mail server on every send. The header keeps
the full form and the envelope gets the address inside it.

A `FROM` with no address in it at all now leaves mail switched off instead of
being accepted at start and failing at every send. `FROM` still defaults to
`USER`, which is right when the username is an address and wrong for providers
whose username is a literal like `resend`.

## 0.18.0

Tokens can be revoked. Until now they could only be minted, so every token
ever issued stayed valid forever and a leaked one could not be taken out of
circulation. There was no listing either, so an owner could not even see what
was outstanding.

`GET /v1/:owner/tokens` lists what an owner has minted, live and revoked, and
`DELETE /v1/:owner/tokens/:id` kills one. Both are admin only, and a
ledger-bound write token is refused. The CLI gains `ledger token list` and
`ledger token revoke --id`, and MCP gains `list_tokens` and `revoke_token`. A
listing identifies a token by id and never returns the secret or its hash,
because neither can be recovered after the mint.

A revoked token fails every way in, not just the bearer header: the
magic-link-by-email lookup and the token load behind a one-time link check it
too. Revoking also deletes any HTML session signed in with that token and any
magic or review link still outstanding on it, so revocation is immediate rather
than a wait for the session to expire. Sessions gained a `token_id` to make
that possible. The token row itself stays, so the audit trail still shows who
held what.

A token cannot revoke itself. Revocation is irreversible and the plaintext is
gone, so revoking the one in your own hand would lock you out of the owner
completely. Mint the replacement, put it everywhere the old one lived, then
revoke the old id with the new token.

The unique index on token email now covers live tokens only. It used to span
every row, so a revoked token held its address hostage and the replacement
could never carry it, which made rotating an email-bound token impossible.
Existing databases migrate on start and keep their rows.

## 0.17.1

A lost `claim_id` no longer strands a task.

0.17.0 tightened `claim_id` across the mutating routes, which made an existing
gap bite: an agent that lost the id still held the lease and had no way back.
`steal` only applied to another actor, and release under your own actor name
demanded the id, so the task sat frozen until the lease ran out. Long leases
made that hours. Chats get compacted and sessions die, so this is routine
rather than rare.

`claim_task` with `steal` and a `reason` now takes a lease back under your own
actor as well as someone else's, minting a fresh `claim_id` and retiring the
old one. Every steal is still written to the event log, so the escape hatch is
audited rather than silent. An owner admin or operator token can now also
release any live lease without the id, which is what the release route already
claimed to allow for other actors.

A write token is unchanged: it still has to prove the claim.

## 0.17.0

Security and correctness pass over the whole server. Four fixes change
behaviour, so read the last two before upgrading a shared board.

Purge deleted a task's notes, checks, deps, idempotency rows and events
but never its tags, so a foreign key aborted the delete. One tagged DONE
task past the purge window stopped retention for every ledger on the
host. Reset already handled tags, which is why reset worked and purge did
not.

An `idempotency_key` was unique across the whole database while the replay
lookup was scoped to one ledger. Two ledgers under one owner using the
same key missed the lookup and then collided on the primary key, so create
failed with a raw SQLite error instead of replaying. The table is rebuilt
on `(ledger_id, key)`. Existing databases migrate on start and keep their
rows.

`set_check` and `set_tags` now require `claim_id` while a lease is live,
the same as `set_phase` and `close_task`. Any write token on the ledger
could previously tick another agent's checks and let it close on evidence
alone. `add_note` stays open on purpose: notes are append-only, so a
reviewer can comment on a task someone else holds.

An HTML sign-in now keeps its token's role. Every non-operator session was
recorded without one and then treated as owner admin, so a ledger-bound
write token could open ledger settings and change retention, which deletes
work. Settings need an admin session, matching the REST route. Sessions
made before this release carry no role and are treated as write, so sign
in again to reach settings.

The sign-in routes are rate limited to 10 attempts per address per minute.
Bearer tokens, magic codes, and review codes were all guessable at line
rate.

`serve` no longer prints the minted boot token to the service log, where
journald kept it. It writes the token to `<db>.boot-token` with mode 0600
and logs the path instead. Rotate any token that a previous version
logged.

MCP reports the real build version rather than 0.4.0, and `ledger://live`
renders against `LEDGER_PUBLIC_URL` rather than a hardcoded localhost.

Docs caught up with the code: the CLI is no longer described as future
work, the README API table lists the tags and review routes and the `done`
and `tag` query parameters, and `owner create --max-ledgers` says what it
actually does. The repository gained a real SECURITY.md, Dependabot,
CODEOWNERS, and issue and pull request templates.

## 0.16.1

The board's **all** tag chip links to the ledger path with no `tag`
query, so it leaves a filtered view. An empty `href` kept `?tag=`.

## 0.16.0

Owner admin or operator can reset a ledger: wipe every task and restart
the series at T-001. `POST /v1/:owner/:ledger/reset`, CLI
`ledger ledger reset`, MCP `reset_ledger`. `confirm` must be exactly
`owner/ledger`. Tokens and the ledger row stay. Write tokens are refused.

## 0.15.13

Skill and MCP docs: when to tag on a mixed board vs a dedicated ledger,
when not to, and what to do if this session's MCP schema is stale.

## 0.15.12

Phase headings sit in space, not between two similar rules. The last
row of a section no longer draws a trailing divider.

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
