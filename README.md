# openrouter-model-tracker

Пересобирает `docs/openrouter-model-comparison.md` в репозитории bobash из живых
данных: цены и контекст — с публичного OpenRouter API, оценки SWE-bench Verified —
со swebench.com и vals.ai.

## Установка и обновление

Локальный disposable tap, без GitHub. Формула `openrouter` должна быть закреплена
на том же exact release tag и immutable commit revision, что и checkout проекта.
Источник синхронизации не хранит старую версию в Makefile или скрипте:

```bash
TAP_FORMULA="$(brew --repository)/Library/Taps/local/homebrew-tap/Formula/openrouter.rb"
make sync-homebrew-formula
make homebrew-reinstall
```

`make sync-homebrew-formula` требует exact `vMAJOR.MINOR.PATCH` tag и вычисляет
его commit через Git. Он атомарно меняет только `tag` и `revision` в локальной
формуле; tap не публикуется и remote не изменяется. Для read-only проверки:

```bash
make check-homebrew-formula
```

`make homebrew-reinstall` сначала синхронизирует формулу, затем выполняет
`brew reinstall --build-from-source`, проверяет `brew list`, `openrouter --version`
и `brew test`. Для первой установки вместо reinstall используйте:

```bash
make sync-homebrew-formula
brew install --formula --build-from-source "$TAP_FORMULA"
```

Этот workflow является distribution contract: после exact tag он проверяет tag и
immutable revision formula до любой reinstall. Stable install не использует
`--HEAD`, branch или hardcoded old version. Если локального tap или formula
нет, `make check-homebrew-formula` завершается blocker вместо проверки
случайного текущего checkout.

Формула: `$(brew --repository)/Library/Taps/local/homebrew-tap/Formula/openrouter.rb`.

## Команды

- `openrouter init [--config PATH] [--data-dir PATH]` — создать пользовательский конфиг и локальный каталог кэша; существующие пути не изменяются
- `openrouter refresh|update|up [--output PATH] [--dry-run]` — собрать данные и перезаписать документ
- `openrouter check` — только отчёт, без записи; кроме ручной карты показывает
  изменения полного каталога OpenRouter с момента последнего успешного `refresh`
- `openrouter history [--model SLUG] [--since RFC3339|YYYY-MM-DD] [--format markdown|tsv]` — показать историю цен
- `openrouter table [-s|--sort KEY] [-S|--slug] [-R|--reverse] [-n|--limit N] [-f|--filter FILTER] [--task-fit=short|long] [--notes] [--no-pager] [--score-source=swebench|arena]` — показать локальные данные моделей в plain-text таблице без Markdown и сети. По умолчанию показывается короткая колонка `Task fit`; `--task-fit=long` выводит полные keywords, а `--notes` возвращает прежнюю колонку `Note`. `--notes` нельзя смешивать с `--task-fit`. `-n N` оставляет первые `N` моделей после сортировки; standalone `-N` является shorthand для `-n N` (`-1`, `-20`), а `-0` и `-n 0` означают отсутствие лимита. Фильтр можно повторять, фильтры объединяются через AND.
- `openrouter completion bash` — сгенерировать Bash completion
- `openrouter version`
- `openrouter --version` — показать версию бинарника

Версия release-бинарника является нормализованным SemVer 2.0.0 без префикса `v`.
Единственный источник release-версии — чистый checkout на exact immutable tag
`vMAJOR.MINOR.PATCH` с optional prerelease (`-rc.1`); build metadata (`+...`) запрещена.
Обычная локальная сборка по-прежнему показывает descriptive version от `git describe`.

### Bash completion

Для текущей shell-сессии:

```bash
source <(openrouter completion bash)
```

Для постоянной установки в macOS Homebrew:

```bash
brew install bash-completion@2
mkdir -p "$(brew --prefix)/etc/bash_completion.d"
openrouter completion bash > "$(brew --prefix)/etc/bash_completion.d/openrouter"
BREW_PREFIX="$(brew --prefix)"
printf '%s\n' 'if [ -r "$(brew --prefix)/etc/profile.d/bash_completion.sh" ]; then' '  source "$(brew --prefix)/etc/profile.d/bash_completion.sh"' 'fi' >> "$HOME/.bash_profile"
source "$BREW_PREFIX/etc/profile.d/bash_completion.sh"
```

На Apple Silicon `brew --prefix` обычно равен `/opt/homebrew`, поэтому startup script
находится в `/opt/homebrew/etc/profile.d/bash_completion.sh`. На Intel macOS и Linux этот
путь может отличаться; используйте значение, которое возвращает `brew --prefix`.
Перезапустите shell или перезагрузите конфигурацию `bash-completion`, чтобы файл был подключён.

Для постоянной установки на Linux от имени пользователя:

```bash
mkdir -p "$HOME/.local/share/bash-completion/completions"
openrouter completion bash > "$HOME/.local/share/bash-completion/completions/openrouter"
```

Каталог `~/.local/share/bash-completion/completions` должен загружаться вашей установкой
`bash-completion`. Универсальная альтернатива — добавить в `~/.bashrc` одну строку:

```bash
source <(openrouter completion bash)
```

Completion включает команды и flags, в том числе `tui`, `table`, `--task-fit`, `--sort` и `--filter`.

## Makefile

Основные локальные цели:

```bash
make help
make build
make test
make test-unit
make test-acceptance
make test-all
make vet
make fmt-check
make check
make history
make table
make init
make version
make check-version
make check-tag
make check-homebrew-formula
make release-check VERSION=1.0.0
make verify-release
make whats-new
make security
make dependency-check
make secrets-check
make sbom
make checksums
make verify-provenance
make signature
make check-docs
```

Acceptance-тест версии использует `OPENROUTER_EXPECTED_VERSION`, который `make test` и
`make release-check` передают из `VERSION`; при прямом запуске `go test ./tests/...` используется
локальное dev-значение `0.0.0-dev`.

`test-unit` запускает быстрый набор тестов пакетов `internal/...` и `cmd/...` с
отключённым test cache. `test-acceptance` сначала собирает `bin/openrouter`, затем
запускает black-box проверки `tests/run/acceptance` через реальный process boundary.
`test-all` объединяет оба набора. Сетевой `refresh` намеренно не входит в быстрый
CI-гейт: функциональные тесты источников используют `httptest`, а production-like
прогон требует отдельного контролируемого окружения.

Makefile является единственным публичным интерфейсом build/test/security/release
действий и не зависит от текущего каталога. `dependency-check` и `sbom` требуют
внешние scanners и завершаются ошибкой при их отсутствии; это не скрытые NO-OP.
Dependency evidence использует строгую схему v2: статусы `blocked`, `error`,
`partial` и `passed` не смешиваются, а запись содержит findings, policy decision,
digest входных файлов, metadata инструментов/базы и native outputs.
`verify-provenance` и `signature` имеют явный локальный NO-OP, потому что
репозиторий не публикует артефакты и не содержит CI builder или signing identity.
Подробный scope находится в `docs/security.md`.

`openrouter table` и `make table` читают `model-map.tsv`, `notes.yaml` и последний локальный
снимок из `cache/last-run-snapshot.json`. Они не обращаются к сети и завершаются с ошибкой,
если локальный снимок отсутствует.

Проверка distribution metadata делегирована в `../guide-tools/bin/guide-distribution-verify`
через `scripts/verify-distribution.sh`; путь можно переопределить через `GUIDE_TOOLS_ROOT`.

По умолчанию таблица сортируется в режиме `mixed-utility`: сначала идут rankable-модели, а платные модели сравниваются по безопасной YAML expression formula. Без `formula` действует совместимая формула `score + price_weight*tier_factor*ln(1+quality_price)`, где `price_weight=10`, price mix равен 3:1, а факторы `opus=1`, `sonnet=1`, `haiku=0.5`, `free=0`. Бесплатные rankable-модели сравниваются по score. `--sort` принимает только `name`, `slug`,
`context`, `input`, `output`, `price`, `quality` и `q/p`, а также `Q`, `P`, `QP`; `Q` означает quality по убыванию, `P` — price по возрастанию, `QP` — q/p по убыванию. Фильтры: `paid`, `free`, `scored`, `tier:*`, `quality>=N`, `context>=N`, `input<=N`, `output<=N`. Качество сортируется по убыванию, `--reverse`/`-R` инвертирует основной порядок, а отсутствующие или неранжируемые quality всегда остаются в конце. Затем применяется `--limit`, поэтому `-n 1` выбирает первую модель уже отсортированного результата. `--reverse` меняет основной порядок, slug
остаётся детерминированным tie-breaker. В интерактивном TTY вывод передаётся в `less -S`, если
не указан `--no-pager`. При перенаправлении в pipe или файл pager не запускается.
Колонка `Task fit` рассчитывается по полной display width самого длинного значения и не
обрезается; если таблица шире терминала, `less -S` позволяет прокручивать её по горизонтали,
а pipe или файл сохраняют полный текст.
Для рейтинга моделей можно явно выбрать `--ranking=legacy`, `--ranking=tier` или `--ranking=mixed`.
`legacy` сохраняет прежнюю сортировку по Q/P и включается только явно. `tier` использует
лексикографический ключ `rankable, tier, score, Q/P, price`: Opus существенно выше Sonnet/Haiku,
а цена учитывается только после качества внутри тира. `mixed` использует глобальную tier-adjusted utility
`score + price_weight*tierPriceFactor*ln(1+Q/P)`; `price_weight` берётся из `ranking.mixed_utility.price_weight` и по умолчанию равен `10`:
абсолютное качество остаётся главным, но цена заметно влияет на близкие результаты. `task_fit`
не участвует в формуле и не является multiplier. Без `--ranking` используется `mixed-utility`.
В TUI `m` переключает `tier-priority` и `mixed-utility`, а
текущий режим показывается в верхней meta-строке.

Источник оценки выбирается отдельно от режима ранжирования:
`--score-source=swebench` (по умолчанию) — SWE-bench Verified в процентах,
`--score-source=arena` — рейтинг Elo с `arena.ai/leaderboard/text`. Это два полностью
независимых представления: в режиме `arena` модель без Arena-строки показывает `н/д`,
даже если у неё есть настоящий SWE-bench-счёт, и наоборот. Значения `auto` нет — числа
двух источников никогда не смешиваются в одной колонке. Elo показывается сырым
(`1453 Elo`), а после min-max нормализации в 0–100 по текущему набору Arena-моделей
попадает и в формулу ранжирования, и в показанную колонку «Качество/цена» (так что
`price_weight` и tier-факторы остаются теми же) — у самой дешёвой модели набора Q/P
может оказаться ровно `0.0`, потому что её Elo стал минимумом диапазона. Диапазон
нормализации зависит от текущего набора моделей, поэтому, в отличие от Q/P на основе
SWE-bench, Arena-based Q/P не стабилен между прогонами. Генерируемый
`docs/openrouter-model-comparison.md` всегда собирается в семантике `swebench`.

### Настройка mixed utility

```yaml
ranking:
  mixed_utility:
    price_weight: 10
    price: {input_weight: 3, output_weight: 1}
    tier_factors: {opus: 1, sonnet: 1, haiku: 0.5, free: 0, default: 0}
    formula:
      op: add
      args:
        - var: score
        - op: mul
          args:
            - var: tier_factor
            - op: log1p
              args: [{var: quality_price}]
```

`formula` и `price_weight` нельзя указывать одновременно: конфигурация завершается ясной ошибкой вместо выбора одной из неоднозначных семантик. Разрешены vars `score`, `input_price`, `output_price`, `price_mix`, `quality_price`, `tier_value`, `tier_factor` и операции `add`, `sub`, `mul`, `div`, `neg`, `log`, `log1p`, `min`, `max`, `clamp`. Arity строгая, глубина максимум 16, узлов максимум 64. Все числа finite, price weights неотрицательны и имеют положительную сумму. Неизвестные ключи/vars/ops, деление на ноль и ошибки домена `log`/`log1p` отклоняются.
Колонка `Claude` использует ручной `tier` из `model-map.tsv` как источник соответствующей ссылки.
Для `opus` выводится `>≈ Opus 5`, для `sonnet` — `≈ Sonnet 5`; для `haiku` и `free`
с rankable Score применяются пороги 70 и 60 относительно `Claude Haiku 4.5`, а без score используются
fallback `≈ Haiku 4.5` для `haiku` и `<≈ Haiku 4.5` для `free`. Неизвестный tier
отображается как `н/д`.
`Claude` и `Task fit` всегда выводятся полными значениями и могут сделать таблицу шире даже при
`COLUMNS=40`; в интерактивном режиме `less -S` позволяет прокручивать её горизонтально, а pipe
или файл сохраняют полный текст. Compact fallback относится только к структурным колонкам
(`Name`/`Slug`, `Status`, `Q/P`, `Context` и ценам), а также к выбранной последней колонке (`Task fit` или `Note`), но не к `Claude`.
При обычной ширине терминала первая колонка имеет минимум 30 и максимум 40 display columns, а
`Q/P` — максимум 5 display columns; числовые значения сохраняются, длинные fallback labels
обрезаются по display width;
для узких терминалов для этих структурных колонок используется compact fallback. В pager mode
она не сохраняется сверх максимума; `less -S` по-прежнему позволяет горизонтально прокручивать
остальные колонки.

`make refresh` явно изменяет данные checkout: обновляет `cache/` и генерируемый
`docs/openrouter-model-comparison.md`. Для проверки гонок используется `make race`.

`make release-check VERSION=1.0.0` — непубликующий pre-tag gate: проверяет чистоту
checkout, формат release-версии, локальный commit SHA, diff hygiene, форматирование,
тесты, vet, security baseline, secret scan и Unreleased notes; candidate binary
собирается только для локальной version-проверки. Этот target не создаёт checksum,
manifest или published evidence. Exact tag для него не требуется; planned tag является
только metadata.
После создания exact tag сначала выполните `make sync-homebrew-formula` в локальном
tap. Затем `make release-build` требует чистый checkout ровно на
`vMAJOR.MINOR.PATCH` (с optional prerelease) и падает, если формула содержит другой
tag или revision. Локальная проверка strict evidence
и собранного бинарника выполняется отдельным `make verify-local-artifact`;
`make verify-release` read-only проверяет локальный stable Homebrew channel: exact
tag/version/commit, clean checkout и formula, установленную версию, оба CLI version
вывода и `brew test`. `file://` formula не является доказательством GitHub publication,
подписи или provenance; эти проверки остаются внешними.
Ни один из этих target не создаёт tag,
не публикует, не устанавливает пакет и не меняет remote.
Bootstrap-сценарий для macOS при запуске из корня checkout вызывается так:

```bash
./scripts/init.sh
make init
```

Из произвольного текущего каталога вызовите `<repo>/scripts/init.sh` или укажите
абсолютный путь к сценарию.

Сценарий определяет root checkout по своему расположению, собирает актуальный
`bin/openrouter`, затем вызывает безопасный `openrouter init` и `openrouter refresh`.
`init` только создаёт отсутствующие config/cache и не обращается к сети. `refresh`,
напротив, обращается к сети, обновляет кэш и domain data, а затем открывает готовый
отчёт через macOS `open`.

Пути можно переопределить переменными окружения:

```bash
OPENROUTER_CONFIG="$HOME/.config/openrouter/config.yaml" \
OPENROUTER_DATA_DIR="/path/to/project-data" \
OPENROUTER_OUTPUT="/path/to/report.md" \
<repo>/scripts/init.sh
```

`OPENROUTER_CONFIG` по умолчанию равен `~/.config/openrouter/config.yaml`,
`OPENROUTER_DATA_DIR` — root checkout, а `OPENROUTER_OUTPUT` —
`<root>/docs/openrouter-model-comparison.md`. Для CI/headless-режима отключите
открытие отчёта: `OPENROUTER_OPEN=0 ./scripts/init.sh`. Другим значением
`OPENROUTER_OPEN` можно заменить команду macOS `open`.

`make clean` удаляет только `bin/openrouter`; кэш и пользовательские данные не удаляются.

Конфиг по умолчанию — `~/.config/openrouter/config.yaml` (`data_dir`, `default_output`).

Инициализация из checkout создаёт только отсутствующие пути:

```bash
openrouter init
```

Будут созданы `~/.config/openrouter/config.yaml` и `<data_dir>/cache/`, если их ещё нет.
Шаблон конфига не содержит machine-specific абсолютных путей. Относительные
пути из config разрешаются относительно самого config-файла, поэтому запуск из
другого cwd не меняет runtime scope:

```yaml
data_dir: .
default_output: docs/openrouter-model-comparison.md
```

`model-map.tsv`, `notes.yaml` и `ignore-candidates.txt` остаются входными файлами checkout и
не создаются командой `init`. Повторный запуск сообщает `Already exists` и не перезаписывает
конфиг. Команда не обращается к сети и не создаёт snapshot, price history, HTTP cache или
сгенерированный документ.

Локальная descriptive версия берётся из `git describe --tags --always --dirty`, нормализуется
удалением только tag-префикса `v` и передаётся в Go через `-X main.version`. Поэтому checkout
на точном теге показывает SemVer без `v`, а commit после тега — descriptive suffix; изменённый
checkout дополнительно получает `-dirty`.

```bash
make release-build
```

Например, при checkout на теге `v0.1.0` ожидаемый вывод:

```text
$ ./bin/openrouter --help
Version: 0.1.0

openrouter collects prices and context from the public OpenRouter API, and scores from swebench.com,
vals.ai and arena.ai using the manual model-map.tsv mapping, then regenerates the markdown document.

$ ./bin/openrouter --version
openrouter version 0.1.0
```

В обычной локальной сборке root help показывает текущее значение `git describe`, включая
suffix для commit после тега или `-dirty` для изменённого checkout.

`make whats-new VERSION=1.0.0` печатает только exact section `## [1.0.0]` из
`CHANGELOG.md` и завершается ошибкой, если такой section отсутствует. `release-check`
отдельно использует `## [Unreleased]` как pre-tag candidate notes.

Локальная Homebrew formula находится вне этого репозитория. `make sync-homebrew-formula`
вычисляет tag и immutable revision из checkout, а во время сборки формула вычисляет ту же
версию через `git describe` из checkout в `buildpath`, поэтому Homebrew больше не подставляет
свой `HEAD-<sha>` в Go ldflags. Exact tag показывает чистую версию, а commit после тега —
describe suffix.

## Что правится руками

- `model-map.tsv` — какие slug'и отслеживаются, к какому тиру Claude отнесены и под
  каким **точным** именем модель известна каждому источнику. Никакого fuzzy-match:
  нет записи — нет оценки.
- `notes.yaml` — вся проза документа: примечания по моделям, обоснования фаворитов,
  таблица FLI, справочные цены Claude, оговорки, вручную заданные вендорские оценки и
  `task_fit` для рабочих задач.

`task_fit` — качественная taxonomy пригодности, а не score: `implement` (I), `plan` (P),
`research` (R), `debug` (D), `audit` (A), `refactor` (F), `test` (T). Порядок канонический,
дубликаты удаляются. Короткий вывод, например, `IT` или `IDFT` без плюсов; длинный — `implement + debug + refactor + test`.
Пустой список выводится как `n/a` и не означает плохое качество: это означает отсутствие
классификации.

`openrouter tui` открывает интерактивную локальную таблицу из последнего snapshot. Команда работает только в TTY и поддерживает те же сортировки (`name`, `slug`, `context`, `input`, `output`, `price`, `quality`, `q/p`, включая `q`, `p`, `qp`), `--sort`, `--reverse`, `--filter`, `--limit`, `--slug` и `--score-source`, что и `table`. Клавиша `/` выполняет отдельный substring search по Name/Slug, а `f` принимает только structured-фильтры (`paid`, `free`, `scored`, `tier:*`, `quality>=N`, `context>=N`, `input<=N`, `output<=N`); ошибочный structured-фильтр не меняет строки и показывается в status. Также поддерживаются выбор колонок (`c`), переключение Task fit/Note (`n`), ручное обновление (`r`) и справка (`?`). Task fit в TUI показывается только компактными кодами. Последнюю колонку нельзя снять. `--refresh-interval 0` отключает автоматическое обновление, ручное `r` остаётся доступным. Для локального запуска достаточно `data_dir`; `default_output` нужен только для live refresh.

В справке TUI разделы можно сразу выбрать клавишами `1`, `2` или `3`; справка занимает весь viewport. В верхней строке TUI показывается RFC3339-время последнего успешного обновления данных.

Для cron доступен `scripts/cron-refresh.sh`. Он считает новый операционный день с 06:00 по локальному времени и не запускает `refresh`, если snapshot уже успешно обновлён в этот день. `updated_at` используется как точное время последней публикации; старые snapshot без этого поля поддерживаются через `fetched_at`.

Безопасная установка для macOS добавляет только собственную строку и не удаляет существующие записи пользователя:

```bash
./scripts/cron-refresh.sh install
```

Команда идемпотентна. Она устанавливает почасовой запуск, поэтому пропуск одного запуска не откладывает обновление до следующего дня; скрипт сам не выполняет больше одного refresh за операционный день. Лог записывается в `~/Library/Logs/openrouter-refresh.log`.

Если нужен ручной вариант, для запуска каждый час:

```cron
0 * * * * /path/to/openrouter-model-tracker/scripts/cron-refresh.sh >> "$HOME/Library/Logs/openrouter-refresh.log" 2>&1
```

Пути можно переопределить переменными `OPENROUTER_CONFIG`, `OPENROUTER_DATA_DIR`, `OPENROUTER_OUTPUT` и `OPENROUTER_BIN`.

Сам `.md` — билд-артефакт. Правки в нём не переживут следующий прогон.

`refresh` сохраняет в `cache/last-run-snapshot.json` полный набор slug'ов каталога как
baseline. `check` сравнивает живой каталог с этим baseline, но не изменяет его, поэтому
один и тот же delta остаётся видимым до следующего успешного `refresh`; повторный
`refresh` фиксирует новый baseline и убирает уже обработанные сообщения. Старые snapshot-файлы
без `catalog_slugs` поддерживаются: первый `check` не считает весь текущий каталог новым,
а baseline появится после успешного обычного `refresh`. `ignore-candidates.txt` по-прежнему
фильтрует только кандидатов ручной карты и не скрывает изменения полного каталога.

`refresh` после успешного live lookup цен добавляет observation в отдельное versioned-хранилище
`cache/price-history.json`. Observation содержит UTC timestamp и числовые поля цены, контекст и
long-context override для каждого slug; история ограничена последними 365 наблюдениями.
`document`, `last-run snapshot` и `price-history` подготавливаются и публикуются через общий локальный
rollback-протокол: обычные ошибки откатываются, но аварийное завершение между отдельными `rename`
всё ещё может оставить смешанное поколение. Если подготовка history не удалась, основное состояние
не продвигается без соответствующего observation. Ошибки benchmark-источников не мешают сохранению цен, но fallback-цены,
`check` и `refresh --dry-run` observation не создают. `check` сравнивает live-цены с последним
сохранённым observation и остаётся read-only; в выводе видны base input/output/context и long-context
threshold/override input/output, включая изменения только override-полей. Если истории нет, изменение
цен не показывается.

`check` и `refresh --dry-run` не изменяют domain snapshot, `price-history.json` и generated document.
При этом HTTP cache может обновиться из-за сетевого чтения.

Например: `openrouter history --model openai/gpt-5.6-luna --since 2026-08-01 --format tsv`.
Кэшированные файлы из `cache/` не добавляются в Git.
