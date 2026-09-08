# openrouter-model-tracker

CLI/TUI для сравнения AI-моделей на OpenRouter по качеству и цене.

`openrouter-model-tracker` собирает живой каталог моделей OpenRouter (цены,
контекст) и сопоставляет его с независимыми оценками качества — SWE-bench
Verified (vals.ai и swebench.com) и LMArena Elo, — затем ранжирует платные
модели по метрике «качество/цена» и раскладывает их по тирам, ориентированным
на Claude Opus/Sonnet/Haiku. Сопоставление строк с разных сайтов проходит через
ручную карту `model-map.tsv` и structured identity gate, а не fuzzy-match по
имени: нет записи в карте — нет оценки. Данные доступны и в интерактивном TUI,
и как plain-text CLI-таблица, и как готовый Markdown-отчёт.

## Скриншоты

Короткий тур по TUI: главный экран с ранжированной таблицей → переключение
источника оценки (`Space`, SWE-bench/Arena) → карточка модели → переключение
фильтра доступности (`p`, paid/free) на бесплатные модели — без выхода из
приложения:

![TUI: главная таблица, переключение SWE-bench/Arena, карточка модели, переключение на бесплатные модели](docs/assets/tui-demo.gif)

## Возможности

- Три независимых источника данных: цены и контекст — из публичного
  OpenRouter API; качество — SWE-bench Verified (vals.ai, swebench.com) и
  LMArena Elo, независимая оценка отдельно от вендорской.
- Ранжирование платных моделей по метрике «качество/цена» (mixed-utility) с
  тирами относительно Claude Opus/Sonnet/Haiku; настраиваемая ranking-формула.
- Интерактивный TUI (`openrouter tui`): сортировка, structured-фильтры,
  детальная карточка модели, переключение источника оценки (SWE-bench/Arena) и
  доступности (paid/free) прямо в интерфейсе.
- Тот же движок как plain-text CLI-таблица (`openrouter table`) — без сети, для
  скриптов и пайпов.
- Настраиваемые фильтры, ranking-формула, иконки производителей, хоткеи и шаги
  редактора фильтра — всё через пользовательский `config.yaml`, без пересборки.
- История цен (`openrouter history`) и отчёт об изменениях каталога (`openrouter
  check`) поверх того же локального снимка.

## Методология

Каждая строка таблицы собрана из трёх независимых источников: живая цена и
контекст из каталога OpenRouter, независимый benchmark score (SWE-bench
Verified с vals.ai/swebench.com или LMArena Elo — никогда оба сразу) и
ручной Claude-relative tier. Строка с лидерборда попадает в оценку модели
только через явное сопоставление в `model-map.tsv` — никогда по похожести
имён, — а платные модели ранжируются по «качество/цена» с настраиваемой
value-формулой. Полное описание identity-gate, трёх измерений качества и
формулы ранжирования — в [docs/methodology.md](docs/methodology.md).

## Установка

Для macOS и Linux — публичный Homebrew tap:

```bash
brew install mikclegrok/tools/openrouter-model-tracker
```

Homebrew устанавливает canonical `openrouter` и короткий alias `omt` как один
и тот же executable. Обновление до новой версии:

```bash
brew upgrade mikclegrok/tools/openrouter-model-tracker
```

Без Homebrew, или на других платформах, — штатная локальная установка через
Makefile (нужен Go 1.26.5 или новее, см. `go.mod`):

```bash
git clone https://github.com/MikcleGrok/openrouter-model-tracker.git
cd openrouter-model-tracker
make install
```

По умолчанию canonical бинарник устанавливается как `/usr/local/bin/openrouter`,
а управляемый symlink `/usr/local/bin/omt` указывает на него. Поэтому `omt`
и `openrouter` всегда имеют одинаковую версию. Каталог
можно изменить без username-specific путей: `PREFIX` задаёт корень, а при
отсутствии явного `BINDIR` используется `$PREFIX/bin`. Явный абсолютный
`BINDIR` является самостоятельным target и может находиться вне `PREFIX`.
`VERSION` по
умолчанию равен точной версии tag checkout или `0.0.0-dev` для checkout без
exact tag; при явном `VERSION` это значение внедряется в бинарник и полностью
проверяется до атомарной замены. `TARGET` позволяет выбрать Go package для сборки.

При установке или переустановке через локальную Homebrew formula Bash completion
генерируется и устанавливается автоматически:

```bash
brew install bash-completion@2
brew install --HEAD local/tap/openrouter
brew reinstall local/tap/openrouter   # обновить бинарник и completion
```

Один раз убедитесь, что установленный `bash-completion@2` загружает каталог completion
Homebrew при старте Bash. Например, добавьте в `~/.bash_profile`:

```bash
if [ -r "$(brew --prefix)/etc/profile.d/bash_completion.sh" ]; then
  source "$(brew --prefix)/etc/profile.d/bash_completion.sh"
fi
```

На Apple Silicon startup script обычно находится в `/opt/homebrew/etc/profile.d/bash_completion.sh`.
На Intel macOS и Linux путь может отличаться; используйте значение, которое возвращает
`brew --prefix`. После первоначальной настройки перезапустите shell или загрузите этот
startup script вручную. Последующая установка или переустановка обновляет completion-файл,
но не изменяет уже запущенную shell-сессию автоматически.

Для постоянной установки на Linux от имени пользователя:

```bash
make install PREFIX="$HOME/.local" BINDIR="$HOME/.local/bin"
make upgrade PREFIX="$HOME/.local" VERSION=1.15.0
make reinstall PREFIX="$HOME/.local"
make uninstall PREFIX="$HOME/.local"
make install-smoke
```

`install`, `upgrade` и `reinstall` собирают бинарник во временный каталог и пишут
canonical binary и alias в один `BINDIR`; временные файлы удаляются после завершения. `PREFIX` и
конечный `BINDIR` обязательны, непусты и должны быть абсолютными; явный `BINDIR`
независим от `PREFIX` и может находиться вне него. На один `BINDIR` берётся bounded
lock, поэтому параллельные установки сериализуются, а ожидание завершается ошибкой
через 60 секунд. Все проверки source/version/help/target/path выполняются до замены.
После preflight binary и symlink заменяются атомарно под одним lock; alias создаётся только как `omt -> openrouter`. Существующий regular `omt`, directory или symlink на другой объект считается unmanaged: install отклоняется и ничего не удаляет. Для documented migration принимается также только Homebrew-owned symlink вида `../Cellar/openrouter/<числовая-версия>/bin/omt` (или тот же target с абсолютным prefix), если его target исполняемый; произвольные symlink и dangling symlink не принимаются. Sidecar marker обновляется отдельно под lock с rollback при ошибке, где это возможно; installer
`$(BINDIR)/openrouter.openrouter-owner` имеет mode 600 и содержит identifier, точный destination
и версию. `uninstall` удаляет binary только при валидном marker с совпадающим destination;
отсутствующий marker оставляет файл и возвращает WARN с exit 0, невалидный marker оставляет
оба объекта и возвращает ошибку.
Все компоненты destination проверяются на symlink traversal по canonical path.
На exact tag грязный checkout с release `VERSION` отклоняется, чтобы изменённый
исходный код не выдавался за release.
`uninstall` никогда не удаляет немаркированный или Homebrew-managed файл
`$(BINDIR)/openrouter` и сохраняет unmanaged/mismatched `omt`. `install-smoke` использует
временный PREFIX, проверяет `--version`, `version` и `--help`, и не изменяет
системную установку. Homebrew остаётся отдельным внешним каналом: `make
verify-release` и `make homebrew-reinstall` проверяют локальный disposable tap,
но local installer от Homebrew не зависит. Symlink rejection является best-effort:
lock с уникальным owner token уменьшает concurrent race; cleanup удаляет lock только при
совпадении token, но shell-проверки TOCTOU не защищают от злонамеренной
замены каталога между проверкой и операцией.

После клонирования полезно выполнить `make install-hooks` — это включает pre-commit
проверку, которая блокирует коммит приватных ключей и credentials в отслеживаемых файлах.

## Exit codes

`openrouter` гарантирует только `0` (успех, включая `--help`/`-h`/`help`) и
non-zero (любой отказ — usage-ошибка, сетевая ошибка, ошибка записи и т.д.);
отдельных документированных кодов для разных классов отказа нет. Это
осознанный выбор, а не недосмотр: см. man-страницу (`man/openrouter.1`,
`EXIT STATUS`) и acceptance-тест `TestE2E_InvalidCommandWritesErrorToStderr`.

## Документация

Локальная разработка, полный список команд, Makefile-таргеты, релиз-процесс и файлы, которые правятся руками, — в [docs/reference.md](docs/reference.md).

## Локальный release и external signing

Канонический локальный flow не требует `COSIGN_PRIVATE_KEY`: `make release-check`
и `make release-local` проверяют сборку, checksum, SBOM и локальные артефакты.
`PROVENANCE_PROFILE=local` (значение по умолчанию) и `candidate` печатают короткий
`NOT APPLICABLE`, завершаются с кодом 0 и не вызывают `cosign`, не создают
signed/provenance или published evidence.

Подписывание и публикация относятся к отдельному внешнему профилю:
`PROVENANCE_PROFILE=external` (также принимается `published`). Он fail-closed:
без cosign, public key или полного набора evidence команда завершается с
ненулевым кодом. Read-only verification не требует `COSIGN_PRIVATE_KEY`;
приватный ключ нужен только для `make sign` и `make attest`. Оба профиля
проходят один и тот же полный verification path, включая `cmd/evidencecheck`.
`codesign` identity `uni-release-selfsign` не является cosign
ключом, secret `openrouter-model-tracker/cosign-key` не создаётся. Тег
`v1.14.37` этим профилем не объявляется подписанным и не перепривязывается.

## Onboarding record

Этот раздел — канонический location record guide-tools compliance-метаданных
для этого проекта; task document его не заменяет.

| Поле | Значение |
| --- | --- |
| `project type` | Go CLI/TUI, read-only data refresh tool; публикуемый release-бинарник |
| `profiles` | `active`: plain CLI/TUI, build/release и supply-chain, **и** publishable (см. `channels`) — оба применимы одновременно, publishable задаёт более строгую SCA cadence; `active`: concurrency-heavy — реальные goroutines в `internal/refresh/run.go` (parallel benchmark-source fetch, `sync.WaitGroup`), поэтому `race` — заявленный conditional target (`make race`, 05-build-test-docs.md «Gates», «race при concurrency»), включённый в `test-all`; `-race` проверяет data races и MUST NOT считаться доказательством отсутствия memory leak или resource retention (12-test-contract.md); `N/A`: daemon, container runtime |
| `OS/ARCH` | Заявленный OS/ARCH matrix (реально собирается и публикуется через `make release-local`/`LOCAL_RELEASE_PLATFORMS`): `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64` — как GitHub Release assets, так и Homebrew asset-channel formula. CI отсутствует полностью (см. `N/A controls/rationale`, native macOS CI); обычная разработческая сборка — host native (macOS, локально). Matrix заявлена, а не «не заявлена»: darwin — реальный declared и shipped target, поэтому 14-cross-platform-ci.md's macOS-разделы применимы, а не `N/A`. |
| `modes` | локальная работа без credentials; отсутствие CI (push/PR/schedule) по деliberate local-only design (см. CHANGELOG: workflows добавлены, затем удалены при переходе на local-only release); exact-tag release с GitHub Release и static-key provenance |
| `channels` | active, machine-verified из этого checkout (`make distribution-check`, `archive` profile, post-tag): GitHub Release binary/evidence в `MikcleGrok/openrouter-model-tracker`, exact `vMAJOR.MINOR.PATCH` tag; active, asset-channel exception (guide-tools/README.md, «Канонический источник бинарников», precedent: `uni-chat`), verified вручную вне этого checkout: канонический Homebrew tap `mikclegrok/tools`, formula `openrouter-model-tracker.rb` — asset-channel Formula (per-platform `url`+`sha256` на immutable release-asset URL, не source `:git` build), фактически установлена и слинкована на машине мейнтейнера (`brew info mikclegrok/tools/openrouter-model-tracker`), версия совпадает с последним релизом; её asset host/tag (`MikcleGrok/tools`, tag `openrouter-model-tracker-vX.Y.Z`) отличается от self-repo GitHub Release channel выше — известное расхождение, см. `docs/security.md`; local-only: disposable source-build tap (`local/homebrew-tap`, `check-homebrew-formula`) для bounded install-flow smoke, не production source; `N/A`: container image |
| `version source` | release version только из clean checkout на exact `vMAJOR.MINOR.PATCH` tag; обычная сборка использует `git describe`; formula синхронизирует tag и revision |
| `Makefile targets` | baseline: `check` (SCA-freshness staleness gate), `fmt-check`, `test-unit`, `test-acceptance`, `vet`, `security`, `dependency-check`, `secrets-check`, `sbom`, `check-docs`; conditional: `race` (concurrency-heavy, part of `test-all`); `man-check`, `completion-check` (applicable CLI, part of `check` and `release-check`); domain: `cli-check`; release: `release-check`, `release-manifest`, `sign`, `attest`, `verify-provenance`, `checksums`, `verify-release`, `distribution-check` |
| `Docker toolchain image` | `N/A`: Docker toolchain не используется и не публикуется |
| `Docker runtime image` | `N/A`: контейнерный runtime не поставляется |
| `Docker runtime base image` | `N/A`: отсутствует shipped runtime image |
| `host-only exceptions` | `HOST-ONLY`: Homebrew reinstall/verification (`brew`, maintainer; macOS package-manager integration); отсутствие CI означает, что ни один контур этот канал не утверждает автоматически |
| `shared-location scoping` | `фиксированное перечисление owned paths`. Config/cache — эксклюзивно свои пути (`--config`'s dir, `<data_dir>/cache/`), не shared location. Install/upgrade/reinstall/uninstall в общем `$(BINDIR)` касаются только `$(BINDIR)/openrouter`, managed symlink `$(BINDIR)/omt` и sidecar-маркера `$(BINDIR)/openrouter.openrouter-owner` (mode 600); `uninstall` отказывается трогать unmarked/mismatched destination (WARN+exit 0 либо error, но всегда preserve) вместо сканирования каталога. `make clean` удаляет только `bin/openrouter`. Owned set перечислен в `scripts/install.sh` и Makefile-таргетах `install`/`uninstall`/`clean`; сохранность постороннего файла в том же `$(BINDIR)` доказывает `scripts/install_test.sh` (unmanaged `omt`, foreign symlink, foreign lock owner — все preserved). |
| `owners` | maintainer: repository owner; release/security: maintainer |
| `SCA cadence` | weekly (`SCA_CADENCE_DAYS=7` — publishable profile, не 30-дневный plain-CLI дефолт), а также каждый PR и перед каждым release. Механизм — staleness gate по свежести SCA-evidence в `make check`/`make release-check` (`scripts/check-sca-freshness.sh`; канонический, самодостаточен, внешнего триггера не требует). Evidence location: `.release/dependency-evidence.json` (не коммитится; материализуется `make dependency-check`). |
| `SCA owner` | maintainer |
| `remediation deadline` | critical/high: 7 календарных дней; остальные findings: 30 календарных дней |
| `N/A controls/rationale` | Docker/container controls: `N/A`, контейнер не поставляется; published Homebrew tap verification: не `N/A` — asset-channel exception (см. `channels`), self-repo GitHub Release channel machine-verified через `make distribution-check`, отдельная `mikclegrok/tools` formula верифицирована вручную (см. `channels`); native macOS CI: не `N/A` — exception. `darwin/arm64`/`darwin/amd64` являются реально заявленными и публикуемыми targets (см. `OS/ARCH`), поэтому это не genuinely inapplicable control, а принятый residual risk: CI отсутствует полностью (ни Linux, ни macOS) по deliberate local-only release design, а не выборочно для macOS. Compensating control: owner mickle (mickle.grok@proton.me) собирает и smoke-тестирует оба macOS-arch локально на своей машине (`make release-local`, `make homebrew-reinstall`, `make verify-release`) перед каждым релизом; проверяется при каждом `last reviewed`, автоматического контроля нет |
| `last reviewed` | 2026-09-08 |
| `review trigger/profile state` | active; пересмотр при изменении release channel, version source, signing/provenance, trust boundary или не позднее 2026-12-07 |
