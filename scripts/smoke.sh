#!/usr/bin/env bash
# v1.0 gate 3. See docs/smoke.md.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TMP="$(mktemp -d)"
BIN="${SMOKE_BIN:-$TMP/ledger}"
if [[ ! -x "$BIN" ]]; then
  (cd "$ROOT" && go build -o "$BIN" ./cmd/ledger)
fi
LISTEN="127.0.0.1:18787"
BASE="http://${LISTEN}"
TOKEN="lgr_smoke"
PID=""
cleanup() {
  if [[ -n "$PID" ]]; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

export LEDGER_CONFIG="$TMP/config"
export LEDGER_BOOT_TOKEN="$TOKEN"
unset LEDGER_URL LEDGER_TOKEN LEDGER_PROFILE || true

PROJ="$TMP/repo"
"$BIN" init --owner smoke --ledger inbox --actor ada --db "$TMP/t.db" --listen "$LISTEN" --project-dir "$PROJ"
MCP_JSON="$PROJ/.cursor/mcp.json"
if [[ ! -f "$MCP_JSON" ]]; then
  echo "smoke: init did not write $MCP_JSON" >&2
  exit 1
fi
grep -q "task-ledger-admin" "$MCP_JSON" || {
  echo "smoke: mcp.json missing task-ledger-admin" >&2
  exit 1
}
grep -q "$TOKEN" "$MCP_JSON" || {
  echo "smoke: mcp.json missing boot token" >&2
  exit 1
}
grep -q "${BASE}/mcp" "$MCP_JSON" || {
  echo "smoke: mcp.json missing ${BASE}/mcp" >&2
  exit 1
}

"$BIN" serve -listen "$LISTEN" -db "$TMP/t.db" &
PID=$!

ok=0
for _ in $(seq 1 50); do
  if curl -sf -o /dev/null "$BASE/login"; then
    ok=1
    break
  fi
  sleep 0.1
done
if [[ "$ok" -ne 1 ]]; then
  echo "smoke: server did not start" >&2
  exit 1
fi

auth=(-H "Authorization: Bearer ${TOKEN}" -H "Content-Type: application/json")
curl -sf "${auth[@]}" -d '{"title":"Smoke HTTP","idempotency_key":"smoke-http-1"}' \
  "$BASE/v1/smoke/inbox/tasks" >/dev/null
claimed="$(curl -sf "${auth[@]}" -d '{}' \
  "$BASE/v1/smoke/inbox/tasks/T-001/claim")"
claim_id="$(printf '%s' "$claimed" | sed -n 's/.*"claim_id":"\([^"]*\)".*/\1/p')"
if [[ -z "$claim_id" ]]; then
  echo "smoke: HTTP claim missing claim_id: $claimed" >&2
  exit 1
fi
curl -sf "${auth[@]}" -d '{"body":"smoke note"}' \
  "$BASE/v1/smoke/inbox/tasks/T-001/notes" >/dev/null
closed="$(curl -sf "${auth[@]}" -d "{\"evidence\":\"scripts/smoke.sh\",\"claim_id\":\"${claim_id}\"}" \
  "$BASE/v1/smoke/inbox/tasks/T-001/close")"
echo "$closed" | grep -q '"phase":"DONE"' || {
  echo "smoke: HTTP close did not return DONE: $closed" >&2
  exit 1
}

(cd "$ROOT" && go run ./scripts/smokemcp -url "${BASE}/mcp" -token "$TOKEN" -key smoke-mcp-1)

echo "smoke: ok"
