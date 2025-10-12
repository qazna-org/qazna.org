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
	@echo "  make clean         - local cleanup (no docker volumes)"

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

# ─── Health & smoke ────────────────────────────────────────────────────────────
.PHONY: health
health:
	@curl -s $(API_URL)/healthz | jq .

.PHONY: smoke
smoke:
	@echo "Health:" && curl -s $(API_URL)/healthz | jq . ; \
	ACC_A=$$(curl -s -X POST $(API_URL)/v1/accounts -H 'Content-Type: application/json' -d '{"currency":"QZN","initial_amount":100000}' | jq -r .id); \
	ACC_B=$$(curl -s -X POST $(API_URL)/v1/accounts -H 'Content-Type: application/json' -d '{"currency":"QZN","initial_amount":0}' | jq -r .id); \
	echo "A=$$ACC_A  B=$$ACC_B"; \
	JSON=$$(printf '{"from_id":"%s","to_id":"%s","currency":"QZN","amount":25000}' "$$ACC_A" "$$ACC_B"); \
	curl -s -X POST $(API_URL)/v1/transfers -H 'Content-Type: application/json' -H 'Idempotency-Key: demo-1' -d "$$JSON" | jq .; \
	echo "Account A:" && curl -s $(API_URL)/v1/accounts/$$ACC_A | jq .; \
	echo "Balance B:" && curl -s "$(API_URL)/v1/accounts/$$ACC_B/balance?currency=QZN" | jq .; \
	curl -s $(API_URL)/v1/ledger/transactions?limit=5 | jq .

# ─── Misc ──────────────────────────────────────────────────────────────────────
.PHONY: clean
clean:
	rm -rf bin/
