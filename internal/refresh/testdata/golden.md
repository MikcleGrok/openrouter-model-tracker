## Содержание

- [Фавориты по категориям (относительно уровня Claude)](#фавориты-по-категориям-относительно-уровня-claude)
- [Цены Claude (справочно)](#цены-claude-справочно)
- [Владельцы, открытость весов и рейтинг безопасности](#владельцы-открытость-весов-и-рейтинг-безопасности)
- [Модели по capability estimate (ranked by valid benchmark quality / price)](#модели-по-capability-estimate-ranked-by-valid-benchmark-quality--price)
- [Сколько токенов даст $10](#сколько-токенов-даст-10)
- [На что обратить внимание](#на-что-обратить-внимание)
- [Бесплатные модели (рейтинг по качеству)](#бесплатные-модели-рейтинг-по-качеству)

Обновлено: 2026-08-04 (цены и оценки собраны автоматически)

## Фавориты по категориям (относительно уровня Claude)

Один лучший вариант на каждый уровень качества Claude.

| Capability estimate | Модель | Цена вход/выход, контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Почему фаворит |
|---|---|---|---|---|---|---|---|
| ≈ Fable 5 | нет достойного кандидата | — | — | — | — | — | Ни одна проверенная модель независимо не подтверждает Fable-уровень. |
| >≈ Opus 5 | GPT-5.6 Luna | $0.50 / $3.00 ($1.00 / $4.00 от 272K+) · 1M | 93.0% | 82.7 | OpenAI (C) | нет | Лучшее соотношение цена/качество. |
  Provenance: raw=93; metric=SWE-bench Verified; unit=н/д; variant=openai/gpt-5.6-luna; identity=н/д; checked=н/д; source=н/д; uncertainty=н/д; sample=н/д; harness=н/д; scaffold=н/д; provider=н/д; configuration=н/д
| ↳ второй выбор | GPT-5.6 Sol | $5.00 / $30.00 · 1M | 96.2% | 8.6 | OpenAI (C) | нет | Ближе всего к Opus 5 по сырой оценке. |
  Provenance: raw=96.2; metric=SWE-bench Verified; unit=н/д; variant=openai/gpt-5.6-sol; identity=н/д; checked=н/д; source=н/д; uncertainty=н/д; sample=н/д; harness=н/д; scaffold=н/д; provider=н/д; configuration=н/д

## Цены Claude (справочно)

| Модель | Цена вход ($/M токенов) | Цена выход ($/M токенов) | Контекст | Заметка |
|---|---|---|---|---|
| Claude Opus 5 | $5 | $25 | 1M | — |

На OpenRouter цены Claude совпадают с прайсом Anthropic 1:1.

## Владельцы, открытость весов и рейтинг безопасности

Рейтинг безопасности — оценка компании в целом, а не модели.

| Компания | Грейд FLI | Комментарий |
|---|---|---|
| OpenAI | C (2.28) | Лидирует в категории Risk Assessment |

SaferAI Frontier Risk Management Tracker: OpenAI 34%.

Полностью закрытые: всё OpenAI.

## Модели по capability estimate (ranked by valid benchmark quality / price)

Категории — по примерному уровню качества относительно Claude.

### >≈ Opus 5

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Примечание |
|---|---|---|---|---|---|---|---|---|---|
| GPT-5.6 Luna | openai/gpt-5.6-luna | $0.50 ($1.00 от 272K+) | $3.00 ($4.00 от 272K+) | 1M | 93.0% | 82.7 | OpenAI (C) | нет | Независимая оценка (vals.ai). |
  Provenance: raw=93; metric=SWE-bench Verified; unit=н/д; variant=openai/gpt-5.6-luna; identity=н/д; checked=н/д; source=н/д; uncertainty=н/д; sample=н/д; harness=н/д; scaffold=н/д; provider=н/д; configuration=н/д
| GPT-5.6 Sol | openai/gpt-5.6-sol | $5.00 | $30.00 | 1M | 96.2% | 8.6 | OpenAI (C) | нет | Оговорка METR сохраняется. |
  Provenance: raw=96.2; metric=SWE-bench Verified; unit=н/д; variant=openai/gpt-5.6-sol; identity=н/д; checked=н/д; source=н/д; uncertainty=н/д; sample=н/д; harness=н/д; scaffold=н/д; provider=н/д; configuration=н/д

## Сколько токенов даст $10

Смешанное соотношение 3:1 (вход:выход).

**>≈ Opus 5**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| GPT-5.6 Luna | 20.0 | 3.33 | 8.89 |
| GPT-5.6 Sol | 2.00 | 0.33 | 0.89 |

**Claude (для сравнения)**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| Claude Opus 5 | 2.00 | 0.40 | 1.00 |

## На что обратить внимание

- Цены на OpenRouter меняются часто.
- Бенчмарки сильно зависят от скаффолда.

## Бесплатные модели (рейтинг по качеству)

Модели с ценой $0/$0 — из каталога OpenRouter.

| Модель | Slug на OpenRouter | Контекст | Benchmark score | Capability estimate | Владелец | Открытые веса | Примечание |
|---|---|---|---|---|---|---|---|
| NVIDIA Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b:free` | 1M | 65–70.4% (только вендор) | <≈ Haiku 4.5 (бесплатная) | NVIDIA | да, OpenMDW-1.1 | 550B/55B-active MoE. |
  Provenance: raw=70.4; metric=SWE-bench Verified; unit=н/д; variant=vendor-claimed; identity=н/д; checked=н/д; source=н/д; uncertainty=н/д; sample=н/д; harness=н/д; scaffold=н/д; provider=н/д; configuration=н/д

Для всех `:free`-моделей: rate-limit 20 запросов/мин.
