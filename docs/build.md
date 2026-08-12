# Build order

1. Store and domain: owner, ledger, series `T`, task allocation, claims, events.
2. HTTP API (intent-shaped) plus read-only HTML and markdown snapshot.
3. Point the skill at the real API and dogfood `markedo/markedo-meta`.
4. MCP Streamable HTTP over the same domain layer.
5. GitHub OAuth for hosted humans.

Do not start 4 or 5 until 1 to 3 are dogfoodable.
