# Database Migrations

This directory contains PostgreSQL migrations and deterministic demo seeds.

## Structure

- `sql/` – ordered migration pairs following the pattern `NNNN_description.up.sql` and `NNNN_description.down.sql`.
- `seeds/` – idempotent data sets executed after schema is migrated.

## CLI usage

```bash
export QAZNA_PG_DSN="postgres://user:pass@localhost:15432/qz?sslmode=disable"

# Apply latest schema
make migrate-up

# Roll back the most recent migration
make migrate-down

# Apply demo data
make migrate-seed

# Inspect applied migrations
make migrate-status

# Run static linting (requires Atlas and Docker)
atlas migrate lint --config ops/migrations/atlas.hcl --env lint
```

All commands are backed by `go run ./cmd/migrate`, which records progress in the tables `schema_migrations` and `schema_seeds`.
