#!/usr/bin/env bash
# Criterion 3 for v1.0: init, serve, create, claim, note, close.
# HTTP is the loop. MCP uses the same app methods.
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

"$BIN" init --owner smoke --ledger inbox --actor ada --db "$TMP/t.db" --listen "$LISTEN" --no-write-cursor
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
curl -sf "${auth[@]}" -d '{"title":"Smoke loop","idempotency_key":"smoke-1"}' \
  "$BASE/v1/smoke/inbox/tasks" >/dev/null
curl -sf "${auth[@]}" -d '{}' \
  "$BASE/v1/smoke/inbox/tasks/T-001/claim" >/dev/null
curl -sf "${auth[@]}" -d '{"body":"smoke note"}' \
  "$BASE/v1/smoke/inbox/tasks/T-001/notes" >/dev/null
closed="$(curl -sf "${auth[@]}" -d '{"evidence":"scripts/smoke.sh"}' \
  "$BASE/v1/smoke/inbox/tasks/T-001/close")"
echo "$closed" | grep -q '"phase":"DONE"' || {
  echo "smoke: close did not return DONE: $closed" >&2
  exit 1
}
echo "smoke: ok"
