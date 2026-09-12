# openrouter-model-tracker

CLI/TUI для сравнения AI-моделей на OpenRouter по качеству и цене.

![TUI: таблица моделей, переключение источника оценки SWE-bench/Arena/GPQA, карточка модели с 4 вкладками, фильтр paid/free](docs/assets/tui-demo.gif)

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
openrouter tui                                     # интерактивный TUI
openrouter table                                    # та же таблица как plain-text (без сети, для скриптов)
openrouter refresh                                   # обновить цены и оценки
openrouter history --model <slug> --format report     # история цен по одной модели
openrouter check                                       # что изменилось в каталоге/карте, без записи
```

(после установки через Homebrew те же команды — через `omt` или
`openrouter-model-tracker`)

## Подробнее

- [docs/methodology.md](docs/methodology.md) — identity gate, три независимых измерения качества, формула ранжирования
- [docs/reference.md](docs/reference.md) — полный список команд, конфиг, Makefile-таргеты, релиз-процесс
- [docs/security.md](docs/security.md) — supply-chain profile и подписывание релиза
- [CHANGELOG.md](CHANGELOG.md) — история изменений
