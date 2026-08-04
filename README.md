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
- `openrouter check` — только отчёт, без записи
- `openrouter version`

Конфиг по умолчанию — `~/.config/openrouter/config.yaml` (`data_dir`, `default_output`).

## Что правится руками

- `model-map.tsv` — какие slug'и отслеживаются, к какому тиру Claude отнесены и под
  каким **точным** именем модель известна каждому источнику. Никакого fuzzy-match:
  нет записи — нет оценки.
- `notes.yaml` — вся проза документа: примечания по моделям, обоснования фаворитов,
  таблица FLI, справочные цены Claude, оговорки, вручную заданные вендорские оценки.

Сам `.md` — билд-артефакт. Правки в нём не переживут следующий прогон.
