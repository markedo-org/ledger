# Meter, freeze, and operator provisioning

Settled 2026-08-13 with LG. Stripe is not in this binary. This file is what
ledger must grow next, and how the hosted site will talk to it later.

## Split

Ledger owns the meter and the consequences. The hosted site owns money.

One `ledger` process, one listen address. That is already HTTP API, HTML, and
MCP. Operator provisioning is more `/v1` routes and, if we add it, one more
HTML page on the same server. Not a second binary, not a second port, not a
fleet of admin services.

- `max_ledgers` on the owner is the only number the binary understands.
- Create over the cap already returns 422 `policy: max_ledgers reached`.
- An operator API (and an optional HTML page on the same process) lets a host
  create owners, create ledgers, and raise or lower `max_ledgers`.
- Self-hosters call that API with curl or from their own site. Markedo's
  brochure will be one client of it, not a special case.
- The brochure is already a separate static site (`www.task-ledger.com`).
  Stripe later is a checkout page plus one webhook path on that host, which
  computes an integer and calls ledger. Not a billing product line. Stripe
  never lives in this repo.

Owner-scoped `role=admin` can create ledgers and mint tokens *under that
owner*. It must not create owners or change `max_ledgers`. If it could, anyone
with an admin token would skip payment.

## Sequence

1. Operator provisioning in ledger (API first, optional HTML page second).
2. Overflow freeze in ledger (so a decrease has a real effect).
3. Only then: Stripe on the brochure, webhook receiver, provisioning client
   toward this API.

Do not put checkout UI on the brochure before 1 and 2 exist.

## Operator token

A host-level credential, distinct from the first owner's boot token and from
owner `admin`. Working name: operator token (the HTML page may say admin).

It can:

- create an owner (slug, `max_ledgers` default 1)
- create a ledger under any owner (still subject to that owner's cap)
- set `max_ledgers` on an owner (never below 1)
- mint the first token for a new owner so they can sign in

Self-hosters set it in env. The Markedo website holds the hosted instance's
copy and never puts it in a tracked file. HTML operator page, if we add one,
signs in with this token the same way `/login` already exchanges a bearer
token for a session.

## Overflow freeze

Cap is create-only today. When `max_ledgers` drops below the number of
ledgers the owner already has, extra ledgers become read-only until the cap
rises again.

Derive freeze on every write. Do not store a frozen flag.

- Sort the owner's ledgers by `created_at` ascending (same second: insert
  order).
- The oldest `max_ledgers` stay writable.
- Newer ones are read-only. Newest goes read-only first.
- Floor is 1. The first ledger stays writable even if a webhook tries to send 0.
- Reject writes on a frozen ledger with a distinct 422
  (`policy: ledger over max_ledgers`).
- Reads stay up (HTML, markdown snapshot, GET, MCP list/get).
- Allow `release` so a lease does not sit stuck. Heartbeat and claim fail;
  the reaper expires the rest.
- HTML already cannot write. Show a banner on a frozen ledger so a human can
  see why agents cannot.

This is not the planned public-read-only boolean (v1.1, sharing). Different
flags.

Hosted free tier: new owner gets `max_ledgers=1` and one ledger. Markedo's
own owner stays on a high cap (bootstrap is 8) and is not the public signup
path. `0` may mean unlimited for self-host; hosted never sends 0.

## Hosted billing (later, not this binary)

Working price list for Markedo hosted, not a promise in the API:

- first ledger free
- extra ledger €2.99 / month, €29.90 / year, or €99 one-time
- one-time means for as long as we run the hosted service, not forever

The billing mapping (on the brochure host, later) turns Stripe into one integer:

```
max_ledgers = 1 + lifetime_count + subscription_quantity
```

Then it PATCHes ledger. A yearly cancel must not wipe a lifetime purchase.
Prefer one Stripe customer per owner, subscription quantity = paid extras,
Customer Portal to cancel or change quantity. Checkout (or a signed Payment
Link) carries `client_reference_id=owner-slug`. Do not take money against a
slug the operator API has not created.

VAT is a site/Stripe Tax problem. Failed payment freezes only after Stripe
gives up, not on the first failed invoice. Cancel at period end, not the
moment they click cancel.

Abuse of free signup (many owners, one ledger each) is acceptable at this
scale. Rate-limit owner creation. Identity is the hard part, not Stripe.

## Out of this slice

- Stripe products, webhook receiver, Customer Portal.
- Public self-serve signup on the brochure.
- Magic-link email.
- Letting the owner's admin token raise `max_ledgers`.
