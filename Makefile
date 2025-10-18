# Qazna.org – Dev conveniences

SHELL := /bin/bash
COMPOSE ?= docker compose
API_URL ?= http://localhost:8080

.PHONY: help
help:
	@echo "Targets:"
	@echo "  make build         - build Go binary locally"
	@echo "  make test          - go vet + go test"
	@echo "  make fmt           - go fmt"
	@echo "  make proto         - regenerate protobuf stubs with buf"
	@echo "  make rust          - cargo fmt/check/test in core/"
	@echo "  make up            - docker compose up -d --build"
	@echo "  make down          - docker compose down -v"
	@echo "  make logs          - tail api logs"
	@echo "  make health        - GET /healthz"
	@echo "  make smoke         - end-to-end API smoke (create accounts, transfer, list)"
	@echo "  make smoke-ledger  - gRPC smoke test against ledgerd"
	@echo "  make clean         - local cleanup (no docker volumes)"
	@echo "  make tla           - run TLC on docs/tla/ledger"

# ─── Go local builds/tests ──────────────────────────────────────────────────────
.PHONY: build
build:
	GO111MODULE=on CGO_ENABLED=0 go build -o bin/qazna-api ./cmd/api

.PHONY: test
test:
	go vet ./...
	go test ./...

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: proto
proto:
	@if ! command -v buf >/dev/null 2>&1; then \
		echo "buf binary is required (https://buf.build)."; exit 1; \
	fi
	rm -rf api/gen/go
	buf generate

# ─── Rust (core/) ──────────────────────────────────────────────────────────────
.PHONY: rust
rust:
	@cd core && cargo fmt --all -- --check || true
	@cd core && cargo check
	@cd core && cargo test -q

# ─── Docker compose lifecycle ──────────────────────────────────────────────────
.PHONY: up
up:
	$(COMPOSE) up -d --build

.PHONY: down
down:
	$(COMPOSE) down -v

.PHONY: logs
logs:
	$(COMPOSE) logs -f api

# ─── Database migrations ─────────────────────────────────────────────────────
.PHONY: migrate-up
migrate-up:
	@if [ -z "$(QAZNA_PG_DSN)" ]; then echo "QAZNA_PG_DSN must be set"; exit 1; fi
	go run ./cmd/migrate -dsn "$(QAZNA_PG_DSN)" up

.PHONY: migrate-down
migrate-down:
	@if [ -z "$(QAZNA_PG_DSN)" ]; then echo "QAZNA_PG_DSN must be set"; exit 1; fi
	go run ./cmd/migrate -dsn "$(QAZNA_PG_DSN)" down

.PHONY: migrate-seed
migrate-seed:
	@if [ -z "$(QAZNA_PG_DSN)" ]; then echo "QAZNA_PG_DSN must be set"; exit 1; fi
	go run ./cmd/migrate -dsn "$(QAZNA_PG_DSN)" seed

.PHONY: migrate-status
migrate-status:
	@if [ -z "$(QAZNA_PG_DSN)" ]; then echo "QAZNA_PG_DSN must be set"; exit 1; fi
	go run ./cmd/migrate -dsn "$(QAZNA_PG_DSN)" status

# ─── Health & smoke ────────────────────────────────────────────────────────────
.PHONY: health
health:
	@curl -s $(API_URL)/healthz | jq .

.PHONY: smoke
smoke:
	@echo "Health:" && curl -s $(API_URL)/healthz | jq . ; \
	TOKEN=$$(curl -s -X POST $(API_URL)/v1/auth/token -H 'Content-Type: application/json' -d '{"user":"smoke-admin","roles":["admin"]}' | jq -r .token); \
	if [ -z "$$TOKEN" ] || [ "$$TOKEN" = "null" ]; then echo "Failed to obtain auth token"; exit 1; fi; \
	ACC_A=$$(curl -s -X POST $(API_URL)/v1/accounts -H 'Content-Type: application/json' -H "Authorization: Bearer $$TOKEN" -d '{"currency":"QZN","initial_amount":100000}' | jq -r .id); \
	ACC_B=$$(curl -s -X POST $(API_URL)/v1/accounts -H 'Content-Type: application/json' -H "Authorization: Bearer $$TOKEN" -d '{"currency":"QZN","initial_amount":0}' | jq -r .id); \
	echo "A=$$ACC_A  B=$$ACC_B"; \
	JSON=$$(printf '{"from_id":"%s","to_id":"%s","currency":"QZN","amount":25000}' "$$ACC_A" "$$ACC_B"); \
	curl -s -X POST $(API_URL)/v1/transfers -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-1' -H "Authorization: Bearer $$TOKEN" -d "$$JSON" | jq .; \
	echo "Account A:" && curl -s $(API_URL)/v1/accounts/$$ACC_A -H "Authorization: Bearer $$TOKEN" | jq .; \
	echo "Balance B:" && curl -s "$(API_URL)/v1/accounts/$$ACC_B/balance?currency=QZN" -H "Authorization: Bearer $$TOKEN" | jq .; \
	curl -s $(API_URL)/v1/ledger/transactions?limit=5 -H "Authorization: Bearer $$TOKEN" | jq .

.PHONY: smoke-ledger
smoke-ledger:
	@go run ./cmd/smoke-ledger

# ─── Misc ──────────────────────────────────────────────────────────────────────
.PHONY: clean
clean:
	rm -rf bin/

.PHONY: tla
tla:
	@scripts/run_tlc.sh docs/tla/ledger

.PHONY: bench-local
bench-local:
	@if command -v hey >/dev/null 2>&1; then \
		echo "Running hey benchmark"; \
		hey -n 1000 -c 50 $(API_URL)/healthz | grep -E 'Requests/sec|Requests per second'; \
	elif command -v ab >/dev/null 2>&1; then \
		echo "Running ab benchmark"; \
		ab -n 1000 -c 50 $(API_URL)/healthz | grep 'Requests per second'; \
	else \
		echo "Install hey or apache bench (ab) to run bench-local"; exit 1; \
	fi

.PHONY: grafana-reset
grafana-reset:
	@if [ -z "$(QAZNA_GRAFANA_ADMIN_PASSWORD)" ]; then echo "QAZNA_GRAFANA_ADMIN_PASSWORD must be set"; exit 1; fi
	@scripts/grafana_reset_admin.sh

.PHONY: dev-up
dev-up:
	@if [ -z "$(QAZNA_POSTGRES_PASSWORD)" ]; then echo "QAZNA_POSTGRES_PASSWORD must be set"; exit 1; fi
	@if [ -z "$(QAZNA_GRAFANA_ADMIN_PASSWORD)" ]; then echo "QAZNA_GRAFANA_ADMIN_PASSWORD must be set"; exit 1; fi
	@if [ -z "$(QAZNA_AUTH_SECRET)" ]; then echo "QAZNA_AUTH_SECRET must be set"; exit 1; fi
	@set -euo pipefail; \
	PG_DSN="$${QAZNA_PG_DSN:-postgres://postgres:$(QAZNA_POSTGRES_PASSWORD)@localhost:15432/qz?sslmode=disable}"; \
	export QAZNA_PG_DSN="$$PG_DSN"; \
	export QAZNA_AUTH_SECRET="$$QAZNA_AUTH_SECRET"; \
	export QAZNA_POSTGRES_PASSWORD="$$QAZNA_POSTGRES_PASSWORD"; \
	export QAZNA_GRAFANA_ADMIN_PASSWORD="$$QAZNA_GRAFANA_ADMIN_PASSWORD"; \
	$(COMPOSE) up -d pg; \
	until $(COMPOSE) exec pg pg_isready -U postgres -d qz >/dev/null 2>&1; do sleep 1; done; \
	echo "Applying migrations..."; \
	go run ./cmd/migrate -dsn "$$PG_DSN" up; \
	echo "Seeding demo data..."; \
	go run ./cmd/migrate -dsn "$$PG_DSN" seed; \
	echo "Starting compose stack..."; \
	QAZNA_AUTH_SECRET="$$QAZNA_AUTH_SECRET" QAZNA_POSTGRES_PASSWORD="$$QAZNA_POSTGRES_PASSWORD" QAZNA_GRAFANA_ADMIN_PASSWORD="$$QAZNA_GRAFANA_ADMIN_PASSWORD" QAZNA_PG_DSN="$$PG_DSN" $(COMPOSE) up -d --build; \
	$(MAKE) grafana-reset >/dev/null; \
	echo "Local stack is ready. Grafana admin password synced."; \
	echo "Rust ledgerd gRPC listening on localhost:9091."

.PHONY: demo-load
demo-load:
	go run ./cmd/aidemo -duration=$(or $(DURATION),2m) -workers=$(or $(WORKERS),4)
