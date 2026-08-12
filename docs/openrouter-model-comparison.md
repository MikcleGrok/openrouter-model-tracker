## Содержание

- [Фавориты по категориям (относительно уровня Claude)](#фавориты-по-категориям-относительно-уровня-claude)
- [Цены Claude (справочно)](#цены-claude-справочно)
- [Владельцы, открытость весов и рейтинг безопасности](#владельцы-открытость-весов-и-рейтинг-безопасности)
- [Модели по capability estimate (ranked by valid benchmark quality / price)](#модели-по-capability-estimate-ranked-by-valid-benchmark-quality--price)
- [Сколько токенов даст $10](#сколько-токенов-даст-10)
- [На что обратить внимание](#на-что-обратить-внимание)
- [Бесплатные модели (рейтинг по качеству)](#бесплатные-модели-рейтинг-по-качеству)

Обновлено: 2026-08-12 (цены и контекст получены из каталога OpenRouter (/api/v1/models), оценки — с vals.ai и swebench.com по ручной карте model-map.tsv (21 запись vals=, 6 записей swebench=); остальные числа заданы вручную в notes.yaml и помечены как вендорские)

## Фавориты по категориям (относительно уровня Claude)

Один лучший вариант на каждый уровень качества Claude, отобранный по единому критерию: платные модели ранжируются по метрике «Качество/цена» — отношению SWE-bench Verified (%) к смешанной цене 3:1 вход:выход. У бесплатных моделей цена $0/$0 у всех, поэтому «качество/цена» не определена — они ранжируются по качеству. Строки, у которых оценка измерена не на том продукте, что продаётся под этим slug'ом (или у которых оценки по SWE-bench Verified нет вовсе), в отборе фаворитов **не участвуют**.

| Capability estimate | Модель | Цена вход/выход, контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Почему фаворит |
|---|---|---|---|---|---|---|---|
| ≈ Fable 5 | нет достойного кандидата | — | — | — | — | — | Ни одна проверенная модель независимо не подтверждает Fable-уровень: на лидерборде vals.ai сам Claude Fable 5 держит 95.0% SWE-bench Verified, а лучший сторонний результат в подборке относится к Opus-уровню. |
| >≈ Opus 5 | GPT-5.6 Luna | $0.10 / $0.60 ($0.20 / $0.90 от 272K+) · 1.1M | 93.0% | 413 | OpenAI (C) | нет | Лучшее соотношение цена/качество в Opus-тире: 93.0% SWE-bench Verified при цене в разы ниже, чем у остальных строк тира, оценка независимая (vals.ai). |
  Provenance: raw=93; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-luna; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a
| ↳ второй выбор | openai/gpt-5.6-terra | $1.00 / $6.00 ($2.00 / $9.00 от 272K+) · 1.1M | 75.2% | 33.4 | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=75.2; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-terra; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a
| ≈ Sonnet 5 | DeepSeek V4 Pro | $1.17 / $2.34 · 1M | 77.4% | 53.0 | DeepSeek (F) | **да, MIT** | _нужен обзор_ |
  Provenance: raw=77.4; metric=SWE-bench Verified; unit=%; variant=deepseek/deepseek-v4-pro; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=DeepSeek; configuration=n/a
| ↳ второй выбор | Gemini 3.6 Flash | $1.50 / $7.50 · 1M | 79.6% | 26.5 | Google DeepMind (C) | нет | _нужен обзор_ |
  Provenance: raw=79.6; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.6-flash; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a
| <≈ Haiku 4.5 | Xiaomi MiMo-V2.5 | $0.14 / $0.28 · 1.1M | 71.0% | 406 | Xiaomi (n/a) | **да, MIT** | _нужен обзор_ |
  Provenance: raw=71; metric=SWE-bench Verified; unit=%; variant=xiaomi/mimo-v2.5; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Xiaomi; configuration=n/a
| ↳ второй выбор | Xiaomi MiMo-V2.5-Pro | $0.44 / $0.87 · 1.1M | 74.0% | 136 | Xiaomi (n/a) | **да, MIT** | _нужен обзор_ |
  Provenance: raw=74; metric=SWE-bench Verified; unit=%; variant=xiaomi/mimo-v2.5-pro; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Xiaomi; configuration=n/a

## Цены Claude (справочно)

| Модель | Цена вход ($/M токенов) | Цена выход ($/M токенов) | Контекст | Заметка |
|---|---|---|---|---|
| Claude Opus 5 | $5 | $25 | 1M | — |
| Claude Sonnet 5 | $3 ($2 акционная цена до 2026-08-31) | $15 ($10 акционная цена до 2026-08-31) | 1M | — |
| Claude Haiku 4.5 | $1 | $5 | 200K | — |

На OpenRouter (`anthropic/claude-opus-5`, `anthropic/claude-sonnet-5`, `anthropic/claude-haiku-4.5`) цены совпадают с прайсом Anthropic 1:1, включая акционную цену Sonnet 5.

## Владельцы, открытость весов и рейтинг безопасности

Рейтинг безопасности ниже — это не оценка конкретной модели, а независимая оценка компании-разработчика в целом (её risk-практик, safety-фреймворков, прозрачности). Источник — Future of Life Institute, **«AI Safety Index — Summer 2026»** (опубликован 2026-07-07, futureoflife.org/ai-safety-index-summer-2026, шкала 0–4.0 с буквенным грейдом).

| Компания | Грейд FLI | Комментарий |
|---|---|---|
| Anthropic | **C+ (2.66)** — лучший результат индекса | Лидирует в 5 из 6 категорий оценки |
| OpenAI | C (2.28) | Лидирует в категории Risk Assessment |
| Google DeepMind | C (2.01) | Обновлённый Frontier Safety Framework |
| Meta | D+ (1.32) | Поднялась с 6-го на 4-е место |
| Z.ai (Zhipu AI, GLM) | D− (0.88) | Прозрачнее китайских конкурентов, но полагается на регулирование |
| Alibaba Cloud (Qwen) | D− (0.87) | — |
| xAI | F (0.65) | Упала с 4-го на 7-е место — «нет свидетельств значимой safety-команды» |
| DeepSeek | F (0.47) | Опирается на регуляторное соответствие, нет опубликованного safety-фреймворка |
| Mistral AI | F (0.33) — худший результат индекса | Отвергает саму рамку «frontier risk» |
| Xiaomi, Tencent, MiniMax, Moonshot AI (Kimi), Kwaipilot/Kuaishou, Meituan | не оценивались | Не входят в 9 компаний, охваченных индексом FLI Summer 2026 |

Второй, более узкий источник для сверки — SaferAI Frontier Risk Management Tracker (tracker.safer-ai.org, только 12 компаний, подписавших сеульские safety-обязательства): Anthropic 35% (#1), OpenAI 34%, Meta 33%, Google DeepMind 20%, xAI 18%, Cohere 8%. Оба источника оценивают компанию в целом, а не конкретную модель — прямого соответствия «модель → грейд» нет.

**Открытые веса (open-source/open-weight) — в таблицах ниже выделены полужирным в колонке «Открытые веса».** Полностью или частично открыты (с ограничениями по выручке/MAU в лицензии): все модели DeepSeek (MIT), Qwen3 Coder Next и Qwen3 Coder 480B (Apache 2.0 — но не Qwen3.7 Max), обе Xiaomi MiMo-V2.5 (MIT), Tencent Hy3 (Apache 2.0), MiniMax M3 (кастомная лицензия), GLM-5.2 (MIT), вся линейка Kimi (модифицированная MIT), Llama 4 Maverick (Llama 4 Community License), большая часть линейки Mistral — Devstral 2, Medium 3.5, Large 3 (но не Codestral 2508). Полностью закрытые: всё OpenAI, весь Gemini, Grok 4.5, Meta Muse Spark 1.1, обе модели KAT-Coder. Статус весов Meituan LongCat 2.0 подтвердить не удалось.

## Модели по capability estimate (ranked by valid benchmark quality / price)

Категории — по примерному уровню качества относительно Claude (см. таблицу фаворитов выше). **Качество/цена** = баллы SWE-bench Verified (%), делённые на смешанную цену 3:1 вход:выход, $/M. Строки, чья единственная доступная оценка измерена на другом продукте/варианте, и строки, у которых SWE-bench Verified не публиковали вовсе, стоят **в конце** таблицы своего тира, отсортированы по смешанной цене и в ранжировании не участвуют. Цены — **типовая цена каталога OpenRouter**, а не самый дешёвый маршрут конкретного провайдера.

### >≈ Opus 5

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Примечание |
|---|---|---|---|---|---|---|---|---|---|
| GPT-5.6 Luna | openai/gpt-5.6-luna | $0.10 ($0.20 от 272K+) | $0.60 ($0.90 от 272K+) | 1.1M | 93.0% | 413 | OpenAI (C) | нет | Оценка независимая, vals.ai. Сейчас лучшая цена/качество в Opus-тире. |
  Provenance: raw=93; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-luna; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a
| openai/gpt-5.6-terra | openai/gpt-5.6-terra | $1.00 ($2.00 от 272K+) | $6.00 ($9.00 от 272K+) | 1.1M | 75.2% | 33.4 | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=75.2; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-terra; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a
| GPT-5.6 Sol | openai/gpt-5.6-sol | $5.00 ($10.00 от 272K+) | $30.00 ($45.00 от 272K+) | 1.1M | 96.2% | 8.6 | OpenAI (C) | нет | Ближе всего к Opus 5 (97.0%) по сырой оценке, 96.2% подтверждены на vals.ai. Оговорка про METR уточнена: его находка про эксплуатацию багов eval'ов относится к **другому** бенчмарку — собственному agentic time-horizon eval'у METR на харнессе Terminal-Bench 2.1/ReAct, где у Sol самый высокий когда-либо измеренный уровень «читинга»; ни один отчёт не связывает это конкретно с числом 96.2% SWE-bench Verified. Отдельный аудит UC Berkeley RDI (апрель 2026, до выхода Sol) показал, что бенчмарки типа SWE-bench Verified в принципе поддаются накрутке харнесс-трюком — это не про Sol персонально. Общая осторожность к eval-результатам Sol сохраняется. |
  Provenance: raw=96.2; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-sol; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a
| openai/gpt-5.6-luna-pro | openai/gpt-5.6-luna-pro | $0.10 ($0.20 от 272K+) | $0.60 ($0.90 от 272K+) | 1.1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| openai/gpt-5.6-terra-pro | openai/gpt-5.6-terra-pro | $1.00 ($2.00 от 272K+) | $6.00 ($9.00 от 272K+) | 1.1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| openai/gpt-5.6-sol-pro | openai/gpt-5.6-sol-pro | $5.00 ($10.00 от 272K+) | $30.00 ($45.00 от 272K+) | 1.1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a

### ≈ Sonnet 5

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Примечание |
|---|---|---|---|---|---|---|---|---|---|
| DeepSeek V4 Pro | deepseek/deepseek-v4-pro | $1.17 | $2.34 | 1M | 77.4% | 53.0 | DeepSeek (F) | **да, MIT** | Заявленная оценка ~80.6% измерена для варианта «V4-Pro-Max», которого на OpenRouter не существует — это не тот продукт, что продаётся под `deepseek/deepseek-v4-pro`. Для реального продукта теперь есть независимая оценка на vals.ai (77.4%). |
  Provenance: raw=77.4; metric=SWE-bench Verified; unit=%; variant=deepseek/deepseek-v4-pro; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=DeepSeek; configuration=n/a
| Gemini 3.6 Flash | google/gemini-3.6-flash | $1.50 | $7.50 | 1M | 79.6% | 26.5 | Google DeepMind (C) | нет | Независимая оценка SWE-bench Verified теперь есть на vals.ai (79.6%) — на официальной странице Google DeepMind по-прежнему только **SWE-bench Pro 58.7% (другая метрика)**. Прошлая версия использовала прокси-оценку ~78%, взятую у предшественника (Gemini 3 Flash Preview, замену которого 3.6 Flash собой представляет): по правилу «оценка другого продукта не переносится» она в ранжирование не пошла — в «Качество/цена» теперь идёт независимое число с vals.ai. GitHub Copilot отключает старый slug (Gemini 3 Flash, вместе с Gemini 2.5 Pro) 2026-07-31 — подтверждено по changelog GitHub. |
  Provenance: raw=79.6; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.6-flash; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a
| Gemini 3.1 Pro Preview | google/gemini-3.1-pro-preview | $2.00 ($4.00 от 200K+) | $12.00 ($18.00 от 200K+) | 1M | 78.8% | 17.5 | Google DeepMind (C) | нет | Всё ещё preview — стабильного Gemini Pro на замену (Gemini 3.5 Pro) по-прежнему нет: Google подтвердила 2026-07-21, что релиз не готов (цель 17 июля сорвана), и выпускает вместо него Flash-модели. |
  Provenance: raw=78.8; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.1-pro-preview; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a
| qwen/qwen3.7-flash | qwen/qwen3.7-flash | $0.03 ($0.10 от 32K+) | $0.13 ($0.40 от 32K+) | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| MiniMax M3 | minimax/minimax-m3 | $0.30 | $1.20 | 1M | 75.0% [variant_mismatch] | n/a (variant mismatch) | MiniMax (n/a) | **да** (кастомная лицензия, коммерч. ограничения) | Лучшая цена/качество в этом тире. Оценка — только из блога вендора (minimax.io); на независимых лидербордах модели нет, достоверность низкая. У GMICloud есть маршрут дешевле типовой цены каталога. |
  Provenance: raw=75; metric=SWE-bench Verified; unit=%; variant=minimax/MiniMax-M3; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=MiniMax; configuration=n/a
| qwen/qwen3.7-plus | qwen/qwen3.7-plus | $0.32 ($0.96 от 256K+) | $1.28 ($3.84 от 256K+) | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| GLM-5.2 | z-ai/glm-5.2 | $0.50 | $3.15 | 1M | 82.8% [variant_mismatch] | n/a (variant mismatch) | Z.ai / Zhipu AI (D−) | **да, MIT** | Официальная документация Z.AI SWE-bench Verified не публикует — там только SWE-bench Pro 62.1% и Terminal-Bench 2.1 81.0% (другие метрики). Независимая оценка SWE-bench Verified есть на vals.ai (82.8%). Модель по-прежнему сильна в агентных задачах. |
  Provenance: raw=82.8; metric=SWE-bench Verified; unit=%; variant=zai/glm-5.2; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Zhipu AI; configuration=n/a
| Meta Muse Spark 1.1 | meta/muse-spark-1.1 | $1.25 | $4.25 | 1M | 82.0% [variant_mismatch] | n/a (variant mismatch) | Meta (D+) | нет | Флагман Meta вместо бренда Llama; 77.4% — заявка самой Meta, независимого подтверждения нет. Цифру SWE-bench Verified Hard 42.9% **не удалось проверить на 2026-07-30** (исходный PDF не читается, часть обзоров её вообще не упоминает) — оставлена с прошлой версии; сторонние обзоры дают только SWE-bench Pro 52.4–61.5% и Terminal-Bench. |
  Provenance: raw=82; metric=SWE-bench Verified; unit=%; variant=meta/muse_spark_1_1; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Meta; configuration=n/a
| Qwen3.7 Max | qwen/qwen3.7-max | $1.48 | $4.43 | 1M | 68.8% [variant_mismatch] | n/a (variant mismatch) | Alibaba Cloud / Qwen (D−) | нет | 80.4% держится только на вендорских и около-вендорских публикациях, независимого лидерборда нет. Alibaba анонсировала превью Qwen 3.8-Max (2026-07-19); к 2026-07-30 у превью появились API и цена ($0.17/$0.51 за M — «10% от стандартной»), но SWE-bench Verified не публиковали и продукта на OpenRouter нет — рано менять рекомендацию. |
  Provenance: raw=68.8; metric=SWE-bench Verified; unit=%; variant=alibaba/qwen3.7-max; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Alibaba; configuration=n/a
| Mistral Medium 3.5 | mistralai/mistral-medium-3-5 | $1.50 | $7.50 | 262K | 66.4% [variant_mismatch] | n/a (variant mismatch) | Mistral AI (F) | **да** (модиф. MIT, лицензия нужна при выручке >$20M/мес) | Теперь основная рекомендация Mistral для агентного кодинга — заменила Devstral 2 по умолчанию в их Vibe CLI. 77.6% — из анонса самой Mistral (в пересказе прессы), независимого подтверждения нет. |
  Provenance: raw=66.4; metric=SWE-bench Verified; unit=%; variant=mistralai/mistral-medium-3.5; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Mistral AI; configuration=n/a
| qwen/qwen3.8-max | qwen/qwen3.8-max | $2.00 | $6.00 | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Grok 4.5 | x-ai/grok-4.5 | $2.00 ($4.00 от 200K+) | $6.00 ($12.00 от 200K+) | 500K | 86.6% [variant_mismatch] | n/a (variant mismatch) | xAI (F) | нет | 86.6% подтверждены независимо (vals.ai), высокая достоверность. Реально близко к Opus по агентным задачам (SWE-bench Pro 64.7% — другая метрика; лидирует в SWE Marathon); свыше 200K токенов — $4/$12, кэш-чтение $0.60. |
  Provenance: raw=86.6; metric=SWE-bench Verified; unit=%; variant=grok/grok-4.5; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=SpaceXAI; configuration=n/a
| Kimi K3 | moonshotai/kimi-k3 | $3.00 | $15.00 | 1M | 93.4% [variant_mismatch] | n/a (variant mismatch) | Moonshot AI (n/a) | **да** (кастом. лицензия, свободно до 100M MAU) | **Основное число изменено в этом обновлении**: 93.4% взяты с живого независимого лидерборда vals.ai (ранг #4, обновление 2026-07-22, продукт совпадает точно). Прошлая версия документа показывала 76.8%, но источник этого числа **не удалось отследить на 2026-07-30** — нашлись только другие метрики (Toolathlon-Verified 76.5%, FrontierSWE 81.2%, DeepSWE 67.5%, SWE Marathon 42.0%, Program Bench 77.8%), ни одна из них не «76.8% SWE-bench Verified». По правилу «независимое измерение приоритетнее вендорского/неотслеживаемого» в ранжирование пошло 93.4% (качество/цена 15.6 вместо прежних 12.8); 76.8% сохранено здесь как альтернативная цифра с неподтверждённым источником. Расхождение такого размера у этой модели правдоподобно объясняется скаффолдом: на Terminal-Bench 2.1 у неё же 88.3% на харнессе Moonshot против 80.9% на прогоне vals.ai. По сырой оценке 93.4% модель уже на уровне тира >≈ Claude Opus 5 (у Luna — 93.0%), но оценка спорная, а цена равна полному прайсу Sonnet 5 без скидки — строка оставлена в этом тире; если 93.4% подтвердится вторым независимым источником, её следует перенести в >≈ Claude Opus 5. |
  Provenance: raw=93.4; metric=SWE-bench Verified; unit=%; variant=kimi/kimi-k3; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Moonshot AI; configuration=n/a

### <≈ Haiku 4.5

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Примечание |
|---|---|---|---|---|---|---|---|---|---|
| Xiaomi MiMo-V2.5 | xiaomi/mimo-v2.5 | $0.14 | $0.28 | 1.1M | 71.0% | 406 | Xiaomi (n/a) | **да, MIT** | Базовая версия MiMo (311B — отдельная модель, не «дешёвый режим» 1T-варианта Pro). Оценки по SWE-bench Verified по-прежнему нет; есть SWE-bench Pro 56.1% (другая метрика). Цена: типовая $0.14/$0.28 — прошлая версия показывала $0.112/$0.224, это маршрут GMICloud (скидка 20%). |
  Provenance: raw=71; metric=SWE-bench Verified; unit=%; variant=xiaomi/mimo-v2.5; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Xiaomi; configuration=n/a
| Xiaomi MiMo-V2.5-Pro | xiaomi/mimo-v2.5-pro | $0.44 | $0.87 | 1.1M | 74.0% | 136 | Xiaomi (n/a) | **да, MIT** | **Метрика заменена в этом обновлении**: появилась оценка именно по SWE-bench Verified — 78.9%, и именно для варианта "MiMo-V2.5-Pro". Но источник — PR «community evaluation results», влитый в HF-репозиторий самой модели: это вендор-хостинг чужого прогона, а не чистый сторонний лидерборд, **достоверность низкая** — ранжирование этой строки принимайте с осторожностью. Прежняя цифра SWE-bench Pro 57.2% никуда не делась и остаётся в силе как дополнительная точка (другая метрика, в ранжирование не идёт). Цена: типовая $0.435/$0.87 — прошлая версия показывала $0.348/$0.696, это маршрут GMICloud (скидка 20%). По данным на 2026-07-28 — #1 модель по доле трафика в категории "coding" на OpenRouter (18%). |
  Provenance: raw=74; metric=SWE-bench Verified; unit=%; variant=xiaomi/mimo-v2.5-pro; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Xiaomi; configuration=n/a
| Mistral Large 3 | mistralai/mistral-large-2512 | $0.50 | $1.50 | 262K | 41.4% | 55.2 | Mistral AI (F) | **да, Apache 2.0** | Универсальный флагман Mistral; преемника (Large 4) на дату проверки не существует, анонсов нет. Независимая оценка SWE-bench Verified есть на vals.ai (41.4%). |
  Provenance: raw=41.4; metric=SWE-bench Verified; unit=%; variant=mistralai/mistral-large-2512; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Mistral AI; configuration=n/a
| nvidia/nemotron-3-ultra-550b-a55b | nvidia/nemotron-3-ultra-550b-a55b | $0.60 | $3.60 | 512K | 69.0% | 51.1 | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=69; metric=SWE-bench Verified; unit=%; variant=nvidia/nemotron-3-ultra-550b-a55b; identity=exact_product; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Nvidia; configuration=n/a
| poolside/laguna-xs-2.1 | poolside/laguna-xs-2.1 | $0.06 | $0.12 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| nvidia/nemotron-3-nano-30b-a3b | nvidia/nemotron-3-nano-30b-a3b | $0.05 | $0.20 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| poolside/laguna-s-2.1 | poolside/laguna-s-2.1 | $0.09 | $0.18 | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| meta-llama/llama-4-scout | meta-llama/llama-4-scout | $0.10 | $0.30 | 1.3M | 9.1% [variant_mismatch] | n/a (variant mismatch) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=9.06; metric=SWE-bench Verified; unit=%; variant=mini-SWE-agent + Llama 4 Scout Instruct; identity=variant_mismatch; checked=2025-07-20; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| google/gemma-4-31b-it | google/gemma-4-31b-it | $0.10 | $0.34 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| nvidia/nemotron-3-super-120b-a12b | nvidia/nemotron-3-super-120b-a12b | $0.09 | $0.40 | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| DeepSeek V4 Flash | deepseek/deepseek-v4-flash | $0.14 | $0.28 | 1M | 88.8% [variant_mismatch] | n/a (variant mismatch) | DeepSeek (F) | **да, MIT** | Старые vendor claims по базовой модели и V4-Flash-Max сохранены как provenance. Живое независимое наблюдение 88.8% относится к deepseek-v4-flash-0731, а не к каталожному deepseek-v4-flash-20260423/0423, поэтому качество этого OpenRouter продукта неизвестно и число не ранжируется. |
  Provenance: raw=88.8; metric=SWE-bench Verified; unit=%; variant=deepseek/deepseek-v4-flash-0731; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=DeepSeek; configuration=n/a
| google/gemma-4-26b-a4b-it | google/gemma-4-26b-a4b-it | $0.12 | $0.40 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Tencent Hy3 | tencent/hy3 | $0.13 | $0.53 | 262K | SWE-V 78.0% (только вендор) | n/a (observation only) | Tencent / Hunyuan (n/a) | **да, Apache 2.0** | **Строка переехала сюда из таблицы «без независимой оценки»**: оценка появилась впервые — 78.0% для полного релиза (анонс 2026-07-06) против 74.4% у апрельского preview. Обе цифры — **самоотчёт Tencent, сторонней проверки (Artificial Analysis и др.) не найдено, достоверность низкая**; прошлая формулировка «бенчмарков нет» этим отменяется, но принимать 78.0% как измеренную независимо нельзя. #2 по объёму трафика и #1 по числу tool-calls в категории "coding" на OpenRouter — судя по всему уже широко используется в проде. Полный релиз без региональных ограничений — не путать с более ранним ограниченным "Hy3 Preview". |
  Provenance: raw=78; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://hunyuan.tencent.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| KAT-Coder V2.5 Air | kwaipilot/kat-coder-air-v2.5 | $0.15 | $0.60 | 256K | n/a | n/a (variant mismatch) | Kwaipilot / Kuaishou (n/a) | нет | Единственное доступное число 69.4% измерено на другом продукте — «KAT-Coder-V2.5-Dev» — и на Air не переносится. Вендорская заявка 79.6% именно для Air встречается только на листингах без ссылки на первоисточник. Строка не участвует в ранжировании. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Qwen3 Coder Next | qwen/qwen3-coder-next | $0.12 | $0.80 | 262K | SWE-V 70.6% | n/a (observation only) | Alibaba Cloud / Qwen (D−) | **да, Apache 2.0** | Все три числа перепроверены и не изменились. SWE-bench Pro 44.3%, Terminal-Bench 2.0 36.2 — другие метрики, модель слабее в общих терминал-агентных задачах. Цена реально изменилась: вход $0.11 → $0.12 (у StreamLake есть маршрут со «скидкой 40%» — $0.18/$0.90, то есть дороже типовой цены; смысла в нём нет). |
  Provenance: raw=70.6; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://qwenlm.github.io/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| DeepSeek V3.2 | deepseek/deepseek-v3.2 | $0.27 | $0.40 | 164K | 70.0% [variant_mismatch] | n/a (variant mismatch) | DeepSeek (F) | **да, MIT** | Оценки по вариантам расходятся: 67.8% (V3.2-Exp) — 73.1% (V3.2-Speciale), оба варианта названы в статье самой DeepSeek, то есть это реальный разброс по подвариантам, а не шум; взята середина. Цена: типовая в каталоге $0.269/$0.40 — прошлая версия показывала $0.21/$0.31, это скидочный маршрут Baidu ($0.2072/$0.3108, скидка 26%). |
  Provenance: raw=70; metric=SWE-bench Verified; unit=%; variant=mini-SWE-agent + DeepSeek V3.2 (high); identity=variant_mismatch; checked=2026-02-17; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Llama 4 Maverick | meta-llama/llama-4-maverick | $0.20 | $0.70 | 1M | 21.0% [variant_mismatch] | n/a (variant mismatch) | Meta (D+) | **да** (Llama 4 Community License, ограничения при >700M MAU) | Старое поколение (апрель 2025), больше не флагман Meta (теперь — Muse Spark). Независимая оценка SWE-bench Verified теперь есть на swebench.com (21.0%) — заметно ниже циркулировавших ранее непроверенных чисел (~24%, 49.2%, а 8.0% — вообще по SWE-bench **Lite**). |
  Provenance: raw=21.04; metric=SWE-bench Verified; unit=%; variant=mini-SWE-agent + Llama 4 Maverick Instruct; identity=variant_mismatch; checked=2025-07-20; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Mistral Codestral 2508 | mistralai/codestral-2508 | $0.30 | $0.90 | 256K | n/a | n/a (no SWE-bench Verified score) | Mistral AI (F) | частично (Mistral AI Non-Production License — только research/testing) | Специализация на автодополнении кода (FIM), не general-purpose — сравнивать с остальными некорректно. Единственное встреченное «52%» — низкоавторитетный источник без подтверждения, в таблицу не берётся. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Qwen3 Coder (480B) | qwen/qwen3-coder | $0.30 | $1.00 | 262K | 55.4% [variant_mismatch] | n/a (variant mismatch) | Alibaba Cloud / Qwen (D−) | **да, Apache 2.0** | Оценки сильно расходятся по источнику (68.4%, ~69-70%, 72.5%) — расхождение не разрешено, взята середина диапазона. Цена: разобрано расхождение прошлой версии — типовая цена каталога $0.30/$1.00, а показанные ранее $0.22/$1.80 относятся к отдельному маршруту Google Vertex (us-south1, без скидки, проверено 2026-07-30). Отсюда же прошлая заметка про «почти удвоившийся выход» — она описывала переход на этот маршрут, а не изменение прайса. |
  Provenance: raw=55.4; metric=SWE-bench Verified; unit=%; variant=mini-SWE-agent + Qwen3-Coder 480B/A35B Instruct; identity=variant_mismatch; checked=2025-08-02; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Meituan LongCat 2.0 | meituan/longcat-2.0 | $0.30 | $1.20 | 1M | n/a | n/a (no SWE-bench Verified score) | Meituan (n/a) | статус не подтверждён | **Новая строка в этом обновлении** — модель вышла 2026-07-20, то есть до прошлого обновления, но в подборку тогда не попала; slug живой (проверен в каталоге 2026-07-30). Оценок SWE-bench не найдено. По сообщениям прессы (Decrypt, VentureBeat) в июле выходила в топ трафика OpenRouter, но **эта проверка популярность независимо не подтвердила** — относитесь к «широко используется» как к непроверенному. Добавлена по тому же принципу, по которому в подборке раньше держалась Tencent Hy3: живой slug + заявленная заметная популярность при отсутствии бенчмарков. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| GPT-5 Mini | openai/gpt-5-mini | $0.25 | $2.00 | 400K | 59.8% [variant_mismatch] | n/a (variant mismatch) | OpenAI (C) | нет | Старое поколение — актуального "mini" в линейке 5.6 нет; ближайший бюджетный аналог сейчас — GPT-5.6 Luna в тире Opus выше. Сводимой оценки нет и в этой проверке (38% и 48% из источников без ссылок). |
  Provenance: raw=59.8; metric=SWE-bench Verified; unit=%; variant=mini-SWE-agent + GPT 5 mini (2025-08-07) (medium); identity=variant_mismatch; checked=2025-08-07; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Kimi K2.5 | moonshotai/kimi-k2.5 | $0.57 | $2.85 | 262K | 70.8% [variant_mismatch] | n/a (variant mismatch) | Moonshot AI (n/a) | **да** (модиф. MIT) | **Строка переехала сюда из таблицы «без независимой оценки»**: подозрение прошлой версии («оценка ~76.8% похожа на перепутанную с Kimi K3») **снято** — 76.8% привязаны конкретно к K2.5 на её официальной карточке Hugging Face, с описанной методикой (non-thinking mode, внутренний eval-фреймворк), а Kimi K3 — отдельная модель с собственной независимой оценкой 93.4% (vals.ai). Путаницы нет, есть два разных реальных числа. Тем не менее 76.8% — вендорская цифра, независимого подтверждения нет. Цена реально выросла: $0.375/$2.03 → $0.57/$2.85 (+52% вход / +40% выход); самый дешёвый маршрут сейчас — StreamLake $0.54/$2.70 (скидка 10%). |
  Provenance: raw=70.8; metric=SWE-bench Verified; unit=%; variant=mini-SWE-agent + Kimi K2.5 (high); identity=variant_mismatch; checked=2026-02-17; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| KAT-Coder V2.5 Pro | kwaipilot/kat-coder-pro-v2.5 | $0.74 | $2.96 | 256K | n/a | n/a (variant mismatch) | Kwaipilot / Kuaishou (n/a) | нет | **Оценка снята в этом обновлении**, по той же причине, что у Air: 69.4% — это Dev-вариант, своей оценки SWE-bench Verified у Pro-V2.5 нет. Побочная загадка прошлой версии разрешена: вендорская фраза «уступает только Opus 4.8» относится к SWE-bench **Pro** (V2.5: 65.2 против 69.2 у Opus 4.8), а не к Verified — то есть аргументом за какую-либо Verified-оценку она никогда не была. Строка **не участвует в ранжировании**. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Kimi K2.7 Code | moonshotai/kimi-k2.7-code | $0.70 | $3.50 | 262K | 78.2% [variant_mismatch] | n/a (variant mismatch) | Moonshot AI (n/a) | **да** (модиф. MIT) | Источник числа 60.4% в этой проверке отследить не удалось — значение оставлено с прошлой версии, **не удалось проверить на 2026-07-30**. Параллельно циркулирует 78.2% (automatio.ai) — расхождение не разрешено, приведены оба; при 78.2% качество/цена была бы 55.6. Страница модели на vals.ai существует, но значение через доступный инструментарий не читается — это **не** повод считать его нулём. Цена: типовая в каталоге $0.73/$3.50 (два независимых замера каталога 2026-07-30), прямая проверка страницы модели в тот же день дала $0.71/$3.50 — расхождение $0.02, уровня округления/маршрута; ещё один источник ранее указывал $0.95/$4.00 — сверьтесь на странице модели. |
  Provenance: raw=78.2; metric=SWE-bench Verified; unit=%; variant=kimi/kimi-k2.7-code; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Moonshot AI; configuration=n/a

## Сколько токенов даст $10

Смешанное соотношение 3:1 (вход:выход) — грубое приближение к типичной нагрузке кодинг-агента, а не гарантия реальных трат. Набор моделей и порядок строк здесь ровно те же, что в разделе выше.

**>≈ Opus 5**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| GPT-5.6 Luna | 100 | 16.7 | 44.4 |
| openai/gpt-5.6-terra | 10.0 | 1.67 | 4.44 |
| GPT-5.6 Sol | 2.00 | 0.33 | 0.89 |
| openai/gpt-5.6-luna-pro | 100 | 16.7 | 44.4 |
| openai/gpt-5.6-terra-pro | 10.0 | 1.67 | 4.44 |
| openai/gpt-5.6-sol-pro | 2.00 | 0.33 | 0.89 |

**≈ Sonnet 5**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| DeepSeek V4 Pro | 8.56 | 4.28 | 6.85 |
| Gemini 3.6 Flash | 6.67 | 1.33 | 3.33 |
| Gemini 3.1 Pro Preview | 5.00 | 0.83 | 2.22 |
| qwen/qwen3.7-flash | 333 | 76.9 | 182 |
| MiniMax M3 | 33.3 | 8.33 | 19.0 |
| qwen/qwen3.7-plus | 31.2 | 7.81 | 17.9 |
| GLM-5.2 | 20.0 | 3.17 | 8.60 |
| Meta Muse Spark 1.1 | 8.00 | 2.35 | 5.00 |
| Qwen3.7 Max | 6.78 | 2.26 | 4.52 |
| Mistral Medium 3.5 | 6.67 | 1.33 | 3.33 |
| qwen/qwen3.8-max | 5.00 | 1.67 | 3.33 |
| Grok 4.5 | 5.00 | 1.67 | 3.33 |
| Kimi K3 | 3.33 | 0.67 | 1.67 |

**<≈ Haiku 4.5**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| Xiaomi MiMo-V2.5 | 71.4 | 35.7 | 57.1 |
| Xiaomi MiMo-V2.5-Pro | 23.0 | 11.5 | 18.4 |
| Mistral Large 3 | 20.0 | 6.67 | 13.3 |
| nvidia/nemotron-3-ultra-550b-a55b | 16.7 | 2.78 | 7.41 |
| poolside/laguna-xs-2.1 | 167 | 83.3 | 133 |
| nvidia/nemotron-3-nano-30b-a3b | 200 | 50.0 | 114 |
| poolside/laguna-s-2.1 | 111 | 55.6 | 88.9 |
| meta-llama/llama-4-scout | 100 | 33.3 | 66.7 |
| google/gemma-4-31b-it | 100 | 29.4 | 62.5 |
| nvidia/nemotron-3-super-120b-a12b | 118 | 25.0 | 61.1 |
| DeepSeek V4 Flash | 71.4 | 35.7 | 57.1 |
| google/gemma-4-26b-a4b-it | 83.3 | 25.0 | 52.6 |
| Tencent Hy3 | 75.8 | 18.9 | 43.3 |
| KAT-Coder V2.5 Air | 66.7 | 16.7 | 38.1 |
| Qwen3 Coder Next | 83.3 | 12.5 | 34.5 |
| DeepSeek V3.2 | 37.2 | 25.0 | 33.1 |
| Llama 4 Maverick | 50.0 | 14.4 | 30.9 |
| Mistral Codestral 2508 | 33.3 | 11.1 | 22.2 |
| Qwen3 Coder (480B) | 33.3 | 10.0 | 21.1 |
| Meituan LongCat 2.0 | 33.3 | 8.33 | 19.0 |
| GPT-5 Mini | 40.0 | 5.00 | 14.5 |
| Kimi K2.5 | 17.5 | 3.51 | 8.77 |
| KAT-Coder V2.5 Pro | 13.5 | 3.38 | 7.72 |
| Kimi K2.7 Code | 14.3 | 2.86 | 7.14 |

**Claude (для сравнения)**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| Claude Opus 5 | 2.00 | 0.40 | 1.00 |
| Claude Sonnet 5 (цена по прайсу $3/$15) | 3.33 | 0.67 | 1.67 |
| Claude Sonnet 5 (акционная цена $2/$10 до 2026-08-31) | 5.00 | 1.00 | 2.50 |
| Claude Haiku 4.5 | 10.0 | 2.00 | 5.00 |

## На что обратить внимание

- Цены на OpenRouter меняются часто (неделями, иногда днями) — перед тем как тратить деньги, сверьтесь напрямую на https://openrouter.ai/models.
- **Часть моделей в подборке не имеет оценки SWE-bench Verified для того самого продукта, который продаётся на OpenRouter** — у них в колонке «Качество/цена» стоит `n/a`, они стоят в конце таблицы своего тира и не участвуют ни в ранжировании, ни в отборе фаворитов.
- Бенчмарки сильно зависят от скаффолда/harness, которым их гоняли, а не только от модели — расхождения в 15-25 процентных пунктов для одной и той же модели между разными лидербордами не редкость.
- Значительная часть оценок в таблицах — **вендорские** (помечено «только вендор»). Независимо подтверждены только те строки, число которых пришло с vals.ai или swebench.com (в model-map.tsv 21 запись vals= и 6 записей swebench=).
- SWE-bench Verified и подобные бенчмарки не идеально предсказывают реальное качество в конкретном воркфлоу — протестируйте 1-2 кандидата на своих задачах перед переключением.
- OpenRouter обычно даёт скидку на закешированный контекст (порядка 60-90% по большинству моделей), но точный процент варьируется по провайдеру.
- В таблицах всюду **типовая цена каталога** OpenRouter (`/api/v1/models`), а не самый дешёвый маршрут конкретного провайдера; актуальный список промо-цен — https://openrouter.ai/collections/discounted-models.

## Бесплатные модели (рейтинг по качеству)

Модели с ценой $0/$0 (не путать с дешёвыми платными) — из каталога OpenRouter (`pricing.prompt == "0"` и `pricing.completion == "0"`). Лучшая по качеству вынесена в «Фавориты по категориям» выше. Две оговорки: **все оценки здесь вендорские** — ни одна бесплатная модель в подборке не имеет независимого прогона SWE-bench Verified; и **каталог бесплатных моделей волатилен** — проверяйте наличие slug'а перед тем, как на него закладываться.

| Модель | Slug на OpenRouter | Контекст | Benchmark score | Capability estimate | Владелец | Открытые веса | Примечание |
|---|---|---|---|---|---|---|---|
| Cohere North Mini Code | `cohere/north-mini-code:free` | 256K | SWE-bench Verified 67.6% (SWE-agent harness) | <≈ Haiku 4.5 (середина диапазона) | Cohere (Cohere Labs) | да, Apache 2.0 | 30B/3B-active MoE; SWE-bench Pro 40.2% (другая метрика). Оценки вендорские, независимого подтверждения не найдено. Единственная компания из этой таблицы, попавшая в трекер SaferAI — 8% (последнее место среди 12). |
  Provenance: raw=67.6; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://cohere.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Google Gemma 4 26B A4B | `google/gemma-4-26b-a4b-it:free` | 262K | n/a | <≈ Haiku 4.5 (меньше активных параметров, чем у 31B-версии) — не подтверждено | Google DeepMind | да, Apache 2.0 | 26B/4B-active MoE. Утверждение прошлой версии про «реальный лимит эндпоинта 131K» **ослаблено**: каталог OpenRouter сейчас показывает 262144, а известный кейс с обрезкой до 131072 документирован для конкретного бэкенда (Cloudflare Workers AI) — маршрутизирует ли OpenRouter этот `:free`-slug через него, выяснить не удалось. ELO 1441 в этой проверке независимо не переподтверждён, оставлен с прошлой версии. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Google Gemma 4 31B | `google/gemma-4-31b-it:free` | 262K | n/a | не определить по этой метрике | Google DeepMind | да, Apache 2.0 | Бенчмарка SWE-bench Verified не нашли. Есть LMArena ELO ~1452 (#3 среди открытых) и Codeforces ELO 2150 — другие метрики, напрямую несопоставимы. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| NVIDIA Nemotron 3 Nano 30B-A3B | `nvidia/nemotron-3-nano-30b-a3b:free` | 256K | SWE-bench Verified ~38.8%; LiveCodeBench 68.3% | <≈ Haiku 4.5 — заметно слабее любой модели из тиров выше | NVIDIA | да, OpenMDW-1.1 | 31.6B/~3.2B-active. LiveCodeBench 68.3% перепроверен точно; у 38.8% в этой проверке нашёлся только слабый источник (community-репозиторий с траекториями, не вендорская публикация) — значение оставлено с прошлой версии, **провенанс на 2026-07-30 подтвердить не удалось**. |
  Provenance: raw=38.8; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=провенанс не подтверждён на 2026-07-30 (см. note); uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| NVIDIA Nemotron 3 Nano Omni 30B-A3B (reasoning) | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | 256K | n/a | не применимо — не кодинг-модель | NVIDIA | да | Не для сравнения по кодингу — иная специализация (мультимодальность подтверждена в этой проверке). |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| NVIDIA Nemotron 3 Super | `nvidia/nemotron-3-super-120b-a12b:free` | 262K | SWE-bench Verified ~60.5% (60.47%, харнесс OpenHands, только вендор) | <≈ Haiku 4.5 (нижняя граница диапазона) | NVIDIA | да, OpenMDW-1.1 | 120B/12B-active MoE. |
  Provenance: raw=60.47; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://developer.nvidia.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| NVIDIA Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b:free` | 1M | 69.0% [variant_mismatch] | <≈ Haiku 4.5 (середина диапазона) | NVIDIA | да, OpenMDW-1.1 | 550B/55B-active MoE. Прогон на 5 разных агентных скаффолдах делала сама NVIDIA — это демонстрация устойчивости к харнессу, а не независимое подтверждение. Независимая оценка на vals.ai теперь есть (69.0%) и используется в таблице вместо вендорского диапазона ниже. |
  Provenance: raw=69; metric=SWE-bench Verified; unit=%; variant=nvidia/nemotron-3-ultra-550b-a55b; identity=variant_mismatch; checked=2026-08-08; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Nvidia; configuration=n/a
| nvidia/nemotron-3.5-content-safety:free | `nvidia/nemotron-3.5-content-safety:free` | 128K | n/a | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| NVIDIA Nemotron Nano 12B v2 VL | `nvidia/nemotron-nano-12b-v2-vl:free` | 128K | n/a | не применимо — не кодинг-модель | NVIDIA | да | Не для сравнения по кодингу — иная специализация. На 2026-07-30 строка **не перепроверялась** (не хватило бюджета проверки) — данные с прошлой версии. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| NVIDIA Nemotron Nano 9B v2 | `nvidia/nemotron-nano-9b-v2:free` | 128K | n/a | <≈ Haiku 4.5 — малая reasoning-модель общего назначения, не кодинг-специализация | NVIDIA | да | Общая reasoning-модель, не кодинг-специализация. Индекс AA (независимый оценщик) опубликован для варианта с тегом "-reasoning"; модель документирована как единая и всегда выдаёт reasoning-трейс, отдельного не-reasoning сиблинга на OpenRouter нет — скорее всего это тот же продукт, но полного совпадения slug'а в этой проверке подтвердить не удалось. LiveCodeBench 71.1% — вендорская цифра. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| OpenAI gpt-oss-20b | `openai/gpt-oss-20b:free` | 131K | n/a | не определить по этой метрике — сильна в математике/олимпиадном кодинге, агентный SWE-bench не публиковался | OpenAI | да, Apache 2.0 | Другие метрики — не сравнивайте число напрямую с SWE-bench Verified. AIME25 98.7% (точную цифру в этой проверке подтвердить не удалось, нашлись только качественные формулировки «на уровне или выше o3-mini»; значение оставлено с прошлой версии), Codeforces ≈ o3-mini. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Poolside Laguna S 2.1 | `poolside/laguna-s-2.1:free` | 262K | n/a | вероятно <≈ Haiku 4.5 — но по не-Verified метрикам, не сравнивайте напрямую | Poolside | да, OpenMDW-1.1 | 118B/8B-active. SWE-bench Verified нет; есть SWE-bench Multilingual 78.5%, Pro 59.4%, Terminal-Bench 2.1 70.2%, DeepSWE v1.1 40.4% — метрики не Verified, напрямую с остальными строками несопоставимы. |
  Provenance: raw=n/a; metric=n/a; unit=n/a; variant=n/a; identity=missing_identity; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a
| Poolside Laguna XS 2.1 | `poolside/laguna-xs-2.1:free` | 262K | SWE-bench Verified 70.9% (только вендор) | <≈ Haiku 4.5 (верх диапазона) | Poolside | да, OpenMDW-1.1 | 33B/3B-active, узкая специализация — агентный кодинг; независимо не подтверждено (Poolside прямо пишет в карточке, что публикует «official scores» из своих релизных постов). Фактически вровень с Nemotron 3 Ultra, уступает по контексту. Курьёз: у той же модели есть отдельный **платный** slug без `:free` — $0.06/$0.12 (скидка 40%), там лимитов бесплатного тира нет. |
  Provenance: raw=70.9; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://poolside.ai/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a

Практические оговорки для всех `:free`-моделей: rate-limit 20 запросов/мин; 50 запросов/день на аккаунте с пополнениями меньше $10 за всё время, 1000/день после разового пополнения от $10 (openrouter.ai/docs/api-reference/limits). Часть провайдеров бесплатных эндпоинтов может использовать запросы для обучения; управляется это настройками приватности аккаунта — openrouter.ai/settings/privacy.
