#!/usr/bin/env bash
# Cross-compile release binaries into dist/.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
VERSION="${VERSION:-$(cat VERSION)}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo unknown)}"
DATE="${DATE:-$(date -u +%Y-%m-%d)}"
LDFLAGS="-X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}"
mkdir -p dist
build() {
  local goos="$1" goarch="$2" ext="${3:-}"
  local out="dist/ledger-${goos}-${goarch}${ext}"
  echo "==> $out"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -ldflags "$LDFLAGS" -o "$out" ./cmd/ledger
}
build darwin arm64
build linux amd64
build windows amd64 .exe
ls -l dist
