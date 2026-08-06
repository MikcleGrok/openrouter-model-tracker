# openrouter-model-tracker

Пересобирает `docs/openrouter-model-comparison.md` в репозитории bobash из живых
данных: цены и контекст — с публичного OpenRouter API, оценки SWE-bench Verified —
со swebench.com и vals.ai.

## Установка и обновление

Локальный tap, без GitHub:

```bash
brew install --HEAD local/tap/openrouter
brew reinstall local/tap/openrouter   # подхватить свежий коммит
```

Формула: `$(brew --repository)/Library/Taps/local/homebrew-tap/Formula/openrouter.rb`.

## Команды

- `openrouter init [--config PATH] [--data-dir PATH]` — создать пользовательский конфиг и локальный каталог кэша; существующие пути не изменяются
- `openrouter refresh|update|up [--output PATH] [--dry-run]` — собрать данные и перезаписать документ
- `openrouter check` — только отчёт, без записи; кроме ручной карты показывает
  изменения полного каталога OpenRouter с момента последнего успешного `refresh`
- `openrouter history [--model SLUG] [--since RFC3339|YYYY-MM-DD] [--format markdown|tsv]` — показать историю цен
- `openrouter table [-s|--sort KEY] [-S|--slug] [-R|--reverse] [-n|--limit N] [-f|--filter FILTER] [--task-fit=short|long] [--notes] [--no-pager]` — показать локальные данные моделей в plain-text таблице без Markdown и сети. По умолчанию показывается короткая колонка `Task fit`; `--task-fit=long` выводит полные keywords, а `--notes` возвращает прежнюю колонку `Note`. `--notes` нельзя смешивать с `--task-fit`. `-n N` оставляет первые `N` моделей после сортировки; standalone `-N` является shorthand для `-n N` (`-1`, `-20`), а `-0` и `-n 0` означают отсутствие лимита. Фильтр можно повторять, фильтры объединяются через AND.
- `openrouter version`
- `openrouter --version` — показать версию бинарника

## Makefile

Основные локальные цели:

```bash
make help
make build
make test
make vet
make fmt-check
make check
make history
make table
make init
```

`openrouter table` и `make table` читают `model-map.tsv`, `notes.yaml` и последний локальный
снимок из `cache/last-run-snapshot.json`. Они не обращаются к сети и завершаются с ошибкой,
если локальный снимок отсутствует.

По умолчанию таблица сортируется по `q/p` по убыванию. `--sort` принимает только `name`, `slug`,
`context`, `input`, `output`, `price`, `quality` и `q/p`, а также `Q`, `P`, `QP`; `Q` означает quality по убыванию, `P` — price по возрастанию, `QP` — q/p по убыванию. Фильтры: `paid`, `free`, `scored`, `tier:*`, `quality>=N`, `context>=N`, `input<=N`, `output<=N`. Качество сортируется по убыванию, `--reverse`/`-R` инвертирует основной порядок, а отсутствующие или неранжируемые quality всегда остаются в конце. Затем применяется `--limit`, поэтому `-n 1` выбирает первую модель уже отсортированного результата. `--reverse` меняет основной порядок, slug
остаётся детерминированным tie-breaker. В интерактивном TTY вывод передаётся в `less -S`, если
не указан `--no-pager`. При перенаправлении в pipe или файл pager не запускается.
Колонка `Task fit` рассчитывается по полной display width самого длинного значения и не
обрезается; если таблица шире терминала, `less -S` позволяет прокручивать её по горизонтали,
а pipe или файл сохраняют полный текст.
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
`docs/openrouter-model-comparison.md`. Для проверки гонок используется `make race`,
а `make release-build` собирает бинарник только из checkout на полном git-теге.
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
Шаблон конфига не содержит machine-specific абсолютных путей:

```yaml
data_dir: .
default_output: docs/openrouter-model-comparison.md
```

`model-map.tsv`, `notes.yaml` и `ignore-candidates.txt` остаются входными файлами checkout и
не создаются командой `init`. Повторный запуск сообщает `Already exists` и не перезаписывает
конфиг. Команда не обращается к сети и не создаёт snapshot, price history, HTTP cache или
сгенерированный документ.

Версия сборки берётся из `git describe --tags --always --dirty` и передаётся в Go через
`-X main.version`. Поэтому checkout на точном теге показывает чистую версию, а commit после
тега — стандартный suffix `-N-g<sha>`; изменённый checkout дополнительно получает `-dirty`.
Для release-сборки `make release-build` требует чистый checkout ровно на полном git-теге:

```bash
make release-build
```

Например, при checkout на теге `v0.1.0` ожидаемый вывод:

```text
$ ./bin/openrouter --help
Version: v0.1.0

openrouter collects prices and context from the public OpenRouter API, and scores from swebench.com
and vals.ai using the manual model-map.tsv mapping, then regenerates the markdown document.

$ ./bin/openrouter --version
openrouter version v0.1.0
```

В обычной локальной сборке root help показывает текущее значение `git describe`, включая
suffix для commit после тега или `-dirty` для изменённого checkout.

Локальная Homebrew formula находится вне этого репозитория и во время сборки вычисляет ту же
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
дубликаты удаляются. Короткий вывод, например, `I+T`; длинный — `implement + test`.
Пустой список выводится как `n/a` и не означает плохое качество: это означает отсутствие
классификации.

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
