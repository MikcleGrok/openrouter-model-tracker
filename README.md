# openrouter-model-tracker

CLI/TUI для сравнения AI-моделей на OpenRouter по качеству и цене.

![TUI: фильтр доступности paid/any/free, поиск моделей Claude, переключение источника оценки SWE-bench/Arena/GPQA, экран справки по хоткеям, карточка модели](docs/assets/tui-demo.gif)

`openrouter-model-tracker` собирает живой каталог моделей OpenRouter (цены,
контекст) и сопоставляет его с независимыми оценками качества — SWE-bench
Verified (vals.ai и swebench.com), LMArena Elo и GPQA Diamond (vals.ai) — через
ручную curated-карту `model-map.tsv` и structured identity gate, а не
fuzzy-match по имени. Платные модели ранжируются по метрике «качество/цена» и
раскладываются по тирам, ориентированным на Claude Opus/Sonnet/Haiku. Данные
доступны в интерактивном TUI, как plain-text CLI-таблица и как готовый
Markdown-отчёт.

## Installation

**macOS**:

```bash
brew install mikclegrok/tools/openrouter-model-tracker
omt tui
```

**Linux** (amd64; для arm64 замените `amd64` на `arm64`):

```bash
curl -L https://github.com/MikcleGrok/openrouter-model-tracker/releases/download/v1.19.0/openrouter-1.19.0-linux-amd64.tar.gz \
  | sudo tar xzf - -C /usr/local/bin --transform 's,.*,openrouter-model-tracker,' \
  && sudo ln -sf /usr/local/bin/openrouter-model-tracker /usr/local/bin/omt
omt tui
```

**Windows 10/11** (amd64; на ARM работает через встроенную x64-эмуляцию) —
через [Scoop](https://scoop.sh):

```powershell
scoop bucket add mikclegrok https://github.com/MikcleGrok/scoop-bucket
scoop install mikclegrok/openrouter-model-tracker
omt tui
```

Через [winget](https://learn.microsoft.com/windows/package-manager/winget/):

```powershell
winget install MikcleGrok.openrouter-model-tracker
openrouter-model-tracker tui
```

winget разрешает только один alias на установщик, поэтому здесь нет
короткого `omt` — только полное имя.

Короткий alias `omt` доступен везде, кроме сборки вручную и winget-установки.
Другие способы установки (сборка из исходников, ручная установка без
пакетного менеджера) — в [docs/reference.md](docs/reference.md).

## Usage

```bash
omt tui                        # интерактивный TUI
omt table                      # та же таблица как plain-text (без сети, для скриптов)
omt refresh                    # обновить цены и оценки
omt report                     # перегенерировать Markdown/HTML-документ офлайн, без сети
omt report --refresh           # то же, но сначала обновить данные (как omt refresh)
omt history                    # история цен
omt check                      # что изменилось в каталоге/карте, без записи
```

(также можно использовать полное имя `openrouter-model-tracker`)

## Example Report

- [Markdown](docs/examples/openrouter-model-comparison.md) — пример вывода `omt report`
- [HTML](https://htmlpreview.github.io/?https://raw.githubusercontent.com/MikcleGrok/openrouter-model-tracker/master/docs/examples/openrouter-model-comparison.html) — тот же отчёт в самостоятельном HTML

## Feedback & Ideas

Нашли баг, хотите новую функцию или просто есть идея — welcome:

- **Идеи и пожелания** — [GitHub Discussions](https://github.com/MikcleGrok/openrouter-model-tracker/discussions/categories/ideas), не обязательно быть программистом.
- **Баг или конкретный запрос функции** — [Issues](https://github.com/MikcleGrok/openrouter-model-tracker/issues/new/choose).

## Learn More

- [docs/methodology.md](docs/methodology.md) — identity gate, три независимых измерения качества, формула ранжирования
- [docs/reference.md](docs/reference.md) — полный список команд, конфиг, Makefile-таргеты, релиз-процесс
- [docs/contributing.md](docs/contributing.md) — процесс приёмки изменений: ветка, PR с историей, гейты, ревью, merge
- [docs/security.md](docs/security.md) — supply-chain profile и подписывание релиза
- [CHANGELOG.md](CHANGELOG.md) — история изменений
