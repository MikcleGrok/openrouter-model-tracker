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

- `openrouter refresh [--output PATH] [--dry-run]` — собрать данные и перезаписать документ
- `openrouter check` — только отчёт, без записи; кроме ручной карты показывает
  изменения полного каталога OpenRouter с момента последнего успешного `refresh`
- `openrouter history [--model SLUG] [--since RFC3339|YYYY-MM-DD] [--format markdown|tsv]` — показать историю цен
- `openrouter version`
- `openrouter --version` — показать версию бинарника

Конфиг по умолчанию — `~/.config/openrouter/config.yaml` (`data_dir`, `default_output`).

Версия по умолчанию — `dev`. Для release-сборки из checkout, содержащего полный git tag,
используй точную инъекцию тега через Go ldflags:

```bash
VERSION="$(git describe --tags --exact-match)" && go build -ldflags "-X main.version=${VERSION}" -o ./bin/openrouter ./cmd/openrouter
```

Например, при checkout на теге `v0.1.0` ожидаемый вывод:

```text
$ ./bin/openrouter --version
openrouter version v0.1.0
```

Локальная Homebrew formula находится вне этого репозитория и не получает git tag автоматически,
поэтому её обычная сборка может показать `dev`. Безопасный вариант — сначала собрать бинарник
командой выше на нужном полном теге, а затем использовать его как release-артефакт; формулу вне
репозитория не изменяй для настройки этой инъекции.

## Что правится руками

- `model-map.tsv` — какие slug'и отслеживаются, к какому тиру Claude отнесены и под
  каким **точным** именем модель известна каждому источнику. Никакого fuzzy-match:
  нет записи — нет оценки.
- `notes.yaml` — вся проза документа: примечания по моделям, обоснования фаворитов,
  таблица FLI, справочные цены Claude, оговорки, вручную заданные вендорские оценки.

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
