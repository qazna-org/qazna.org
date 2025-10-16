# Qazna Dashboards Overview

The API now serves two responsive, Bootstrap 5 driven control panels that surface
operational data for internal teams and participating central banks.

## Endpoints

| Audience | URL | Purpose |
|----------|-----|---------|
| Administrators | `http://localhost:8080/admin/dashboard` | Shows operational uptime, connected institutions, pending settlements, recent alerts, and latest ledger activity. |
| National/Central Banks | `http://localhost:8080/banks/dashboard` | Highlights reserve balances, intraday transaction volumes, liquidity pool health, settlement queue, and regional flow snapshot. |

Both dashboards share the global navigation shell and style guide defined in
`web/templates/parts`, but each page is rendered through its own template under
`web/templates/pages/`.

## Bootstrapping locally

1. Ensure the stack is stopped:
   ```bash
   docker compose down -v
   ```
2. Apply database migrations and seed demo data (PostgreSQL):
   ```bash
   export QAZNA_PG_DSN="postgres://postgres:postgres@localhost:15432/qz?sslmode=disable"
   make migrate-up
   make migrate-seed
   docker compose up -d --build
   make grafana-reset
   make demo-load WORKERS=6 DURATION=5m
   ```
   > Tip: run `make dev-up` (optionally followed by `make demo-load`) to execute all steps above using the environment variables.
4. Sanity-check the API:
   ```bash
   curl -s http://localhost:8080/healthz
   curl -s http://localhost:8080/readyz
   go test ./...
   ```
5. Visit the dashboards using the URLs above.

> NOTE: RSA signing keys rotate automatically and are stored in `auth_keys`. Retrieve public keys from `/v1/auth/jwks` if you integrate external clients.

## Structure recap

```
web/
  templates/
    layout/base.html        # common HTML skeleton
    parts/                  # navigation, footer, scripts, etc.
    pages/
      admin_dashboard.html  # admin console markup
      bank_dashboard.html   # central bank console markup
      map.html              # global flow map landing page
```

The handlers in `internal/httpapi/handlers.go` populate the templates with
placeholder data. When real metrics (Prometheus, Postgres, audit log) are ready,
replace these placeholders with dynamic queries to surface live metrics.
