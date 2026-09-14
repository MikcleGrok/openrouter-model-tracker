## Содержание

- [Фавориты по категориям (относительно уровня Claude)](#фавориты-по-категориям-относительно-уровня-claude)
- [Рейтинг моделей по цене и качеству](#рейтинг-моделей-по-цене-и-качеству)
- [Цены Claude (справочно)](#цены-claude-справочно)
- [Владельцы, открытость весов и рейтинг безопасности](#владельцы-открытость-весов-и-рейтинг-безопасности)
- [Модели по capability estimate (ranked by valid benchmark quality / price)](#модели-по-capability-estimate-ranked-by-valid-benchmark-quality--price)
- [Сколько токенов даст $10](#сколько-токенов-даст-10)
- [На что обратить внимание](#на-что-обратить-внимание)
- [Бесплатные модели (рейтинг по качеству)](#бесплатные-модели-рейтинг-по-качеству)
- [Приложение: происхождение оценок](#приложение-происхождение-оценок)

Обновлено: 2026-08-04 (цены и оценки собраны автоматически)
Ranking: mixed-utility · Sort: q/p · Score source: swebench

`Copyright` — ручная классификация поведения модели при запросе на защищённый контент: `compliant` — соблюдает ограничения, `non_compliant` — может обойти ограничения, `unknown` — проверяемого результата нет. Значение не выводится из лицензии.

## Фавориты по категориям (относительно уровня Claude)

Один лучший вариант на каждый уровень качества Claude.

| Capability estimate | Модель | Цена вход/выход, контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Copyright | Почему фаворит |
|---|---|---|---|---|---|---|---|---|
| ≈ Fable 5 | нет достойного кандидата | — | — | — | — | — | unknown | Ни одна проверенная модель независимо не подтверждает Fable-уровень. |
| >≈ Opus 5 | GPT-5.6 Luna | $0.50 / $3.00 ($1.00 / $4.00 от 272K+) · 1M | 93.0%v | 82.7 | OpenAI (C) | нет | unknown | Лучшее соотношение цена/качество. |
| ↳ второй выбор | GPT-5.6 Sol | $5.00 / $30.00 · 1M | 96.2%s | 8.6 | OpenAI (C) | нет | unknown | Ближе всего к Opus 5 по сырой оценке. |

## Рейтинг моделей по цене и качеству

| # | Модель | Slug | Claude | SWE % | Q/P | Контекст | Вход $/M | Выход $/M | Task fit |
|---:|---|---|---|---:|---:|---:|---:|---:|---|
| 1 | GPT-5.6 Sol | `openai/gpt-5.6-sol` | >≈ Opus 5 | 96.2%s | 8.6 | 1M | $5.00 | $30.00 | — |
| 2 | GPT-5.6 Luna | `openai/gpt-5.6-luna` | >≈ Opus 5 | 93.0%v | 82.7 | 1M | $0.50 | $3.00 | — |
| 3 | NVIDIA Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b:free` | ≈ Haiku 4.5 | 65–70.4% (только вендор) | n/a (free) | 1M | $0.00 | $0.00 | — |

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

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Copyright |
|---|---|---|---|---|---|---|---|---|---|
| GPT-5.6 Luna | `openai/gpt-5.6-luna` | $0.50 ($1.00 от 272K+) | $3.00 ($4.00 от 272K+) | 1M | 93.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-08-01 | 82.7 | OpenAI (C) | нет | unknown |
| GPT-5.6 Sol | `openai/gpt-5.6-sol` | $5.00 | $30.00 | 1M | 96.2% · [swebench.com](https://www.swebench.com/), 2026-08-02 | 8.6 | OpenAI (C) | нет | unknown |

**Заметки**

- **GPT-5.6 Luna** (`openai/gpt-5.6-luna`) — Независимая оценка (vals.ai).
- **GPT-5.6 Sol** (`openai/gpt-5.6-sol`) — Оговорка METR сохраняется.

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

| Модель | Slug на OpenRouter | Контекст | Benchmark score | Capability estimate | Владелец | Открытые веса | Copyright |
|---|---|---|---|---|---|---|---|
| NVIDIA Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b:free` | 1M | 65–70.4% (только вендор) | <≈ Haiku 4.5 (бесплатная) | NVIDIA | да, OpenMDW-1.1 | unknown |

**Заметки**

- **NVIDIA Nemotron 3 Ultra** (`nvidia/nemotron-3-ultra-550b-a55b:free`) — 550B/55B-active MoE, vendor-claimed & не подтверждено независимо.

Для всех `:free`-моделей: rate-limit 20 запросов/мин.

## Приложение: происхождение оценок

Служебный раздел для аудита: полная provenance каждого опубликованного
наблюдения. Для чтения и выбора модели он не нужен — смотрите таблицы выше.

- `openai/gpt-5.6-sol` — raw=96.2; metric=SWE-bench Verified; unit=n/a; variant=openai/gpt-5.6-sol; identity=n/a; checked=2026-08-02; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=n/a
- `openai/gpt-5.6-luna` — raw=93; metric=SWE-bench Verified; unit=n/a; variant=openai/gpt-5.6-luna; identity=n/a; checked=2026-08-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=n/a
- `nvidia/nemotron-3-ultra-550b-a55b:free` — raw=70.4; metric=SWE-bench Verified; unit=n/a; variant=vendor-claimed; identity=n/a; checked=n/a; source=n/a; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=n/a
