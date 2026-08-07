# Security и supply-chain profile

## Область применимости

Это локальный read-only CLI/TUI-инструмент. Он читает публичные HTTPS-источники,
локальный YAML/TSV/JSON и пользовательский конфиг, записывает cache и отчёт в
каталог данных. Секреты для работы не требуются; API credentials не принимаются
через аргументы, help, логи или fixtures. Установка, публикация и подпись
артефактов находятся за пределами этого репозитория.

## Makefile gates

- `make security` выполняет базовый `go vet` и не заменяет ручной threat-model review.
- `make secrets-check` ищет tracked private keys и высокоинформативные token patterns.
- `make dependency-check` требует установленный `govulncheck` и OSV-Scanner, не
  изменяет `go.mod`, `go.sum` или dependency graph и сохраняет native output и
  commit/tool-version evidence в `.release/`. Отсутствие scanner является blocker,
  а не успешным NO-OP.
- `make sbom` требует Syft и генерирует SPDX JSON в `.release/sbom.spdx.json`.
- `make checksums` создаёт SHA-256 checksum локального бинарника.
- `make verify-local-artifact` проверяет строгую схему manifest/checksum, exact tag и
  commit, а также digest самого локального бинарника.
- `make verify-provenance` и `make signature` являются честными локальными
  NO-OP: в checkout нет CI builder, published artifact или signing identity для
  проверки. Они не утверждают provenance или подпись опубликованного объекта.

Инструменты не скачиваются автоматически. Версии scanner, источник базы и
политика severity должны быть закреплены в CI/release-профиле до публикации.

## Границы надёжности

Сетевые операции выполняются с context cancellation и конечными timeout; ошибки
внешних benchmark-источников не маскируются как свежие данные и явно попадают в
отчёт. TUI применяется только в TTY, а `table`, `check` и `--help` пригодны для
pipe/cron/CI. CLI-пути из config относительны к текущему рабочему каталогу;
Makefile targets всегда нормализуют root checkout через `make -C`/`ROOT` и не
зависят от cwd вызывающего процесса.
