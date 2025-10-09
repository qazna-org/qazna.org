## Qazna.org — Project Makefile (bootstrap)
## Usage:
##   make help

SHELL := /bin/bash

.PHONY: help fmt lint build clean proto docs

help:
	@echo "Qazna.org — common tasks"
	@echo "  make fmt     - format (Go, Rust) if present"
	@echo "  make lint    - quick static checks (Go vet)"
	@echo "  make build   - build stubs if code exists"
	@echo "  make proto   - placeholder for protobuf/TLA+"
	@echo "  make docs    - validate Markdown links (local)"
	@echo "  make clean   - clean build artifacts"

fmt:
	@if command -v go >/dev/null 2>&1 && [ -n "$$(find . -name '*.go' -print -quit)" ]; then \
		echo "[fmt] go fmt"; go fmt ./...; \
	else echo "[fmt] skip go"; fi
	@if command -v cargo >/dev/null 2>&1 && [ -n "$$(find core -name 'Cargo.toml' -print -quit)" ]; then \
		echo "[fmt] rust fmt"; cargo fmt; \
	else echo "[fmt] skip rust"; fi

lint:
	@if command -v go >/dev/null 2>&1 && [ -n "$$(find . -name '*.go' -print -quit)" ]; then \
		echo "[lint] go vet"; go vet ./...; \
	else echo "[lint] skip go"; fi

build:
	@if command -v go >/dev/null 2>&1 && [ -n "$$(find cmd -name main.go -print -quit)" ]; then \
		echo "[build] go build"; go build ./...; \
	else echo "[build] skip go"; fi
	@if command -v cargo >/dev/null 2>&1 && [ -n "$$(find core -name 'Cargo.toml' -print -quit)" ]; then \
		echo "[build] cargo build"; cargo build --release; \
	else echo "[build] skip rust"; fi

openapi:
	@echo "[openapi] spec at api/openapi.yaml"
	@test -f api/openapi.yaml && echo "OK" || (echo "missing openapi.yaml"; exit 1)

proto:
	@echo "[proto] .proto files under api/proto/"
	@test -d api/proto && find api/proto -name '*.proto' -print || (echo "missing api/proto"; exit 1)
	@echo "[proto] placeholder (add protobuf/TLA+ generation here)"

docs:
	@if command -v npx >/dev/null 2>&1; then \
		echo "[docs] markdown-link-check"; npx -y markdown-link-check -q README.md; \
	else echo "[docs] skipped (node not installed)"; fi

clean:
	@rm -rf bin/ dist/ build/ target/
	@echo "[clean] done"

docker:
	@docker build -t qazna/api:dev -f cmd/api/Dockerfile .
