# Build order

1. Store and domain: owner, ledger, series `T`, task allocation, claims, events.
2. HTTP API (intent-shaped) plus read-only HTML and markdown snapshot.
3. Point the skill at the real API and dogfood one ledger.
4. MCP Streamable HTTP over the same domain layer.
5. GitHub OAuth for host humans (optional). Token login is the baseline.
   See `docs/auth.md`.
6. Self-host with `docs/deploy.md`. Markedo's hosted instance uses
   `scripts/deploy.sh`.
7. Operator provisioning: create owners, set `max_ledgers`, overflow freeze
   (`docs/provisioning.md`). Same API for self-hosters and for a site.
8. Stripe, if any, lives on the brochure. Not in this binary.

Do not start 4 or 5 until 1 to 3 are dogfoodable. Do not start 8 until 7
exists.
