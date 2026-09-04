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
  изменяет `go.mod`, `go.sum` или dependency graph и сохраняет strict v2 evidence
  с `scan_status`, `findings`, `policy_decision`, input digest, tool/database
  metadata и хешированными native outputs в `.release/`. Отсутствие scanner является `blocked`,
  ошибка сканера — `error`, смешанный результат — `partial`; ни один из них не
  считается успешным gate.
- `make sbom` требует Syft и генерирует SPDX JSON в `.release/sbom.spdx.json`.
- `make checksums` создаёт SHA-256 checksum локального бинарника.
- `make verify-local-artifact` проверяет строгую схему manifest/checksum, exact tag и
  commit, а также digest самого локального бинарника.
- `make release-check` не создаёт manifest/checksum и не утверждает опубликованное
  evidence; эти локальные артефакты проверяются только после exact tag через
  `make verify-local-artifact`.
- `make verify-release` выполняет только read-only проверку локального stable Homebrew
  channel: exact tag/version/commit, clean checkout, локальную formula, установленную
  версию, оба варианта CLI version и `brew test`. Источник formula намеренно `file://`;
  это не доказательство GitHub publication, подписи или provenance.
- `make verify-provenance` и `make signature` по умолчанию работают в
  `PROVENANCE_PROFILE=local`: печатают `NOT APPLICABLE`, завершаются с кодом 0,
  не вызывают cosign и не создают signed/provenance evidence. `candidate` имеет
  ту же семантику для pre-tag gate.
- `PROVENANCE_PROFILE=external` (алиас `published`) является отдельным будущим
  или внешним profile публикации. Read-only verification требует committed
  public key, signed evidence и `cmd/evidencecheck`, но не требует
  `COSIGN_PRIVATE_KEY`; приватный ключ нужен только для `make sign` и
  `make attest`. Оба профиля проходят одинаковый полный verification path и
  fail-closed при отсутствии любого обязательного bundle. `codesign` identity
  не используется как cosign key.

Инструменты не скачиваются автоматически. Версии scanner, источник базы и
политика severity должны быть закреплены в CI/release-профиле до публикации.

## Границы надёжности

Сетевые операции выполняются с context cancellation и конечными timeout; ошибки
внешних benchmark-источников не маскируются как свежие данные и явно попадают в
отчёт. TUI применяется только в TTY, а `table`, `check` и `--help` пригодны для
pipe/cron/CI. CLI-пути из config относительны к каталогу самого config-файла;
Makefile targets всегда нормализуют root checkout через `make -C`/`ROOT` и не
зависят от cwd вызывающего процесса.

## Ключ подписи релизов

Ключевая пара cosign была ротирована 2026-09-04. Предыдущий приватный ключ
существовал только в write-only секретах GitHub Actions
(`OPENROUTER_TRACKER_COSIGN_KEY`, `OPENROUTER_TRACKER_COSIGN_PASSWORD`,
созданы 2026-08-08) и был безвозвратно утерян — потребляющий их workflow был
удалён, а перечитать write-only секрет невозможно.

Новый приватный ключ и его пароль хранятся в macOS login Keychain владельца
(аккаунт `mickle.grok`), а не в CI:

- `cosign.openrouter-model-tracker.private-key` — base64 зашифрованного паролем
  cosign PEM;
- `cosign.openrouter-model-tracker.key-password` — пароль к этому ключу.

Получить их вручную:

```sh
security find-generic-password -s cosign.openrouter-model-tracker.private-key -a mickle.grok -w | openssl base64 -d -A
security find-generic-password -s cosign.openrouter-model-tracker.key-password  -a mickle.grok -w
```

Проверить, что ключ в Keychain действительно является приватной половиной
закоммиченного `cosign.pub`, можно через `make cosign-key-check`. Подписать и
приложить attestation к релизу, используя ключ и пароль напрямую из Keychain
(без ручного экспорта в переменные окружения), можно через
`make cosign-sign-release`.

Дополнительно ключевая пара хранится в холодной резервной копии —
GPG-зашифрованном архиве в
`~/Documents/mickle.grok/cosign-key-backup/openrouter-model-tracker/`.
Пароль к архиву — отдельный секрет, Keychain-item
`cosign.backup-bundle.passphrase` (аккаунт `mickle.grok`), общий для всех
резервных копий cosign-ключей этого владельца.

Старый публичный ключ сохранён как `cosign.pub.previous` — именно он
позволяет `scripts/verify-provenance.sh` (fallback `cosign.pub` →
`cosign.pub.previous`) по-прежнему проверять релизы, подписанные старым
ключом: v1.5.0, v1.6.0, v1.7.0, v1.9.0, v1.10.1, v1.11.1, v1.12.1, v1.13.7,
v1.13.8, v1.13.9, v1.13.10, v1.13.13.

Этот ключ никогда не должен попадать в secret CI/CD-системы (GitHub Actions и
подобные) — именно так был потерян предыдущий.
