#!/usr/bin/env bash

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
READY_TIMEOUT="${READY_TIMEOUT:-60}"

deadline=$((SECONDS + READY_TIMEOUT))
while true; do
	if curl -sf "${BASE_URL}/readyz" | jq -e '.status=="ready"' >/dev/null 2>&1; then
		break
	fi
	if (( SECONDS >= deadline )); then
		echo "readyz not ready after ${READY_TIMEOUT}s" >&2
		exit 1
	fi
	sleep 1
done

token=$(curl -sf -X POST "${BASE_URL}/v1/auth/token" \
	-H 'Content-Type: application/json' \
	-d '{"user":"demo","roles":["admin"]}' | jq -r '.token')

if [[ -z "${token}" || "${token}" == "null" ]]; then
	echo "failed to obtain token" >&2
	exit 1
fi

curl -sf -H "Authorization: Bearer ${token}" "${BASE_URL}/v1/ledger/transactions?limit=1" >/dev/null

curl -sf "${BASE_URL}/metrics" | grep -q '^build_info'

echo "E2E smoke passed"
