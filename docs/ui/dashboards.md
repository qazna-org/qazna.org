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
2. Generate a local RSA key pair once (keys are written to `dev_keys/`):
   ```bash
   mkdir -p dev_keys
   openssl genpkey -algorithm RSA -pkeyopt rsa_keygen_bits:2048 -out dev_keys/private.pem
   openssl rsa -pubout -in dev_keys/private.pem -out dev_keys/public.pem
   ```
3. Export the keys for the current shell and start the stack:
   ```bash
   export QAZNA_AUTH_PRIVATE_KEY="$(cat dev_keys/private.pem)"
   export QAZNA_AUTH_PUBLIC_KEY="$(cat dev_keys/public.pem)"
   docker compose up -d --build
   ```
4. Sanity-check the API:
   ```bash
   curl -s http://localhost:8080/healthz
   curl -s http://localhost:8080/readyz
   go test ./...
   ```
5. Visit the dashboards using the URLs above.

> NOTE: `QAZNA_AUTH_*` exports are **not** persisted; run the export commands
> again in any new shell before `docker compose up`.

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
