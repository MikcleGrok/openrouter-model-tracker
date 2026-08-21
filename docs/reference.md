# Справочник

Локальная разработка, полный список команд и Makefile-таргетов, семантика
ранжирования, релиз-процесс и файлы, которые правятся руками, — для
мейнтейнера и контрибьюторов. Общее описание проекта и установка — в
[README.md](../README.md).

- [Локальная разработка](#локальная-разработка)
- [Onboarding record](#onboarding-record)
- [Команды](#команды)
  - [Семантика score, quality и tier](#семантика-score-quality-и-tier)
  - [Bash completion](#bash-completion)
- [Makefile](#makefile)
  - [Семантика Q/P и utility](#семантика-qp-и-utility)
  - [Настройка mixed utility](#настройка-mixed-utility)
  - [Offline local release](#offline-local-release)
- [Что правится руками](#что-правится-руками)

### Локальная разработка

Этот раздел — не для обычной установки, а для мейнтейнера и контрибьюторов
этого репозитория: локальный disposable tap без публикации на GitHub,
используемый только внутри существующего checkout.

Формула `openrouter` должна быть закреплена на том же exact release tag и
immutable commit revision, что и checkout проекта. Источник синхронизации не
хранит старую версию в Makefile или скрипте:

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

## Onboarding record

| Поле | Значение |
| --- | --- |
| `project type` | Go CLI/TUI, read-only data refresh tool; публикуемый release-бинарник |
| `profiles` | `active`: plain CLI/TUI, build/release и supply-chain; `N/A`: daemon, container runtime |
| `OS/ARCH` | CI и release: `linux/amd64`; локальная distribution-проверка: macOS/Homebrew; cross-platform matrix не заявлена |
| `modes` | локальная работа без credentials; CI PR/push; exact-tag release с GitHub Release и static-key provenance |
| `channels` | active: GitHub Release binary/evidence; local-only: Homebrew formula в disposable tap; `N/A`: опубликованный Homebrew tap и container image |
| `version source` | release version только из clean checkout на exact `vMAJOR.MINOR.PATCH` tag; обычная сборка использует `git describe`; formula синхронизирует tag и revision |
| `Makefile targets` | baseline: `fmt-check`, `test-unit`, `test-acceptance`, `vet`, `security`, `dependency-check`, `secrets-check`, `sbom`, `check-docs`; release: `release-check`, `release-manifest`, `sign`, `attest`, `verify-provenance`, `checksums`, `verify-release` |
| `Docker toolchain image` | `N/A`: Docker toolchain не используется и не публикуется |
| `Docker runtime image` | `N/A`: контейнерный runtime не поставляется |
| `Docker runtime base image` | `N/A`: отсутствует shipped runtime image |
| `host-only exceptions` | `HOST-ONLY`: Homebrew reinstall/verification (`brew`, maintainer; macOS package-manager integration); CI Linux не утверждает этот канал |
| `owners` | maintainer: repository owner; release/security: maintainer |
| `SCA cadence` | weekly для publishable profile, а также каждый PR и перед каждым release |
| `SCA owner` | maintainer |
| `remediation deadline` | critical/high: 7 календарных дней; остальные findings: 30 календарных дней |
| `N/A controls/rationale` | Docker/container controls: `N/A`, контейнер не поставляется; published Homebrew tap verification: `N/A`, tap disposable и не публикуется; native macOS CI: `N/A`, release builder Linux, macOS покрывается локальным Homebrew gate |
| `last reviewed` | 2026-08-10 |
| `review trigger/profile state` | active; пересмотр при изменении release channel, version source, signing/provenance, trust boundary или не позднее 2026-11-08 |

## Команды

### Семантика score, quality и tier

`Benchmark score` — сырое наблюдение с явными `metric`, `value`, `unit`, `source`,
`measured variant` и датой проверки. SWE-bench Verified хранится в процентах;
LMArena хранится отдельно как сырой Elo. Arena Elo не сравнивается с процентом
SWE-bench и не подменяет его.

`Capability estimate` / Claude tier — ручная Claude-relative оценка из
`model-map.tsv`. Она не выводится из score, score не выводится из tier и не
является измеренным quality. `Quality` и `quality>=` означают только
rankable exact-product benchmark observation.

Перед публикацией score проходит source-family-aware structured identity gate.
Для SWE-bench/vals: если строка нашлась по ключу, сопоставленному в `model-map.tsv`
(`vals=`/`swebench=`), это сопоставление и есть утверждение об идентичности — по
умолчанию оно доверяется, даже если ключ текстово не совпадает со slug'ом
OpenRouter (разные пространства имён и написание на двух сайтах — обычное дело,
а не признак другого продукта). Для vals.ai доверие дополнительно перепроверяется:
строка сама эхом возвращает ключ, по которому её нашли (`measured variant`), и
несовпадение с сопоставленным ключом — это не разница в написании, а подделанная
или устаревшая строка, которая остаётся `variant_mismatch`. У swebench.com такого
поля для проверки нет (там `measured variant` — имя скаффолда сабмишена, а не эхо
ключа), поэтому его сопоставление доверяется напрямую, без встречной проверки.
Если сопоставленная строка на самом деле измеряет другой чекпоинт/вариант, а не
тот же продукт, — человек обязан явно пометить это маркером `!variant` на имени
источника в `model-map.tsv`, например `vals!variant=deepseek/deepseek-v4-flash-0731`:
deepseek/deepseek-v4-flash сопоставлена с vals.ai-ключом чекпоинта `0731`, а
каталожный canonical_slug модели содержит дату `20260423`, так что без маркера
это был бы тихий `exact_product` на самом деле другого чекпоинта. Помеченная
строка остаётся `variant_mismatch` и не ранжируется, несмотря на найденное
совпадение по ключу; ловит такое расхождение только человек, редактирующий файл —
код это не проверяет автоматически ни для vals, ни тем более для swebench.com.
Строка без сопоставления в `model-map.tsv` для этого источника по-прежнему
проверяется буквально против исходных идентификаторов OpenRouter
(slug/canonical_slug) — без каких-либо cross-namespace догадок.
Для Arena `modelKey` и configured `arena=` из `model-map.tsv` принадлежат отдельному
namespace и не сравниваются с OpenRouter slug/canonical_slug: exact возможен только
при непустом и совпадающем configured/source Arena key. Несовпадение доступного
provider, release/model variant, reasoning или configuration также блокирует score;
отсутствующая, неоднозначная или неполная Arena identity получает `missing_identity`
или `legacy_unknown` и не допускается в ranking. Cross-namespace догадок нигде нет
сами по себе — единственное место, где имя с одного сайта принимается за имя с
другого, это явное, курируемое человеком сопоставление `vals=`/`swebench=` в
`model-map.tsv`, описанное выше; Arena identity и любая несопоставленная строка
по-прежнему сверяются буквально. Старое snapshot без статуса получает
`legacy_unknown`; `exact_product` никогда не принимается только из входного status
и всегда пересчитывается.

Каждое наблюдение сохраняет raw value, metric, unit, source URL, measured
variant, checked date, identity status, uncertainty/error bar, sample size,
harness/scaffold, provider и configuration. Если источник поле не публикует,
сохраняется явное `н/д` при отображении и пустой/missing marker в snapshot, а
не молчаливое удаление provenance. Snapshot fallback помечается `stale` и
`[snapshot fallback]`, включая Arena metadata/source URL/license/model URL.

`notes.yaml` хранит ручное поле `copyright_guardrail` со значениями `enforces`,
`bypasses` и `unknown`. Оно описывает поведение модели при попытке обойти
ограничения на защищённый контент: соблюдение ограничений, готовность помогать
обходить их или отсутствие проверяемого результата соответственно. Пропущенное
значение считается `unknown`; лицензия модели для этого поля не используется.

В таблице `Score` показывает raw benchmark number и короткое состояние; Q/P
равен только `valid benchmark quality / mixed price`, а при mismatch, legacy,
missing или ручном observation-only значении равен `н/д` и сортируется последним.
`utility` — отдельная value heuristic: она может учитывать manual tier premium,
но не меняет quality и не делает invalid score rankable. Detail view показывает
metric, unit, source/provenance, measured variant, identity status и manual tier.

- `openrouter init [--config PATH] [--data-dir PATH]` — создать пользовательский конфиг и локальный каталог кэша; существующие пути не изменяются
- `openrouter refresh|update|up [--output PATH] [--dry-run]` — собрать данные и перезаписать документ
- `openrouter check` — только отчёт, без записи; кроме ручной карты показывает
  изменения полного каталога OpenRouter с момента последнего успешного `refresh`
- `openrouter history [--model SLUG] [--since RFC3339|YYYY-MM-DD] [--format markdown|tsv]` — показать историю цен
- `openrouter tui [--filter FILTER]` — интерактивная таблица; `f` открывает редактор структурированного фильтра и применяет его сразу, а подтверждённый custom-фильтр сохраняется в `tui_filter` пользовательского `config.yaml` и загружается при следующем запуске. Если `tui_filter` отсутствует, пуст или равен legacy `has-q/p`, используется текущий `default_filter` (по умолчанию `quality>=75,has-q/p,availability:paid`), и его effective-поля показываются в редакторе. Явный `--filter` имеет приоритет над сохранённым и default-значением; `--filter=` намеренно отключает все фильтры. Очистка фильтра в редакторе удаляет persisted override, поэтому после reload снова применяется default. Изменение `default_filter` в конфиге подхватывается следующим auto-refresh TUI.
- В TUI источник оценки переключается прямо на основном экране клавишей `Space`; альтернативно нажмите `o`, стрелкой `Down` перейдите на `Score source`, затем нажмите `Space`. Это переключает `SWE-bench` и `Arena`. На основном списке `Enter` открывает подробную страницу модели. Текущий источник виден в meta-строке, Settings и status hints.
- Клавиши TUI можно переопределить в том же YAML-конфиге через `tui_keymap`. Контексты и действия проверяются отдельно, поэтому одинаковый `space` допустим в Settings, фильтре и выборе колонок. Binding может быть строкой или списком:

  ```yaml
  tui_keymap:
    main:
      open_settings: o
      open_details: [enter, right, l]
      help: ['?']
      full_help: f1
      navigate_up: [up, k]
      navigate_down: [down, j]
      switch_source: space
    settings:
      close: [esc, o]
      navigate_up: [up, k]
      navigate_down: [down, j]
      switch_source: space
    detail:
      close: [esc, left, h]
      navigate_up: [up, k]
      navigate_down: [down, j]
  ```

  Доступны также контексты `help`, `columns` и `filter` с действиями `close`, `full_help`, `navigate_up`, `navigate_down`, `toggle` и `apply` по смыслу контекста. Неизвестные действия/контексты, пустые bindings и повтор одного binding для разных действий в одном контексте дают ошибку конфига. При reload TUI эта секция перечитывается вместе с `default_filter`, `tui_filter` и `tui_steps`; до успешной загрузки snapshot источник оценки не меняется, а Settings показывает pending или ошибку.
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
make openrouter-launchd-refresh-check
make openrouter-launchd-refresh-install
make openrouter-launchd-refresh-status
make openrouter-launchd-refresh-start
make openrouter-launchd-refresh-uninstall
make release-check VERSION=1.0.0
make release-local
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
Если нужно исключить stale binary и проверить CLI end-to-end, используйте полный gate
`make test-acceptance`, а не прямой запуск acceptance-тестов.
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
снимок из `model-snapshot.json` (версионированный файл в корне каталога данных, рядом с
`model-map.tsv`). Они не обращаются к сети и завершаются с ошибкой,
если локальный снимок отсутствует.

Проверка distribution metadata делегирована в `../guide-tools/bin/guide-distribution-verify`
через `scripts/verify-distribution.sh`; путь можно переопределить через `GUIDE_TOOLS_ROOT`.

По умолчанию таблица сортируется в явном режиме `utility` с ranking `mixed-utility`: сначала идут rankable-модели, а платные модели сравниваются по безопасной YAML expression formula. Без `formula` действует value heuristic формула `score + price_weight*tier_factor*ln(1+quality_price)`, где `price_weight=10`, price mix равен 3:1, а факторы `opus=1`, `sonnet=1`, `haiku=0.5`, `free=0`. Она не меняет quality и не делает invalid observation rankable. Бесплатные rankable-модели сравниваются по score. `--sort` принимает `utility`, `name`, `slug`,
`context`, `input`, `output`, `price`, `quality`, `q/p` и `utility`, а также `Q`, `P`, `QP`; явный `q/p` всегда сортирует по показанному raw Q/P независимо от `--ranking`, а `utility` использует выбранный ranking. `Q` означает valid quality по убыванию, `P` — price по возрастанию, `QP` — q/p по убыванию. Фильтры: `paid`, `free`, `scored`, `tier:*`, `quality>=N`, `context>=N`, `input<=N`, `output<=N`. Фильтры объединяются через AND: например, `openrouter table --filter 'paid,quality>=80' --filter 'tier:sonnet'`. Операторы: `:` для выбора значения, `>=` для минимального порога и `<=` для максимального. `quality` использует только valid exact-product observation активного источника и шкалу `0..100`; mismatch, legacy и observation-only исключаются. Для удобства допускается дробный формат `0..1`, поэтому `quality>=0.8` эквивалентен `quality>=80`. Качество и Q/P сортируются по убыванию, `--reverse`/`-R` инвертирует основной порядок, а отсутствующие или неранжируемые значения всегда остаются в конце. Затем применяется `--limit`, поэтому `-n 1` выбирает первую модель уже отсортированного результата. `--reverse` меняет основной порядок, slug
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
`price_weight` и tier-факторы остаются теми же) — у модели с минимальным Elo в наборе
Q/P может оказаться ровно `0.0`, независимо от её цены. Диапазон
нормализации зависит от текущего набора моделей, поэтому, в отличие от Q/P на основе
SWE-bench, Arena-based Q/P не стабилен между прогонами. Генерируемый
`docs/openrouter-model-comparison.md` всегда собирается в семантике `swebench`.

### Семантика Q/P и utility

Показанный Q/P и `sort:q/p` используют только canonical raw metric:
`raw_qp = valid_benchmark_quality / mixed_price`. Tier не входит в quality и не
может создать Q/P для mismatch, legacy, observation-only, missing или free
модели: такие строки получают `н/д` и остаются последними. `sort:utility`
отдельно может использовать `score + price_weight*tier_factor*ln(1+raw_qp)` как
value heuristic; это не quality и не benchmark score. Для `source=arena` quality
внутри Arena view строится из normalized Arena Elo, тогда как raw Elo остаётся
отдельно подписанным `Elo`.

### Настройка mixed utility

Иконки производителей CLI и TUI настраиваются в пользовательском YAML без пересборки:

```yaml
icons:
  manufacturers:
    openai: '🌀'
    anthropic: '🔶'
    google: '🌐'
    meta: 'Ⓜ️'
    deepseek: '🐋'
    qwen: '🌸'
    mistral: '🌪️'
    xai: '🚀'
    xiaomi: 'ⓧ'
    nvidia: 'Ⓝ'
    z.ai: 'Ⓩ'
    minimax: '♟️'
    moonshot: '🌙'
    tencent: '🐧'
  unknown: '❔'
```

Ширина первой колонки `Name` настраивается в той же секции `table` и применяется
к CLI и TUI:

```yaml
table:
  name_width: 40
  icon_gap: 1
  icon_gaps: {meta: 0, mistral: 3}
```

По умолчанию используется 40 display columns. Значения от 1 до 120 принимаются;
нулевые, отрицательные, слишком большие и некорректные значения безопасно
возвращаются к default. В TUI фактическая ширина дополнительно ограничивается
текущим viewport, поэтому узкие терминалы не переполняются.

`table.icon_gap` задаёт отступ после фиксированного слота иконки производителя
до его имени и применяется одинаково в CLI и TUI. Ширина слота по умолчанию
составляет 2 display columns, поэтому узкие glyphs получают padding внутри
слота, а сам glyph не изменяется. По умолчанию это 1 пробел; принимаются
значения от 0 до 8. Некорректные, отрицательные и слишком большие значения
возвращаются к 1. Между именем производителя и названием модели всегда остаётся
один пробел.

Ключи производителей нормализуются так же, как входное имя: обрезаются, пробелы
схлопываются, регистр игнорируется, а совпадение остаётся substring-поиском.
`table.icon_gaps` переопределяет gap только после фиксированного слота
(`icon slot -> manufacturer`);
граница `manufacturer -> model`, padding ячейки и `table.name_width` не меняются.
По умолчанию все производители, включая Meta и Mistral, используют глобальный
`icon_gap` (1). Значения override также ограничены диапазоном 0..8;
некорректный override возвращается к эффективному глобальному gap, поэтому его
можно переопределить обратно в 1 или задать любое другое значение.
Arena organization используется только при verified Arena identity; иначе сохраняется
fallback на Owner и Provider. Пустые или содержащие управляющие символы icons
игнорируются: для известного производителя используется его default, для unknown
используется `❔`. Изменение секции применяется при следующем запуске CLI/TUI.

Фильтр по умолчанию: `quality>=75,has-q/p,availability:paid`. Дополнительно поддерживаются predicates `has-q/p` и `availability:any|free|paid`.

Default filter для TUI настраивается отдельно и читается при запуске и auto-refresh:

```yaml
default_filter: quality>=75,has-q/p,availability:paid
```

Шаги числовых полей редактора фильтра настраиваются без пересборки бинарника:

```yaml
tui_steps:
  quality_points: 5 # percentage points
  context_tokens: 8192 # integer tokens per step
  input_cents: 5 # cents per $/M per step ($0.05 by default)
  output_cents: 5 # cents per $/M per step ($0.05 by default)
```

Все значения должны быть неотрицательными целыми; отсутствующие или нулевые ключи
получают defaults `5/8192/5/5` (Input/Output по `$0.05`). `Context minimum` показывается целым числом
токенов, а Input/Output всегда показываются и сохраняются канонически с двумя
знаками после запятой. Шаг цены всегда равен настроенному числу cents, поэтому
переходы `$0.99 -> $1.00` и `$9.99 -> $10.00` не пропускаются.

Старые ключи `quality`, `context`, `input`, `output` остаются принимаемыми только
если в `tui_steps` нет новых ключей; они сохраняют прежнюю процентную семантику.
Смешивание старых и новых ключей отклоняется с ошибкой. Новый init-шаблон старые
ключи не создаёт. Конфиг перечитывается при каждом auto-refresh TUI
вместе с `default_filter`, поэтому изменение `tui_steps` применяется без перезапуска.

`tui_filter` имеет приоритет над `default_filter`, если это непустой custom-фильтр. Пустой `tui_filter` и legacy-значение `has-q/p` мигрируют на текущий `default_filter`; произвольные custom-значения не перезаписываются. CLI `--filter` имеет наивысший приоритет, включая намеренно пустой `--filter=`. Effective default показывается в форме своими полями, а не как `(any)`; очистка формы удаляет persisted override. Остальные hardcoded значения интерфейса и фильтрации (например, хоткеи, набор колонок, разрешённые имена predicates, ограничения ranking expression и semantic thresholds) не выносились.

Операционные defaults можно менять в конфиге без пересборки. Относительный `cache.dir` разрешается относительно `data_dir`; CLI flags имеют приоритет над config:

```yaml
cache:
  dir: cache
  ttl: 12h
  request_timeout: 30s
table:
  sort: q/p
  ranking: mixed
  score_source: swebench
  limit: 0
  task_fit: short
tui:
  refresh_interval: 5m
  sort: q/p
  ranking: mixed
  score_source: swebench
  limit: 0
```

`cache.ttl`, `cache.request_timeout` и `tui.refresh_interval` используют формат Go duration (`30s`, `12h`, `0s`). `tui.refresh_interval` читается при запуске; уже работающий TUI, как и раньше, перечитывает на auto-refresh только `default_filter` и `tui_steps`. Нулевой `limit` означает отсутствие ограничения. Невалидные duration и отрицательные limits отклоняются при загрузке конфигурации.

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
отображается как `n/a`.
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

`make refresh` явно изменяет данные checkout: обновляет `cache/`, версионированный
`model-snapshot.json` и генерируемый `docs/openrouter-model-comparison.md`. Для проверки
гонок используется `make race`.

LaunchAgent управляется через Make без необходимости запоминать путь к скрипту:
`make openrouter-launchd-refresh-install`, `make openrouter-launchd-refresh-uninstall`,
`make openrouter-launchd-refresh-status` и `make openrouter-launchd-refresh-start`. Эти цели передают
операцию в `scripts/launchd-refresh.sh`; `openrouter-launchd-refresh-check` использует только
dry-run и не вызывает `launchctl`. Все `OPENROUTER_*` переменные можно задать перед
Make-командой, например:

```bash
OPENROUTER_CONFIG="$HOME/.config/openrouter/config.yaml" \
OPENROUTER_DATA_DIR="$HOME/.local/share/openrouter" \
make openrouter-launchd-refresh-install
```

`make release-local` (алиас `make local-release`) включает в каждый archive бинарник
и runtime-скрипты `scripts/launchd-refresh.sh` и `scripts/cron-refresh.sh` с mode
`0755`. `scripts/launchd-refresh_test.sh` остаётся только в исходном checkout и
намеренно не входит в runtime package. Local-release проверяет наличие обоих скриптов,
отсутствие тестового скрипта и executable mode до вычисления checksum. GitHub Release
публикует те же archives вместе с binary/provenance artifacts. LaunchAgent использует
`cron-refresh.sh` как единственную refresh-логику, поэтому отдельного дублирующего
расписания в package не создаётся.

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

### Offline local release

`make release-local` (алиас `make local-release`) — канонический offline-flow для
уже созданного release tag. Сначала он требует clean checkout ровно на exact
`vMAJOR.MINOR.PATCH` tag через `check-tag`, затем запускает существующие
форматирование, тесты, vet, security, secrets и documentation checks. После этого
собираются deterministic `CGO_ENABLED=0` Go-бинарники для
`darwin/arm64`, `darwin/amd64`, `linux/amd64` и `linux/arm64`; список можно
переопределить через `LOCAL_RELEASE_PLATFORMS`.

Результат сохраняется в persistent каталоге
`dist/local-release/<version>/`: архивы `tar.gz`, `SHA256SUMS`,
`RELEASE_NOTES.md` и `manifest.json` с version, tag, commit, artifact, sha256 и
UTC `built_at`. Каталог локальный и игнорируется Git. Команда не создаёт и не
двигает tags, не меняет remote, не вызывает GitHub/GitLab, `gh`, API, Homebrew,
signing keys или secrets. Homebrew остаётся отдельным disposable local flow
через `sync-homebrew-formula` и `homebrew-reinstall`.

Для проверки на существующем теге:

```bash
make release-local
```

Если checkout dirty, detached от exact tag или в CHANGELOG отсутствует exact
version section с notes, команда завершается blocker до сборки артефактов.
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

Настройки layout TUI сохраняются в `tui.layout` и `tui.top_n` (`all`, `top-paid-free`; default `top_n: 3`). В `top-paid-free` сначала выполняются фильтрация и сортировка, затем выбирается до `top_n` платных и бесплатных строк, substring search работает внутри этого результата, а глобальный `tui.limit` ограничивает объединённый список. Явные `availability:any|free|paid` и legacy-предикаты `free`/`paid` ограничивают соответствующие секции; `has-q/p` применяется к обеим секциям и исключает бесплатные строки без Q/P. Чтобы effective default с `availability:paid` не скрывал бесплатную секцию, только этот default в top mode трактуется как `availability:any`; custom `default_filter` и `tui_filter` соблюдаются буквально. `p` переключает availability, `v` переключает layout.

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

`openrouter tui` открывает интерактивную локальную таблицу из последнего snapshot. Команда работает только в TTY и поддерживает те же сортировки (`name`, `slug`, `context`, `input`, `output`, `price`, `quality`, `q/p`, включая `q`, `p`, `qp`), `--sort`, `--reverse`, `--filter`, `--limit`, `--slug` и `--score-source`, что и `table`. Клавиша `/` выполняет отдельный substring search по Name/Slug, а `f` принимает только structured-фильтры (`paid`, `free`, `scored`, `tier:*`, `quality>=N`, `context>=N`, `input<=N`, `output<=N`); ошибочный structured-фильтр не меняет строки и показывается в status. Также поддерживаются выбор колонок (`c`), переключение Task fit/Note (`n`), ручное обновление (`R`) и справка (`?`). Все односимвольные хоткеи TUI работают и при русской раскладке клавиатуры: клавиша распознаётся по своей физической позиции через явную таблицу соответствия символов, заданную литералами в коде, — ни `locale`, ни `LANG`, ни системные настройки раскладки не читаются, а расширенные keyboard protocol (например Kitty) не используются, поэтому char-level таблица — единственный работающий в этом проекте механизм fallback. Встроенная справка при этом остаётся полностью английской и локализованных подписей клавиш не содержит. Task fit в TUI показывается только компактными кодами. Последнюю колонку нельзя снять. Клавиши `Enter`, `→` или `l` открывают экран деталей выделенной модели — производитель, дата релиза (абсолютная и относительная), тир, контекст, полная цена вместе с длинноконтекстным тарифом, обе оценки отдельными подписанными блоками (SWE-bench Verified в процентах и LMArena в Elo, никогда не смешиваются), task fit, ручная заметка и вендорское описание с переносом по словам; `Esc`, `←` или `h` возвращают к списку на ту же строку, а `↑↓`, `PgUp/PgDown` и `Home/End` внутри экрана прокручивают текст. На экране деталей также показаны ссылка на карточку модели на `openrouter.ai` (строится из `canonical_slug` каталога) и — только у моделей, у которых он есть, — ссылка на репозиторий HuggingFace; подписи полей, заголовки блоков, ссылки и `н/д` выделены цветом, но раскладка при этом не меняется ни на один символ. `--refresh-interval 0` отключает автоматическое обновление, ручное `R` остаётся доступным. Для локального запуска достаточно `data_dir`; `default_output` нужен только для live refresh.

Клавиша `o` открывает окно Settings: в нем можно переключить ranking и score source, отредактировать текущий structured filter и увидеть выбранные колонки. Смена score source читает локальный snapshot и не требует сети. `?` открывает только справку по горячим клавишам, а `F1` — полный help, включая горячие клавиши.

Справка TUI занимает весь viewport и прокручивается клавишами ↑/↓ (или `j`/`k`), `Home`/`End` (или `g`/`G`) и `PgUp`/`PgDown`; `/` открывает поиск по тексту справки: набираемый запрос сразу виден отдельной строкой `/ …_` внизу экрана, как в основном списке, а найденные вхождения выделяются цветом в тексте справки. `Enter` переходит к следующему совпадению, а `Esc` или `?` закрывают открытый help. В верхней строке TUI показывается RFC3339-время последнего успешного обновления данных.

Для cron доступен `scripts/cron-refresh.sh`. Он считает новый операционный день с 06:00 по локальному времени и не запускает `refresh`, если snapshot уже успешно обновлён в этот день. `updated_at` используется как точное время последней публикации; старые snapshot без этого поля поддерживаются через `fetched_at`.

Для macOS доступен пользовательский LaunchAgent без root-доступа. Он запускает тот же
`cron-refresh.sh` каждые 15 минут через `StartInterval=900`; `RunAtLoad=false`. Первый 15-minute tick
после 06:00 по local time выполняет refresh, если snapshot ещё не обновлялся в текущий
операционный день. Последующие ticks до следующего 06:00 пропускаются той же проверкой
snapshot/cutoff в `cron-refresh.sh`. Если Mac спал в момент запуска, launchd может выполнить
missed job при следующей возможности; проверка snapshot всё равно не позволит выполнить
больше одного refresh за операционный день.
Параметры сохраняются в plist, поэтому job не зависит от текущего каталога:

```bash
./scripts/launchd-refresh.sh install
./scripts/launchd-refresh.sh status
./scripts/launchd-refresh.sh start  # ручной запуск сейчас
./scripts/launchd-refresh.sh uninstall
```

Установка идемпотентна: перед `bootstrap` существующая версия job отключается через
`bootout`, поэтому дубликаты не остаются. Plist устанавливается в
`~/Library/LaunchAgents/com.openrouter.model-tracker.refresh.plist`, логи пишутся в
`~/Library/Logs/openrouter-refresh.log`. Значения по умолчанию для бинарника, конфига,
data directory и output совпадают с cron workflow: `bin/openrouter` в checkout,
`~/.config/openrouter/config.yaml`, root checkout и `docs/openrouter-model-comparison.md`.
Их можно задать при установке через `OPENROUTER_BIN`, `OPENROUTER_CONFIG`,
`OPENROUTER_DATA_DIR`, `OPENROUTER_OUTPUT`; значения с пробелами поддерживаются.
Для проверки plist без launchd используйте `OPENROUTER_LAUNCHD_DRY_RUN=1 ... install`
или `make openrouter-launchd-refresh-check`. `validate` проверяет сгенерированный plist через
`plutil`.

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

`refresh` сохраняет в `model-snapshot.json` (версионированный файл в корне каталога данных,
не в `cache/`) полный набор slug'ов каталога как
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
Кэшированные файлы из `cache/` (HTTP cache, `price-history.json`) не добавляются в Git.
`model-snapshot.json` — версионированные вычисленные данные о моделях (цены, контекст, оценки,
provenance) — трекается в Git и лежит в корне репозитория, рядом с `model-map.tsv`.
