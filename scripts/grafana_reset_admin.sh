#!/usr/bin/env bash

set -euo pipefail

if [[ -z "${QAZNA_GRAFANA_ADMIN_PASSWORD:-}" ]]; then
  echo "QAZNA_GRAFANA_ADMIN_PASSWORD is required" >&2
  exit 1
fi

if ! docker compose ps grafana >/dev/null 2>&1; then
  echo "Grafana container is not part of the compose project" >&2
  exit 1
fi

attempts=0
until docker compose ps --status running grafana >/dev/null 2>&1; do
  attempts=$((attempts + 1))
  if (( attempts > 30 )); then
    echo "Grafana container did not reach running state" >&2
    exit 1
  fi
  sleep 2
done

for i in $(seq 1 15); do
  if docker compose exec grafana grafana-cli admin reset-admin-password "$QAZNA_GRAFANA_ADMIN_PASSWORD"; then
    echo "Grafana admin password reset to the value of QAZNA_GRAFANA_ADMIN_PASSWORD."
    exit 0
  fi
  sleep 2
done

echo "Failed to reset Grafana password after multiple attempts." >&2
exit 1
