# ADR-0002: Core Ledger Implementation in Rust

- Status: Draft
- Date: 2025-10-11
- Authors: Qazna Foundation Technical Council

## Context

Проект Qazna объявляет Rust в качестве языка для ядра (ledger runtime, consensus, cryptographic accounting). На момент ADR ядро в директории `core/ledger` содержит только заглушку. Финансовая инфраструктура требует детерминированности, безопасности и формальной верификации.

## Decision

1. **Ядро бухгалтерии переходит на Rust.**
   - Создаём crate `core/ledger` как часть `core/` workspace.
   - Реализуем deterministic state machine: журнал операций, double-entry bookkeeping, проверка инвариантов.
   - Разграничиваем «storage layer» (под управлением Go service) и «execution layer» (Rust).

2. **Интерфейс между Go и Rust — через protobuf/gRPC (в дальнейшем FFI).**
   - На первом этапе используем gRPC API внутри контейнера.
   - Рассмотрим FFI (cgo) после стабилизации протоколов.

3. **Формальная верификация.**
   - Пишем TLA+ спецификации для базовых сценариев (создание счёта, перевод, идемпотентность).
   - Связываем проверку с CI.

4. **Архитектура данных.**
   - Сохраняем PostgreSQL как первичное хранилище для MVP.
   - Рассматриваем FoundationDB/Kafka для event sourcing в следующих фазах.

## Consequences

- Go API становится thin-layer вокруг Rust ядра.
- Появляется отдельная команда/подпроцесс по Rust-разраб., тестированию и формальной верификации.
- Расширяем CI: `cargo fmt/check/test`, `go vet/test`, `tla`. 

## References

- README.md — roadmap (Phase 1 → Phase 2).
- docs/adr/0001-b2g-strategy.md — приоритет B2G.
