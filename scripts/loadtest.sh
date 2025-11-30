#!/usr/bin/env bash
set -euo pipefail

RATE=${RATE:-2500}
DURATION=${DURATION:-60s}
TARGET=${TARGET:-http://localhost:8080/api/trucks}
VEGETA_BIN=${VEGETA_BIN:-$(command -v vegeta || true)}

if [[ -z "$VEGETA_BIN" ]]; then
  echo "vegeta not found, installing to GOPATH/bin..." >&2
  VEGETA_BIN="$(go env GOPATH)/bin/vegeta"
  go install github.com/tsenart/vegeta/v12@latest
fi

echo "GET ${TARGET}" | "${VEGETA_BIN}" attack -rate=${RATE} -duration=${DURATION} | "${VEGETA_BIN}" report
