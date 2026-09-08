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
  `clean`. OSV-Scanner запускается в full-native режиме: `osv-scanner scan
  source --lockfile go.mod --data-source native --all-vulns` — `native`
  вместо дефолтного `deps.dev` (прямые OSV-базы, а не сторонний API) и
  `--all-vulns` (без фильтрации неважных/невызываемых находок), как того
  требует guide-tools 08-security-and-reliability.md для этой версии
  (`github.com/google/osv-scanner/v2 v2.5.1`, закреплена в `tools/go.mod`).
  Перед каждым запуском gate сам снимает `osv-scanner scan source --help`
  пиненой версии в `.release/osv-scanner-help.txt` и проверяет, что оба флага
  там документированы; отсутствие любого из них — hard fail с явным
  сообщением, а не молчаливый откат на дефолтный режим. `scan_status` этой
  проверки на 2026-09-08 — `clean` (0 находок с этими флагами); реестра
  исключений в проекте нет, потому что найти было нечего.
- `make sbom` требует Syft и генерирует SPDX JSON в `.release/sbom.spdx.json`.
- `make checksums` создаёт SHA-256 checksum локального бинарника.
- `make verify-local-artifact` проверяет строгую схему manifest/checksum, exact tag и
  commit, а также digest самого локального бинарника.
- `make release-check` не создаёт manifest/checksum и не утверждает опубликованное
  evidence; эти локальные артефакты проверяются только после exact tag через
  `make verify-local-artifact`.
- `make verify-release` выполняет read-only проверку локального stable Homebrew
  channel: exact tag/version/commit, clean checkout, локальную formula, установленную
  версию, оба варианта CLI version и `brew test`. Источник formula намеренно `file://`;
  это не доказательство GitHub publication, подписи или provenance. Дополнительно
  запускает `make homebrew-source-check` — machine-checkable верификацию
  **канонического опубликованного** Homebrew-канала (`mikclegrok/tools/openrouter-model-tracker`,
  отдельно от локального disposable tap выше): tap collision, installed receipt,
  cross-check `brew info --json=v2`/`brew --prefix`/`command -v`/`realpath`/`<binary>
  version` и evidence `.release/homebrew-source-evidence.json`. Требует `brew` и сеть
  (host-only exception, см. onboarding record), поэтому не входит в `make check`.
- `make homebrew-source-check` — та же проверка канонического канала отдельным
  target'ом (`VERSION=...` обязателен), для запуска без полного `verify-release`.
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

## Криптография и TLS

guide-tools 08-security-and-reliability.md requires cryptography to use
supported libraries and algorithms fixed in the project's own security
profile, with a cited source/standard, plus verified TLS hostname/chain
checking and an explicit pinning policy. This section is that fixation, and
every claim below was verified against this checkout rather than assumed.

**Подпись релизов.** The committed release-signing key is ECDSA over the
NIST P-256 curve — confirmed live, not assumed, via
`openssl pkey -pubin -text -noout < cosign.pub` (`ASN1 OID: prime256v1`,
`NIST CURVE: P-256`); `cosign.pub.previous` (the rotated-out key still used
to verify older releases, see above) is the same curve. That is
[NIST FIPS 186-5](https://csrc.nist.gov/pubs/fips/186-5/final) (Digital
Signature Standard, which specifies ECDSA over NIST curves including P-256).
`cosign`'s default digest algorithm for this key type is SHA-256, i.e.
[NIST FIPS 180-4](https://csrc.nist.gov/pubs/fips/180/4/final) (Secure Hash
Standard).

**Provenance.** Release provenance is an in-toto Statement v1 with predicate
type SLSA Provenance v1 (`cosign attest-blob --type slsaprovenance1`,
`Makefile`'s `attest` target), matching
[SLSA v1.2](https://slsa.dev/spec/v1.2/) — this is a real, already-shipped
mechanism (guide-tools 08-security-and-reliability.md, "Provenance-предикат"
section), not an aspirational claim.

**Дайджесты.** SHA-256
([FIPS 180-4](https://csrc.nist.gov/pubs/fips/180/4/final)) is the only
digest algorithm this project computes or verifies: `checksums`/`SHA256SUMS`
(local and local-release artifacts), `input_digest` in dependency-check
evidence, `sha256` fields in `release-manifest.json` and every evidence
bundle in `.release/`.

**TLS.** The only outbound HTTPS client this project constructs is
`internal/httpcache/httpcache.go`'s `&http.Client{Timeout: timeout}` — no
custom `http.Transport` or `tls.Config`, so it runs on Go's
`http.DefaultTransport` and `crypto/tls` defaults exactly as shipped by the
pinned toolchain. `MinVersion` is not set explicitly by this project's code;
verified against the pinned toolchain itself (`go doc
crypto/tls.Config.MinVersion` on go1.26.7, the toolchain `.builder/local/
go-wrapper` selects for this repository at the time of writing): "By
default, TLS 1.2 is currently used as the minimum" — that is the actual
floor this client negotiates up from, honestly documented rather than
guessed, and it satisfies [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446)
(TLS 1.3, which Go's client negotiates when the server supports it) as an
upper bound. Neither `InsecureSkipVerify` nor any custom
`VerifyPeerCertificate`/`VerifyConnection` callback is set anywhere in this
codebase (verified: no match for either identifier), so hostname and
certificate-chain validation run unmodified against the OS trust store, per
[RFC 5280](https://www.rfc-editor.org/rfc/rfc5280) (chain/path validation)
and RFC 8446's own hostname-verification requirements.

**Pinning.** No certificate pinning is applied, as an explicit decision, not
a silent gap: every endpoint this tool talks to (OpenRouter's public API,
vals.ai, swebench.com, arena.ai's public JSON endpoints) is a public HTTPS
service whose CA and leaf certificates rotate on a schedule outside this
project's control, and pinning them would trade a real availability risk
(a legitimate rotation breaking every installed copy of this CLI with no
update path other than a new release) for a threat model — a compromised
public CA targeting these specific low-value read-only endpoints — this
project has no better defense against than the standard trust-store
validation already in place.

## Известное расхождение: два канала бинарных assets

Onboarding record (`README.md`, `channels`) декларирует два реально
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
   (`uni-chat-vX.Y.Z` в том же shared repo). Source provenance этой formula с
   2026-09-08 верифицируется machine-checkably через `make homebrew-source-check`
   (`scripts/verify-homebrew-source.sh`) — не чтением локального `Formula/*.rb`
   (в этом checkout такого файла нет, поэтому `guide-distribution-verify`'s
   `formula`-профиль ей не подходит), а прямыми запросами к живому состоянию
   `brew` (`search`/`list --formula --full-name`/`info --json=v2`/`--prefix`) и
   cross-check `command -v`/`realpath`/`<binary> version` против
   резолвнутого keg; evidence — `.release/homebrew-source-evidence.json`.
   Заодно подтверждён и задокументирован (README.md, «Установка») ещё один
   факт: canonical-бинарник этой formula называется `openrouter-model-tracker`
   (алиас `omt`), а не `openrouter`, как ошибочно утверждал README до этой
   правки, — другое имя, чем у локальной установки через `make install`.
   Синхронизирующий скрипт (внешний, не часть этого репозитория) публикует
   релиз с другим owner/repo/tag, чем `make release-github` — то есть путь от
   `git tag` в этом checkout до опубликованного Homebrew asset проходит через
   ручной внешний шаг, а не через один документированный Makefile-flow; это
   расхождение (в отличие от source provenance выше) остаётся неустранённым.

Оба канала реальны и живы — это не выдуманная возможность и не сломанный
канал, — но их несогласованность (два разных host/tag на один и тот же
релиз) не устранена в рамках этой правки: устранение потребовало бы либо
привести внешний publish-скрипт в соответствие с `GITHUB_REPOSITORY`/`vX.Y.Z`
этого репозитория, либо описать и заскриптовать канонический republish-шаг
из self-repo релиза в shared-repo asset. Зафиксировано здесь как известный
факт для следующего пересмотра onboarding record, а не смолчано.

## Crosswalk controls

guide-tools 08-security-and-reliability.md requires a filled crosswalk for
every control applicable to this project (the standards list there is a
menu, not a fixed mandatory set — only what actually applies gets a row).
Each row: `control | source + verified version and exact ID/section title |
automatic/manual check | evidence/gate | owner`. Per that same rule, an
ASVS/NIST ID that has not been checked against a pinned version of the
standard is marked `VERIFY` with the closest exact section title instead of
a guessed number. Controls this project genuinely does not have (container
runtime, Docker) live in the onboarding record's `N/A controls/rationale`
field (README.md) instead of a row here, so as not to duplicate that
statement.

| Control/практика | Источник и идентификатор/раздел | Проверка | Evidence/gate | Owner |
| --- | --- | --- | --- | --- |
| Dependency vulnerability scanning (SCA) | NIST SP 800-218 v1.1 (SSDF), practice family PW (Produce Well-Secured Software), конкретная subpractice `VERIFY` | automatic: `govulncheck` + OSV-Scanner (full-native mode), weekly cadence gate | `make dependency-check`, `make check` (staleness gate), `.release/dependency-evidence.json` | maintainer |
| Immutable dev-tool pinning (govulncheck/OSV-Scanner/mandoc) | NIST SP 800-218 v1.1 (SSDF), practice family PS (Protect the Software), конкретная subpractice `VERIFY` | automatic: `go tool` resolution from `tools/go.mod`'s own tool directives, never bare `PATH` | `tools/go.mod`, `make dependency-check` | maintainer |
| Release provenance (source, builder, artifact digest) | [SLSA v1.2](https://slsa.dev/spec/v1.2/), Provenance requirements (fact, not `VERIFY`: `--type slsaprovenance1` is the real predicate type this project emits — see «Криптография и TLS» above) | automatic: `cosign attest-blob` + `cosign verify-blob-attestation` | `make attest`, `make verify-provenance`, `.release/release-manifest.json.att.bundle.json` | maintainer |
| Signing key custody, algorithm and rotation | Этот файл, раздел «Ключ подписи релизов» и «Криптография и TLS» | manual: owner, Keychain storage, rotation procedure; automatic: `sign-flags-check` (no Rekor upload) | `make cosign-key-check`, `scripts/sign_flags_test.sh`, this file | maintainer |
| SBOM generation and artifact linkage | SPDX 2.3 (via Syft); this project's own supply-chain profile (this file, «Makefile gates») | automatic: `make sbom` generates from the exact artifact digested into `release-manifest.json` | `make sbom`, `.release/sbom.spdx.json` | maintainer |
| Secrets scanning of tracked source | NIST SP 800-218 v1.1 (SSDF), practice family PW (Produce Well-Secured Software), конкретная subpractice `VERIFY` | automatic: pattern scan (private-key headers, AWS/GitHub token shapes) + pre-commit hook | `make secrets-check`, `.githooks` (`make install-hooks`) | maintainer |
| TLS transport security for outbound HTTPS clients | [RFC 8446](https://www.rfc-editor.org/rfc/rfc8446) (TLS 1.3) + [RFC 5280](https://www.rfc-editor.org/rfc/rfc5280) (chain/path validation) | manual: verified once against the pinned Go toolchain's own documented defaults and this codebase's one `http.Client` construction, no automatic re-check | this file, «Криптография и TLS»; `internal/httpcache/httpcache.go` | maintainer |
| Install/uninstall path scoping (no unowned-file mutation) | Этот файл и README.md's onboarding record, поле `shared-location scoping` | automatic: unmanaged/foreign/mismatched destination preservation checks | `scripts/install_test.sh`, `make install-smoke` | maintainer |
| Repository tag-protection (no force-update/delete on release tags) | guide-tools 06-release.md, «Repository policy MUST запрещать force-update и delete release tags» | automatic: GitHub repository ruleset lookup via `gh api` | `make tag-protection-check`, `.release/tag-protection-evidence.json` (ruleset `protect-release-tags`, id 22587144) | maintainer |
| Homebrew asset-channel source provenance | guide-tools 11-distribution-verifier.md, «Acceptance criteria для source provenance» | automatic, run on the maintainer's machine (requires `brew`; not part of `make check`, which stays network/tool-free) | `make homebrew-source-check`, `.release/homebrew-source-evidence.json` | maintainer |

Ни один ряд не закрыт по одной ссылке: у каждого есть и источник с проверенной
версией, и реальный gate или evidence-файл. Строка про builder identity
намеренно отсутствует: подпись выполняется статическим cosign-ключом без
Fulcio/Rekor, так что builder identity криптографически не подтверждается —
заявлять этот контроль означало бы утверждать то, чего нет (см. «Криптография
и TLS» выше и `scripts/sign_flags_test.sh`, запрещающий transparency-log
upload).
