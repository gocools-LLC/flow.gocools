#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${FLOW_SMOKE_PORT:-18080}"
BASE_URL="http://127.0.0.1:${PORT}"
TMP_DIR="$(mktemp -d)"
LOG_PATH="${TMP_DIR}/flow.log"

cleanup() {
  if [[ -n "${FLOW_PID:-}" ]]; then
    kill -TERM "${FLOW_PID}" >/dev/null 2>&1 || true
    wait "${FLOW_PID}" >/dev/null 2>&1 || true
  fi
  rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

cd "${ROOT_DIR}"

echo "Starting Flow smoke target on ${BASE_URL}..."
FLOW_HTTP_ADDR=":${PORT}" FLOW_INGEST_MODE=disabled go run ./cmd/flow >"${LOG_PATH}" 2>&1 &
FLOW_PID=$!

READY=0
for _ in $(seq 1 60); do
  if curl -fsS "${BASE_URL}/healthz" >"${TMP_DIR}/healthz.json" 2>/dev/null; then
    READY=1
    break
  fi
  sleep 0.25
done

if [[ "${READY}" -ne 1 ]]; then
  echo "Flow failed readiness checks."
  echo "Flow logs:"
  cat "${LOG_PATH}"
  exit 1
fi

healthz="$(cat "${TMP_DIR}/healthz.json")"
readyz="$(curl -fsS "${BASE_URL}/readyz")"
metrics="$(curl -fsS "${BASE_URL}/metrics")"
timeline="$(curl -fsS "${BASE_URL}/api/v1/incidents/timeline?stack_id=dev-stack&environment=dev&start=2026-03-05T00:00:00Z&end=2026-03-05T00:05:00Z")"
correlation="$(curl -fsS "${BASE_URL}/api/v1/telemetry/correlation?start=2026-03-05T00:00:00Z&end=2026-03-05T00:05:00Z&max_skew_seconds=120")"

echo "${healthz}" | grep -q '"status":"ok"' || {
  echo "unexpected /healthz response: ${healthz}"
  exit 1
}
echo "${readyz}" | grep -q '"status":"ready"' || {
  echo "unexpected /readyz response: ${readyz}"
  exit 1
}
echo "${metrics}" | grep -q 'flow_http_requests_total' || {
  echo "missing expected HTTP counter metric in /metrics output"
  exit 1
}
echo "${timeline}" | grep -q '"events"' || {
  echo "unexpected timeline API response: ${timeline}"
  exit 1
}
echo "${correlation}" | grep -q '"nodes"' || {
  echo "unexpected correlation API response: ${correlation}"
  exit 1
}

echo "Flow smoke checks passed."
