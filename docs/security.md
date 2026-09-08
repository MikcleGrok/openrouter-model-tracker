# Security и supply-chain profile

## Область применимости

Это локальный read-only CLI/TUI-инструмент. Он читает публичные HTTPS-источники,
локальный YAML/TSV/JSON и пользовательский конфиг, записывает cache и отчёт в
каталог данных. Секреты для работы не требуются; API credentials не принимаются
через аргументы, help, логи или fixtures. Установка, публикация и подпись
артефактов находятся за пределами этого репозитория.

## Makefile gates

- `make check` — SCA-freshness staleness gate (guide-tools
  08-security-and-reliability.md, «Gate по свежести SCA-evidence»), не
  доменная команда: он читает `.release/dependency-evidence.json` от
  последнего `make dependency-check`, сверяет input digest с текущими
  `go.mod`/`go.sum` и требует `scan_status == clean` не старше 7 дней
  (`SCA_CADENCE_DAYS`; weekly — publishable profile, не 30-дневный plain-CLI
  дефолт). Ничего не сканирует и не требует сети сам по себе;
  просроченная, отсутствующая или не-`clean` evidence — blocker с точной
  командой-подсказкой. `make release-check` выполняет ту же проверку повторно
  как pre-tag ступень. Прежняя доменная проверка каталога — `make cli-check`.
- `make security` выполняет базовый `go vet` и не заменяет ручной threat-model review.
- `make secrets-check` ищет tracked private keys и высокоинформативные token patterns.
- `make dependency-check` запускает `govulncheck` и OSV-Scanner, закреплённые
  по точной версии через tool directive в `tools/go.mod` (никогда не
  разрешаются через `PATH`), не изменяет `go.mod`, `go.sum` или dependency
  graph и сохраняет strict v3 evidence с `scan_status`, `findings`,
  `policy_decision`, input digest, tool/database metadata и хешированными
  native outputs в `.release/`. `scan_status` — ровно одно из четырёх
  значений: `clean` (находок нет), `findings` (сканер нашёл хотя бы одну),
  `error` (сканер не смог отработать или дал непригодный вывод) или `partial`
  (часть сканеров ошиблась, часть — нет); это ровно те четыре состояния,
  которые определяет контракт `make dependency-check` в
  guide-tools/08-security-and-reliability.md. Успешным gate считается только
  `clean`.
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

## Известное расхождение: два канала бинарных assets

Onboarding record (`docs/reference.md`, `channels`) декларирует два реально
живых канала asset-channel-типа, но с разным происхождением:

1. **Self-repo GitHub Release** — `github.com/MikcleGrok/openrouter-model-tracker`,
   тег `vMAJOR.MINOR.PATCH`, публикуется через `make release-github` (полный
   provenance: manifest, signature, attestation, SBOM). Проверяется машинно
   из этого checkout через `make distribution-check`.
2. **Homebrew asset-channel formula** `mikclegrok/tools/openrouter-model-tracker`
   (канонический org tap, precedent — `uni-chat`) — реально установлена и
   работает (`brew info mikclegrok/tools/openrouter-model-tracker`), но её
   `url`/`sha256` ссылаются на **другой** host и tag: shared-репозиторий
   `github.com/MikcleGrok/tools`, тег с project-namespace
   `openrouter-model-tracker-vX.Y.Z` — тот же паттерн, что у `uni-chat`
   (`uni-chat-vX.Y.Z` в том же shared repo). Эта formula верифицирована
   только вручную (нет `Formula/*.rb` файла в этом checkout, поэтому нет
   входа для `guide-distribution-verify`); синхронизирующий скрипт
   (внешний, не часть этого репозитория) публикует релиз с другим
   owner/repo/tag, чем `make release-github` — то есть путь от `git tag` в
   этом checkout до опубликованного Homebrew asset проходит через ручной
   внешний шаг, а не через один документированный Makefile-flow.

Оба канала реальны и живы — это не выдуманная возможность и не сломанный
канал, — но их несогласованность (два разных host/tag на один и тот же
релиз) не устранена в рамках этой правки: устранение потребовало бы либо
привести внешний publish-скрипт в соответствие с `GITHUB_REPOSITORY`/`vX.Y.Z`
этого репозитория, либо описать и заскриптовать канонический republish-шаг
из self-repo релиза в shared-repo asset. Зафиксировано здесь как известный
факт для следующего пересмотра onboarding record, а не смолчано.
