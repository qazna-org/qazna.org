# Initial Backlog for B2G Trajectory

## Stream A — Core Ledger & Formal Methods
- [x] `core/ledger`: создать каркас state machine (инициализация, apply-entry, snapshot API).
- [x] Прототип gRPC-сервиса между Go/API и Rust-ядром.
- [x] TLA+: спецификация операций `CreateAccount` и `Transfer`, интеграция в CI.

## Stream B — Security & Access Control
- [x] Проект RBAC: схема БД (users, organizations, roles, permissions).
- [x] Проект API авторизации (OAuth 2.0/OIDC, token issuance, audit log).
- [x] Draft audit log storage (append-only).

## Stream C — Infrastructure & CI
- [x] GitHub Actions (lint, tests, buf) — **готово**.
- [x] Helm chart scaffolding + values (pg, api, prometheus, grafana).
- [x] Secret management ADR (Vault / SOPS).

## Stream D — B2G Pilot & Governance
- [ ] Whitepaper / презентация для пилотного регулятора (через docs/).
- [ ] Charter обновление — секция о founding members и тех. комитете.
- [ ] Демонстрационный стенд: подготовить сценарии на Grafana + карта потоков.

> Эти пункты будем переносить в GitHub Issues (инкапсулируя по epic / milestone) по мере готовности.
