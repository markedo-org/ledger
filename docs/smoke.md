# Smoke protocol

This is the repeatable proof for v1.0 gate 3: a real binary, a real MCP
session, create / get / claim / note / close, and Cursor MCP written to
disk. Run it after CLI or MCP changes, and before a release tag.

```bash
make smoke
```

That is `scripts/smoke.sh`. It builds a fresh binary (never the stale
`./ledger` in the repo), then:

1. `ledger init` with `--project-dir`, and checks `.cursor/mcp.json`
   contains the admin server and the boot token.
2. `ledger serve` on `127.0.0.1:18787`.
3. HTTP create / claim / note / close on `T-001`. Close must return
   `"phase":"DONE"`.
4. MCP create / get / claim / note / close on a second task, through
   `/mcp`, using the same sequence as `internal/workloop`.

`make test` includes `TestWorkLoopOnHandler` (MCP handler only) and
`TestWorkLoopThroughMux` (full mux at `/mcp`). Those do not start the
CLI binary. `make smoke` does.

MCP and HTTP call the same `app` methods. The smoke is not a substitute
for a human using Cursor, but it is the gate we refuse to argue with.

To run only the in-process loop:

```bash
go test -count=1 ./internal/workloop
```
