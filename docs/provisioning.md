# Meter, freeze, and operator provisioning

Settled 2026-08-13 with LG. Stripe is not in this binary. This file is how
the meter works today and how the hosted site will talk to it later.

## Split

Ledger owns the meter and the consequences. The hosted site owns money.

One `ledger` process, one listen address. HTTP API, HTML, MCP, and operator
provisioning share that process. Operator routes are on `/v1` and `/admin`.

- `max_ledgers` on the owner is the only number the binary understands.
- Create over the cap returns 422 `policy: max_ledgers reached`.
- The operator API and HTML `/admin` page create owners, create ledgers, and
  raise or lower `max_ledgers`.
- Self-hosters call that API with curl, the CLI, or from their own site.
  Markedo's brochure will be one client of it, not a special case.
- The brochure is a separate site (`www.task-ledger.com`). Stripe there is a
  checkout page plus one webhook path, which computes an integer and calls
  ledger. Not a billing product line. Stripe never lives in this repo.

Owner-scoped `role=admin` can create ledgers and mint tokens *under that
owner*. It must not create owners or change `max_ledgers`. If it could, anyone
with an admin token would skip payment.

## What ships today

Operator provisioning and overflow freeze are in the binary. Stripe is not,
and will not be: money lives in the hosted site, a separate service that talks
to this API like any other client.

1. Operator API: `POST /v1/owners`, `GET /v1/owners`, `PATCH /v1/owners/:owner`
   (set `max_ledgers`). Same auth as MCP `create_owner` and `set_max_ledgers`.
2. HTML `/admin` when signed in with the operator token (or a GitHub allowlist
   session). Create owners, set caps, create ledgers, mint tokens.
3. Overflow freeze when `max_ledgers` is below the ledger count.

## Operator token

A host-level credential, distinct from the first owner's boot token and from
owner `admin`. Working name: operator token (the HTML page may say admin).

It can:

- create an owner (slug; `max_ledgers` defaults to 1 on create, and `0` on
  create is treated as 1)
- create a ledger under any owner (still subject to that owner's cap)
- set `max_ledgers` on an owner (`0` means unlimited; negative values are
  rejected)
- mint the first token for a new owner so they can sign in

Self-hosters set it in env (`LEDGER_OPERATOR_TOKEN`). The Markedo website holds
the hosted instance's copy and never puts it in a tracked file. HTML: sign in
at `/login` with that token, then `/admin`. See [`auth.md`](auth.md).

CLI: `ledger owner create`, `ledger owner list`, `ledger owner set-max
--max-ledgers N` with an operator profile. MCP: `create_owner`,
`set_max_ledgers`.

## Overflow freeze

When `max_ledgers` drops below the number of ledgers the owner already has,
extra ledgers become read-only until the cap rises again. When
`max_ledgers` is `0` (unlimited), nothing freezes.

Derive freeze on every write. Do not store a frozen flag.

- Sort the owner's ledgers by `created_at` ascending (same second: insert
  order).
- The oldest `max_ledgers` stay writable.
- Newer ones are read-only. Newest goes read-only first.
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
path. `0` means unlimited for self-host (`ledger owner set-max --max-ledgers 0`
or MCP `set_max_ledgers` with `0`). Hosted never sends `0`.

## Hosted billing (a separate service, not this binary)

Price list for Markedo hosted, not a promise in the API:

- first ledger free
- extra ledger €2.99 / month, €29.90 / year, or €99 one-time
- one-time means for as long as we run the hosted service, not forever

The billing mapping, on the brochure host, turns Stripe into one integer:

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

## Out of scope here

- Stripe products, webhook receiver, Customer Portal.
- Public self-serve signup on the brochure.
- Letting the owner's admin token raise `max_ledgers`.
