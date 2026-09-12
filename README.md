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

## Установка

**macOS** — публичный Homebrew tap (бинарник ставится как
`openrouter-model-tracker`, короткий alias — `omt`):

```bash
brew install mikclegrok/tools/openrouter-model-tracker
```

**Linux** (amd64/arm64) — бинарник из
[GitHub Releases](https://github.com/MikcleGrok/openrouter-model-tracker/releases):

```bash
curl -LO https://github.com/MikcleGrok/openrouter-model-tracker/releases/download/v1.18.7/openrouter-1.18.7-linux-amd64.tar.gz
tar xzf openrouter-1.18.7-linux-amd64.tar.gz
sudo install -m 0755 openrouter-1.18.7-linux-amd64 /usr/local/bin/openrouter
```

(для arm64 замените `amd64` на `arm64`; актуальная версия — на странице Releases)

**Windows 10/11** (amd64; на ARM-устройствах работает через встроенную
x64-эмуляцию) — zip-архив из
[GitHub Releases](https://github.com/MikcleGrok/openrouter-model-tracker/releases),
в PowerShell:

```powershell
Invoke-WebRequest https://github.com/MikcleGrok/openrouter-model-tracker/releases/download/v1.18.7/openrouter-1.18.7-windows-amd64.zip -OutFile openrouter.zip
Expand-Archive openrouter.zip -DestinationPath .
Move-Item openrouter-1.18.7-windows-amd64.exe openrouter.exe
.\openrouter.exe tui
```

(чтобы запускать просто `openrouter`, положите `openrouter.exe` в любой каталог из
`PATH`; TUI рассчитан на Windows Terminal. Сборка из исходников:
`go build -o openrouter.exe ./cmd/openrouter`, нужен [Go](https://go.dev) 1.26.5+;
бинарник не подписан, поэтому при первом запуске SmartScreen покажет
предупреждение — это ожидаемо, жмите «Подробнее» → «Выполнить в любом случае»)

На macOS и Linux доступна и сборка из исходников без Homebrew:
`git clone ... && cd ... && make install` — подробности (`PREFIX`/`BINDIR`,
локальный disposable tap для разработки) в
[docs/reference.md](docs/reference.md).

## Использование

```bash
omt tui                        # интерактивный TUI
omt table                      # та же таблица как plain-text (без сети, для скриптов)
omt refresh                    # обновить цены и оценки
omt history                    # история цен
omt check                      # что изменилось в каталоге/карте, без записи
```

(также можно использовать полное имя `openrouter-model-tracker`)

## Подробнее

- [docs/methodology.md](docs/methodology.md) — identity gate, три независимых измерения качества, формула ранжирования
- [docs/reference.md](docs/reference.md) — полный список команд, конфиг, Makefile-таргеты, релиз-процесс
- [docs/security.md](docs/security.md) — supply-chain profile и подписывание релиза
- [CHANGELOG.md](CHANGELOG.md) — история изменений
