# ADR-0003: Secret Management Strategy

- Status: Draft
- Date: 2025-10-11

## Context

Секреты (пароли БД, токены внешних сервисов) пока передаются через переменные окружения. Для продакшена необходимо централизованное и аудируемое хранение секретов.

## Decision

1. **Kubernetes-секреты + SOPS (краткосрочно).**
   - Шифруем значения с помощью [Mozilla SOPS](https://github.com/mozilla/sops) и храним в git-репозитории (`deploy/secrets`).
   - Расшифровка выполняется на этапе деплоя (GitOps).

2. **HashiCorp Vault (долгосрочно).**
   - Для B2G требуется аппаратная поддержка (HSM) и аудит: Vault в HA-режиме.
   - Внедряем динамические креды для доступа к БД и сервисам.

3. **Стандартизируем переменные.**
   - Все сервисы читают секреты через `QAZNA_*` env vars.
   - Документация (`README`, `.env.example`) отражает минимальный набор.

## Consequences

- Добавляем инструмент SOPS в dev-toolchain.
- Helm chart должен ожидать Kubernetes Secret/ExternalSecret.
- CI в дальнейшем запрещает прямое размещение значений в YAML/Compose.

## References
- `.env.example`
- `docker-compose.yml`
- Helm chart (`helm/qazna`)
