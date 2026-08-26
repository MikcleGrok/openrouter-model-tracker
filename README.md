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
brew install MikcleGrok/openrouter/openrouter-model-tracker
```

Ставит бинарник `openrouter-model-tracker` и короткий алиас `omt` — это один и
тот же бинарник под двумя именами. Обновление до новой версии:

```bash
brew upgrade MikcleGrok/openrouter/openrouter-model-tracker
```

Без Homebrew, или на других платформах, — сборка из исходников (нужен Go
1.26.5 или новее, см. `go.mod`):

```bash
git clone https://github.com/MikcleGrok/openrouter-model-tracker.git
cd openrouter-model-tracker
go build -o bin/openrouter ./cmd/openrouter
```

После клонирования полезно выполнить `make install-hooks` — это включает pre-commit
проверку, которая блокирует коммит приватных ключей и credentials в отслеживаемых файлах.

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
