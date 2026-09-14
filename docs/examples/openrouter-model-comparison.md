## Содержание

- [Фавориты по категориям (относительно уровня Claude)](#фавориты-по-категориям-относительно-уровня-claude)
- [Рейтинг моделей по цене и качеству](#рейтинг-моделей-по-цене-и-качеству)
- [Цены Claude (справочно)](#цены-claude-справочно)
- [Владельцы, открытость весов и рейтинг безопасности](#владельцы-открытость-весов-и-рейтинг-безопасности)
- [Модели по capability estimate (ranked by valid benchmark quality / price)](#модели-по-capability-estimate-ranked-by-valid-benchmark-quality--price)
- [Сколько токенов даст $10](#сколько-токенов-даст-10)
- [На что обратить внимание](#на-что-обратить-внимание)
- [Бесплатные модели (рейтинг по качеству)](#бесплатные-модели-рейтинг-по-качеству)
- [Актуальные модели без ручного сопоставления](#актуальные-модели-без-ручного-сопоставления)
- [Приложение: происхождение оценок](#приложение-происхождение-оценок)

Обновлено: 2026-09-14 (цены и контекст получены из каталога OpenRouter (/api/v1/models), оценки — с vals.ai и swebench.com по ручной карте model-map.tsv (34 записи vals=, 10 записей swebench=); остальные числа заданы вручную в notes.yaml и помечены как вендорские)
Ranking: mixed-utility · Sort: q/p · Score source: swebench

`Copyright` — ручная классификация поведения модели при запросе на защищённый контент: `compliant` — соблюдает ограничения, `non_compliant` — может обойти ограничения, `unknown` — проверяемого результата нет. Значение не выводится из лицензии.

## Фавориты по категориям (относительно уровня Claude)

Один лучший вариант на каждый уровень качества Claude, отобранный по единому критерию: платные модели ранжируются по метрике «Качество/цена» — отношению SWE-bench Verified (%) к смешанной цене 3:1 вход:выход. У бесплатных моделей цена $0/$0 у всех, поэтому «качество/цена» не определена — они ранжируются по качеству. Строки, у которых оценка измерена не на том продукте, что продаётся под этим slug'ом (или у которых оценки по SWE-bench Verified нет вовсе), в отборе фаворитов **не участвуют**.

| Capability estimate | Модель | Цена вход/выход, контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Copyright | Почему фаворит |
|---|---|---|---|---|---|---|---|---|
| ≈ Fable 5 | нет достойного кандидата | — | — | — | — | — | unknown | Сам Claude Fable 5 независимо подтверждён на лидерборде vals.ai с результатом 95.0% SWE-bench Verified; лучший сторонний результат в подборке относится к Opus-уровню. |
| >≈ Opus 5 | GPT-5.6 Luna | $0.20 / $1.20 ($0.40 / $1.80 от 272K+) · 1.1M | 93.0%v | 207 | OpenAI (C) | нет | unknown | Лучшее соотношение цена/качество в Opus-тире: 93.0% SWE-bench Verified при цене в разы ниже, чем у остальных строк тира, оценка независимая (vals.ai). |
| ↳ второй выбор | GPT-5.6 Sol | $2.00 / $10.00 ($4.00 / $15.00 от 272K+) · 1.1M | 96.2%v | 24.1 | OpenAI (C) | нет | unknown | Заметно хуже по цена/качество, чем Luna, но ближе всего к Opus 5 по сырой оценке. METR фиксировал эксплуатацию багов eval'ов у Sol в **другом** бенчмарке, а не в SWE-bench Verified — но общая осторожность к её eval-результатам сохраняется. |
| ≈ Sonnet 5 | MiniMax M3 | $0.30 / $1.20 · 1M | 75.0%v | 143 | MiniMax (n/a) | **да** (кастомная лицензия, коммерч. ограничения) | unknown | Топ-1 по цена/качество в Sonnet-тире при цене в 10-12 раз ниже полного прайса Sonnet 5. Оценка теперь независимая (vals.ai, 75.0%) — прежняя вендорская заявка 80.5% снята. |
| ↳ второй выбор | GLM-5.2 | $0.68 / $2.15 · 1M | 82.8%v | 78.9 | Z.ai / Zhipu AI (D−) | **да, MIT** | unknown | _нужен обзор_ |
| <≈ Haiku 4.5 | Xiaomi MiMo-V2.5 | $0.14 / $0.28 · 1.1M | 71.0%v | 406 | Xiaomi (n/a) | **да, MIT** | unknown | _нужен обзор_ |
| ↳ второй выбор | GLM 5.3 Flash | $0.15 / $0.50 · 1.3M | 92.0%v | 387 | _нужен обзор_ | _нужен обзор_ | unknown | _нужен обзор_ |
| <≈ Haiku 4.5 (бесплатная) | NVIDIA Nemotron 3 Ultra | $0.00 / $0.00 · 1M | 69.0%v | n/a (free) | NVIDIA | да, OpenMDW-1.1 | unknown | Лидер бесплатного тира в этом обновлении: прежний лидер z-ai/glm-5.2:free исчез из каталога OpenRouter (slug мёртв, снят из model-map.tsv), а Nemotron 3 Ultra — единственная строка тира с независимо подтверждённой оценкой (vals.ai, 69.0%). У Laguna XS 2.1 сырая цифра формально выше (70.9%), но это вендорская, а не независимая оценка — Nemotron выигрывает по достоверности источника, а не по сырому числу; контекст также больше (1M против 262K у Laguna). |

## Рейтинг моделей по цене и качеству

| # | Модель | Slug | Claude | SWE % | Q/P | Контекст | Вход $/M | Выход $/M | Task fit |
|---:|---|---|---|---:|---:|---:|---:|---:|---|
| 1 | Xiaomi MiMo-V2.5 | `xiaomi/mimo-v2.5` | ≈ Haiku 4.5 | 71.0%v | 406 | 1.1M | $0.14 | $0.28 | implement + debug + refactor + test |
| 2 | GLM 5.3 Flash | `z-ai/glm-5.3-flash` | ≈ Haiku 4.5 | 92.0%v | 387 | 1.3M | $0.15 | $0.50 | implement + debug + refactor + test |
| 3 | DeepSeek V3.2 | `deepseek/deepseek-v3.2` | ≈ Haiku 4.5 | 70.0%s | 232 | 164K | $0.27 | $0.40 | implement + debug + refactor + test |
| 4 | GPT-5.6 Luna | `openai/gpt-5.6-luna` | >≈ Opus 5 | 93.0%v | 207 | 1.1M | $0.20 | $1.20 | implement + plan + research + debug + audit + refactor + test |
| 5 | MiniMax M2.5 | `minimax/minimax-m2.5` | ≈ Haiku 4.5 | 74.2%v | 157 | 205K | $0.27 | $1.08 | implement + debug + refactor + test |
| 6 | MiniMax M3 | `minimax/minimax-m3` | ≈ Sonnet 5 | 75.0%v | 143 | 1M | $0.30 | $1.20 | implement + plan + research + debug + audit + refactor + test |
| 7 | MiniMax M2 | `minimax/minimax-m2` | <≈ Haiku 4.5 | 61.0%s | 137 | 205K | $0.26 | $1.02 | implement + test |
| 8 | Xiaomi MiMo-V2.5-Pro | `xiaomi/mimo-v2.5-pro` | ≈ Haiku 4.5 | 74.0%v | 136 | 1.1M | $0.44 | $0.87 | implement + plan + debug + audit + refactor + test |
| 9 | Qwen3 Coder (480B) | `qwen/qwen3-coder` | <<≈ Haiku 4.5 | 55.4%s | 117 | 262K | $0.30 | $1.00 | implement + plan + debug + audit + refactor + test |
| 10 | GLM 4.7 | `z-ai/glm-4.7` | <≈ Haiku 4.5 | 69.4%v | 94.1 | 205K | $0.40 | $1.75 | implement + debug + refactor + test |
| 11 | GPT-5 Mini | `openai/gpt-5-mini` | <<≈ Haiku 4.5 | 58.0%s | 84.4 | 400K | $0.25 | $2.00 | implement + plan + debug + refactor + test |
| 12 | GLM-5.2 | `z-ai/glm-5.2` | ≈ Sonnet 5 | 82.8%v | 78.9 | 1M | $0.68 | $2.15 | implement + plan + research + debug + audit + refactor + test |
| 13 | Kimi K2.5 | `moonshotai/kimi-k2.5` | ≈ Haiku 4.5 | 70.8%s | 78.7 | 262K | $0.45 | $2.25 | implement + plan + research + debug + audit + refactor + test |
| 14 | Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b` | <≈ Haiku 4.5 | 69.0%v | 65.7 | 262K | $0.60 | $2.40 | implement + plan + debug + refactor + test |
| 15 | Llama 4 Maverick | `meta-llama/llama-4-maverick` | <<≈ Haiku 4.5 | 21.0%s | 64.9 | 1M | $0.20 | $0.70 | implement + test |
| 16 | Llama 4 Scout | `meta-llama/llama-4-scout` | <<≈ Haiku 4.5 | 9.1%s | 60.4 | 1.3M | $0.10 | $0.30 | implement + test |
| 17 | Kimi K2.7 Code | `moonshotai/kimi-k2.7-code` | ≈ Haiku 4.5 | 78.2%v | 55.6 | 262K | $0.71 | $3.50 | implement + debug + refactor + test |
| 18 | Mistral Large 3 | `mistralai/mistral-large-2512` | <<≈ Haiku 4.5 | 41.4%v | 55.2 | 262K | $0.50 | $1.50 | implement + plan + research + debug + refactor + test |
| 19 | Gemini 3.7 Flash | `google/gemini-3.7-flash` | ≈ Sonnet 5 | 80.8%v | 53.9 | 1M | $0.75 | $3.75 | implement + plan + debug + refactor + test |
| 20 | Gemini 3.8 Flash | `google/gemini-3.8-flash` | ≈ Sonnet 5 | 80.0%v | 53.3 | 1M | $0.75 | $3.75 | implement + plan + debug + refactor + test |
| 21 | Gemini 3.6 Flash | `google/gemini-3.6-flash` | ≈ Sonnet 5 | 79.6%v | 53.1 | 1M | $0.75 | $3.75 | implement + plan + debug + refactor + test |
| 22 | GLM 5.3 | `z-ai/glm-5.3` | ≈ Sonnet 5 | 95.4%v | 44.4 | 1.3M | $1.40 | $4.40 | implement + plan + research + debug + audit + refactor + test |
| 23 | Meta Muse Spark 1.1 | `meta/muse-spark-1.1` | ≈ Sonnet 5 | 82.0%v | 41.0 | 1M | $1.25 | $4.25 | implement + plan + research + debug + audit + refactor + test |
| 24 | DeepSeek V4 Pro | `deepseek/deepseek-v4-pro` | ≈ Sonnet 5 | 77.4%v | 38.7 | 1M | $1.60 | $3.20 | implement + plan + research + debug + audit + refactor + test |
| 25 | Claude Haiku 4.5 | `anthropic/claude-haiku-4.5` | <≈ Haiku 4.5 | 66.6%s | 33.3 | 200K | $1.00 | $5.00 | implement + debug + refactor + test |
| 26 | SpaceXAI: Grok 4.6 | `x-ai/grok-4.6` | ≈ Sonnet 5 | 95.6%v | 31.9 | 500K | $2.00 | $6.00 | implement + plan + research + debug + audit + refactor + test |
| 27 | Qwen3.7 Max | `qwen/qwen3.7-max` | ≈ Sonnet 5 | 68.8%v | 31.1 | 1M | $1.48 | $4.43 | implement + plan + research + debug + audit + refactor + test |
| 28 | Grok 4.5 | `x-ai/grok-4.5` | ≈ Sonnet 5 | 86.6%v | 28.9 | 500K | $2.00 | $6.00 | implement + plan + research + debug + audit + refactor + test |
| 29 | Qwen3.8 Max (0902) | `qwen/qwen3.8-max-0902` | ≈ Sonnet 5 | 85.6%v | 28.5 | 1M | $2.00 | $6.00 | implement + plan + research + debug + audit + refactor + test |
| 30 | GPT-5.6 Sol | `openai/gpt-5.6-sol` | >≈ Opus 5 | 96.2%v | 24.1 | 1.1M | $2.00 | $10.00 | implement + plan + research + debug + audit + refactor + test |
| 31 | Gemini 3.5 Flash | `google/gemini-3.5-flash` | ≈ Sonnet 5 | 78.8%v | 23.3 | 1M | $1.50 | $9.00 | implement + plan + debug + refactor + test |
| 32 | Mistral Medium 3.5 | `mistralai/mistral-medium-3-5` | ≈ Sonnet 5 | 66.4%v | 22.1 | 262K | $1.50 | $7.50 | implement + plan + research + debug + audit + refactor + test |
| 33 | GPT-5.6 Terra | `openai/gpt-5.6-terra` | >≈ Opus 5 | 95.4%v | 21.2 | 1.1M | $2.00 | $12.00 | implement + plan + research + debug + audit + refactor + test |
| 34 | Claude Sonnet 5 | `anthropic/claude-sonnet-5` | ≈ Sonnet 5 | 79.6%v | 19.9 | 1M | $2.00 | $10.00 | implement + plan + research + debug + audit + refactor + test |
| 35 | Kimi K3 | `moonshotai/kimi-k3` | ≈ Sonnet 5 | 93.4%v | 17.6 | 1M | $2.65 | $13.28 | implement + plan + research + debug + audit + refactor + test |
| 36 | Gemini 3.1 Pro Preview | `google/gemini-3.1-pro-preview` | ≈ Sonnet 5 | 78.8%v | 17.5 | 1M | $2.00 | $12.00 | implement + plan + research + debug + audit + refactor + test |
| 37 | Claude Sonnet 4.6 | `anthropic/claude-sonnet-4.6` | ≈ Sonnet 5 | 77.4%v | 12.9 | 1M | $3.00 | $15.00 | implement + plan + research + debug + audit + refactor + test |
| 38 | Claude Sonnet 4.5 | `anthropic/claude-sonnet-4.5` | ≈ Sonnet 5 | 74.8%s | 12.5 | 1M | $3.00 | $15.00 | implement + plan + research + debug + audit + refactor + test |
| 39 | Claude Opus 5 | `anthropic/claude-opus-5` | >≈ Opus 5 | 97.0%v | 9.7 | 1M | $5.00 | $25.00 | implement + plan + research + debug + audit + refactor + test |
| 40 | Claude Opus 4.8 | `anthropic/claude-opus-4.8` | >≈ Opus 5 | 88.6%v | 8.9 | 1M | $5.00 | $25.00 | implement + plan + research + debug + audit + refactor + test |
| 41 | Claude Opus 4.7 | `anthropic/claude-opus-4.7` | >≈ Opus 5 | 82.0%v | 8.2 | 1M | $5.00 | $25.00 | implement + plan + research + debug + audit + refactor + test |
| 42 | Claude Opus 4.6 | `anthropic/claude-opus-4.6` | >≈ Opus 5 | 75.6%s | 7.6 | 1M | $5.00 | $25.00 | implement + plan + research + debug + audit + refactor + test |
| 43 | Claude Fable 5 | `anthropic/claude-fable-5` | >≈ Opus 5 | 95.0%v | 4.8 | 1M | $10.00 | $50.00 | implement + plan + research + debug + audit + refactor + test |
| 44 | AionLabs: Aion-2.0 | `aion-labs/aion-2.0` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.80 | $1.60 | — |
| 45 | AionLabs: Aion-3.0 | `aion-labs/aion-3.0` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $3.00 | $6.00 | — |
| 46 | AionLabs: Aion-3.0-Mini | `aion-labs/aion-3.0-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.70 | $1.40 | — |
| 47 | AionLabs: Aion-RP 1.0 (8B) | `aion-labs/aion-rp-llama-3.1-8b` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.80 | $1.60 | — |
| 48 | Nova 2 Lite | `amazon/nova-2-lite-v1` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.30 | $2.50 | — |
| 49 | Nova Lite 1.0 | `amazon/nova-lite-v1` | n/a | n/a | n/a (no SWE-bench Verified score) | 300K | $0.06 | $0.24 | — |
| 50 | Nova Micro 1.0 | `amazon/nova-micro-v1` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.04 | $0.14 | — |
| 51 | Nova Premier 1.0 | `amazon/nova-premier-v1` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.50 | $12.50 | — |
| 52 | Nova Pro 1.0 | `amazon/nova-pro-v1` | n/a | n/a | n/a (no SWE-bench Verified score) | 300K | $0.80 | $3.20 | — |
| 53 | Magnum v4 72B | `anthracite-org/magnum-v4-72b` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $2.50 | $5.00 | — |
| 54 | Claude 3 Haiku | `anthropic/claude-3-haiku` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $0.25 | $1.25 | — |
| 55 | Claude Fable 5.1 | `anthropic/claude-fable-5.1` | >≈ Opus 5 | n/a | n/a (no SWE-bench Verified score) | 1M | $10.00 | $50.00 | implement + plan + research + debug + audit + refactor + test |
| 56 | Claude Fable 5.1 (batch) | `anthropic/claude-fable-5.1:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $5.00 | $25.00 | — |
| 57 | Claude Fable 5 (batch) | `anthropic/claude-fable-5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $5.00 | $25.00 | — |
| 58 | Claude Haiku 4.5 (batch) | `anthropic/claude-haiku-4.5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $0.50 | $2.50 | — |
| 59 | Claude Opus 4 | `anthropic/claude-opus-4` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $15.00 | $75.00 | — |
| 60 | Claude Opus 4.1 | `anthropic/claude-opus-4.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $15.00 | $75.00 | — |
| 61 | Claude Opus 4.1 (batch) | `anthropic/claude-opus-4.1:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $7.50 | $37.50 | — |
| 62 | Claude Opus 4.5 | `anthropic/claude-opus-4.5` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $5.00 | $25.00 | — |
| 63 | Claude Opus 4.5 (batch) | `anthropic/claude-opus-4.5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $2.50 | $12.50 | — |
| 64 | Claude Opus 4.6 (batch) | `anthropic/claude-opus-4.6:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.50 | $12.50 | — |
| 65 | Claude Opus 4.7 (batch) | `anthropic/claude-opus-4.7:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.50 | $12.50 | — |
| 66 | Claude Opus 4.8 (batch) | `anthropic/claude-opus-4.8:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.50 | $12.50 | — |
| 67 | Claude Opus 5 (batch) | `anthropic/claude-opus-5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.50 | $12.50 | — |
| 68 | Claude Sonnet 4 | `anthropic/claude-sonnet-4` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $3.00 | $15.00 | — |
| 69 | Claude Sonnet 4.5 (batch) | `anthropic/claude-sonnet-4.5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.50 | $7.50 | — |
| 70 | Claude Sonnet 4.6 (batch) | `anthropic/claude-sonnet-4.6:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.50 | $7.50 | — |
| 71 | Claude Sonnet 5 (batch) | `anthropic/claude-sonnet-5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.00 | $5.00 | — |
| 72 | Arcee AI: Trinity Large Thinking | `arcee-ai/trinity-large-thinking` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.25 | $0.80 | — |
| 73 | ERNIE 4.5 VL 424B A47B | `baidu/ernie-4.5-vl-424b-a47b` | n/a | n/a | n/a (no SWE-bench Verified score) | 123K | $0.42 | $1.25 | — |
| 74 | ByteDance Seed: Seed 1.6 | `bytedance-seed/seed-1.6` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.25 | $2.00 | — |
| 75 | ByteDance Seed: Seed 1.6 Flash | `bytedance-seed/seed-1.6-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.08 | $0.30 | — |
| 76 | ByteDance Seed: Seed 2.1 Turbo | `bytedance-seed/seed-2-1-turbo` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.50 | $2.50 | — |
| 77 | ByteDance Seed: Seed-2.0-Code | `bytedance-seed/seed-2.0-code` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.50 | $3.00 | — |
| 78 | ByteDance Seed: Seed-2.0-Lite | `bytedance-seed/seed-2.0-lite` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.25 | $2.00 | — |
| 79 | ByteDance Seed: Seed-2.0-Mini | `bytedance-seed/seed-2.0-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.10 | $0.40 | — |
| 80 | UI-TARS 7B | `bytedance/ui-tars-1.5-7b` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.10 | $0.20 | — |
| 81 | Venice: Uncensored | `cognitivecomputations/dolphin-mistral-24b-venice-edition` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.20 | $0.90 | — |
| 82 | Command A | `cohere/command-a` | n/a | n/a | n/a (no SWE-bench Verified score) | 256K | $2.50 | $10.00 | — |
| 83 | Command R (08-2024) | `cohere/command-r-08-2024` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.15 | $0.60 | — |
| 84 | Command R+ (08-2024) | `cohere/command-r-plus-08-2024` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.50 | $10.00 | — |
| 85 | Command R7B (12-2024) | `cohere/command-r7b-12-2024` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.04 | $0.15 | — |
| 86 | Cohere North Mini Code | `cohere/north-mini-code:free` | <≈ Haiku 4.5 | 67.6% [observation_only]s | n/a (free) | 256K | $0.00 | $0.00 | implement + debug + refactor + test |
| 87 | DeepSeek V3 | `deepseek/deepseek-chat` | n/a | n/a | n/a (no SWE-bench Verified score) | 164K | $0.26 | $1.03 | — |
| 88 | DeepSeek V3 0324 | `deepseek/deepseek-chat-v3-0324` | n/a | n/a | n/a (no SWE-bench Verified score) | 164K | $0.25 | $1.00 | — |
| 89 | DeepSeek V3.1 | `deepseek/deepseek-chat-v3.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 164K | $0.25 | $0.95 | — |
| 90 | R1 | `deepseek/deepseek-r1` | n/a | n/a | n/a (no SWE-bench Verified score) | 64K | $0.70 | $2.50 | — |
| 91 | R1 0528 | `deepseek/deepseek-r1-0528` | n/a | n/a | n/a (no SWE-bench Verified score) | 164K | $0.50 | $2.15 | — |
| 92 | R1 Distill Llama 70B | `deepseek/deepseek-r1-distill-llama-70b` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.80 | $0.80 | — |
| 93 | DeepSeek V3.1 Terminus | `deepseek/deepseek-v3.1-terminus` | n/a | n/a | n/a (no SWE-bench Verified score) | 164K | $0.27 | $1.00 | — |
| 94 | DeepSeek V3.2 Exp | `deepseek/deepseek-v3.2-exp` | n/a | n/a | n/a (no SWE-bench Verified score) | 164K | $0.27 | $0.41 | — |
| 95 | DeepSeek V4 Flash | `deepseek/deepseek-v4-flash` | ≈ Haiku 4.5 | 88.8% [variant_mismatch]v | n/a (variant mismatch) | 1M | $0.09 | $0.18 | implement + debug + refactor + test |
| 96 | DeepSeek V4 Flash 0731 | `deepseek/deepseek-v4-flash-0731` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.3M | $0.06 | $0.12 | — |
| 97 | DeepSeek V4 Flash 0731 (batch) | `deepseek/deepseek-v4-flash-0731:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.11 | $0.33 | — |
| 98 | DeepSeek V4 Flash Vision Exp | `deepseek/deepseek-v4-flash-vision-exp` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.22 | $0.66 | — |
| 99 | DeepSeek V4 Flash Vision Exp (batch) | `deepseek/deepseek-v4-flash-vision-exp:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.11 | $0.33 | — |
| 100 | DeepSeek V4 Pro 0813 | `deepseek/deepseek-v4-pro-0813` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.98 | $2.95 | — |
| 101 | DeepSeek V4 Pro 0813 (batch) | `deepseek/deepseek-v4-pro-0813:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.66 | $1.98 | — |
| 102 | DeepSeek V4.1 Flash | `deepseek/deepseek-v4.1-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.15 | $0.60 | — |
| 103 | Dots Studio: Dots3-Note Preview (free) | `dots-studio/dots-3-note-preview:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 512K | $0.00 | $0.00 | — |
| 104 | Gemini 2.5 Flash | `google/gemini-2.5-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.30 | $2.50 | — |
| 105 | Nano Banana (Gemini 2.5 Flash Image) | `google/gemini-2.5-flash-image` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.30 | $2.50 | — |
| 106 | Gemini 2.5 Flash Lite | `google/gemini-2.5-flash-lite` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.10 | $0.40 | — |
| 107 | Gemini 2.5 Flash Lite (batch) | `google/gemini-2.5-flash-lite:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.05 | $0.20 | — |
| 108 | Gemini 2.5 Flash (batch) | `google/gemini-2.5-flash:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.15 | $1.25 | — |
| 109 | Gemini 2.5 Pro | `google/gemini-2.5-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.25 | $10.00 | — |
| 110 | Gemini 2.5 Pro Preview 06-05 | `google/gemini-2.5-pro-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.25 | $10.00 | — |
| 111 | Gemini 2.5 Pro Preview 05-06 | `google/gemini-2.5-pro-preview-05-06` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.25 | $10.00 | — |
| 112 | Gemini 2.5 Pro (batch) | `google/gemini-2.5-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.63 | $5.00 | — |
| 113 | Gemini 3 Flash Preview | `google/gemini-3-flash-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.50 | $3.00 | — |
| 114 | Gemini 3 Flash Preview (batch) | `google/gemini-3-flash-preview:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.25 | $1.50 | — |
| 115 | Nano Banana Pro (Gemini 3 Pro Image) | `google/gemini-3-pro-image` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $2.00 | $12.00 | — |
| 116 | Nano Banana Pro (Gemini 3 Pro Image Preview) | `google/gemini-3-pro-image-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $2.00 | $12.00 | — |
| 117 | Nano Banana 2 (Gemini 3.1 Flash Image) | `google/gemini-3.1-flash-image` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.50 | $3.00 | — |
| 118 | Nano Banana 2 (Gemini 3.1 Flash Image Preview) | `google/gemini-3.1-flash-image-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $0.50 | $3.00 | — |
| 119 | Gemini 3.1 Flash Lite | `google/gemini-3.1-flash-lite` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.25 | $1.50 | — |
| 120 | Nano Banana 2 Lite (Gemini 3.1 Flash Lite Image) | `google/gemini-3.1-flash-lite-image` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $0.25 | $1.50 | — |
| 121 | Gemini 3.1 Flash Lite Preview | `google/gemini-3.1-flash-lite-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.25 | $1.50 | — |
| 122 | Gemini 3.1 Flash Lite (batch) | `google/gemini-3.1-flash-lite:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.13 | $0.75 | — |
| 123 | Gemini 3.1 Pro Preview Custom Tools | `google/gemini-3.1-pro-preview-customtools` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.00 | $12.00 | — |
| 124 | Gemini 3.1 Pro Preview (batch) | `google/gemini-3.1-pro-preview:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.00 | $6.00 | — |
| 125 | Gemini 3.5 Flash Lite | `google/gemini-3.5-flash-lite` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.30 | $2.50 | — |
| 126 | Gemini 3.5 Flash Lite (batch) | `google/gemini-3.5-flash-lite:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.15 | $1.25 | — |
| 127 | Gemini 3.5 Flash (batch) | `google/gemini-3.5-flash:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.75 | $4.50 | — |
| 128 | Gemini 3.6 Flash (batch) | `google/gemini-3.6-flash:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.38 | $1.88 | — |
| 129 | Gemini 3.7 Flash (batch) | `google/gemini-3.7-flash:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.38 | $1.88 | — |
| 130 | Gemini 3.8 Flash (batch) | `google/gemini-3.8-flash:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.38 | $1.88 | — |
| 131 | Gemma 2 27B | `google/gemma-2-27b-it` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.65 | $0.65 | — |
| 132 | Gemma 3 12B | `google/gemma-3-12b-it` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.05 | $0.15 | — |
| 133 | Gemma 3 27B | `google/gemma-3-27b-it` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.08 | $0.45 | — |
| 134 | Gemma 3 4B | `google/gemma-3-4b-it` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.05 | $0.10 | — |
| 135 | Gemma 4 26B A4B | `google/gemma-4-26b-a4b-it` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 262K | $0.09 | $0.30 | implement + test |
| 136 | Google Gemma 4 26B A4B | `google/gemma-4-26b-a4b-it:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 262K | $0.00 | $0.00 | implement + test |
| 137 | Gemma 4 31B | `google/gemma-4-31b-it` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 262K | $0.09 | $0.34 | implement + test |
| 138 | Gemma 4 31B (batch) | `google/gemma-4-31b-it:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.39 | $0.97 | — |
| 139 | Google Gemma 4 31B | `google/gemma-4-31b-it:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 262K | $0.00 | $0.00 | implement + test |
| 140 | Lyria 3 Clip Preview | `google/lyria-3-clip-preview` | n/a | n/a | n/a (free) | 1M | $0.00 | $0.00 | — |
| 141 | Lyria 3 Pro Preview | `google/lyria-3-pro-preview` | n/a | n/a | n/a (free) | 1M | $0.00 | $0.00 | — |
| 142 | MythoMax 13B | `gryphe/mythomax-l2-13b` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.06 | $0.06 | — |
| 143 | IBM: Granite 4.0 Micro | `ibm-granite/granite-4.0-h-micro` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.02 | $0.11 | — |
| 144 | IBM: Granite 4.2 8B | `ibm-granite/granite-4.2-8b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.06 | $0.25 | — |
| 145 | Mercury 2 | `inception/mercury-2` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.25 | $0.75 | — |
| 146 | Mercury 2.5 | `inception/mercury-2.5` | n/a | n/a | n/a (no SWE-bench Verified score) | 260K | $0.04 | $0.15 | — |
| 147 | Ling 3.0 Flash | `inclusionai/ling-3.0-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.02 | $0.06 | — |
| 148 | Ling 3.0 Flash Fin | `inclusionai/ling-3.0-flash-fin` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.06 | $0.18 | — |
| 149 | Ling 3.0 Flash Fin (free) | `inclusionai/ling-3.0-flash-fin:free` | n/a | n/a | n/a (free) | 262K | $0.00 | $0.00 | — |
| 150 | Ling 3.0 Flash Sante (free) | `inclusionai/ling-3.0-flash-sante:free` | n/a | n/a | n/a (free) | 262K | $0.00 | $0.00 | — |
| 151 | Ling 3.0 Flash VL | `inclusionai/ling-3.0-flash-vl` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.06 | $0.18 | — |
| 152 | Ling 3.0 Flash VL (free) | `inclusionai/ling-3.0-flash-vl:free` | n/a | n/a | n/a (free) | 262K | $0.00 | $0.00 | — |
| 153 | Inference.net: Schematron V2 Small | `inference-net/schematron-v2-small` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.05 | $0.23 | — |
| 154 | Inference.net: Schematron V2 Turbo | `inference-net/schematron-v2-turbo` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.03 | $0.15 | — |
| 155 | KAT-Coder-Pro V2 | `kwaipilot/kat-coder-pro-v2` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.30 | $1.20 | — |
| 156 | KAT-Coder V2.5 Pro | `kwaipilot/kat-coder-pro-v2.5` | ≈ Haiku 4.5 | n/a | n/a (variant mismatch) | 262K | $0.74 | $2.96 | implement + debug + refactor + test |
| 157 | LiquidAI: LFM2.5-2.6B (free) | `liquid/lfm-2.5-2.6b:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 66K | $0.00 | $0.00 | — |
| 158 | Weaver (alpha) | `mancer/weaver` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.40 | $0.75 | — |
| 159 | Meituan LongCat 2.0 | `meituan/longcat-2.0` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 1M | $0.30 | $1.20 | implement + debug + refactor + test |
| 160 | Llama 3.1 70B Instruct | `meta-llama/llama-3.1-70b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.40 | $0.40 | — |
| 161 | Llama 3.1 8B Instruct | `meta-llama/llama-3.1-8b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.05 | $0.08 | — |
| 162 | Llama 3.2 1B Instruct | `meta-llama/llama-3.2-1b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 60K | $0.03 | $0.20 | — |
| 163 | Llama 3.2 3B Instruct | `meta-llama/llama-3.2-3b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.05 | $0.33 | — |
| 164 | Llama 3.3 70B Instruct | `meta-llama/llama-3.3-70b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.10 | $0.32 | — |
| 165 | Llama Guard 4 12B | `meta-llama/llama-guard-4-12b` | n/a | n/a | n/a (no SWE-bench Verified score) | 164K | $0.18 | $0.18 | — |
| 166 | Muse Glimmer 30B | `meta/muse-glimmer-30b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.35 | $1.50 | — |
| 167 | Muse Glimmer 30B (batch) | `meta/muse-glimmer-30b:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.18 | $0.75 | — |
| 168 | Muse Spark 1.2 | `meta/muse-spark-1.2` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.25 | $4.25 | — |
| 169 | Muse Spark 1.2 Contributor | `meta/muse-spark-1.2-contributor` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.10 | $0.20 | — |
| 170 | Muse Spark 1.3 | `meta/muse-spark-1.3` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.25 | $4.25 | — |
| 171 | Muse Spark 1.3 Contributor | `meta/muse-spark-1.3-contributor` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.10 | $0.20 | — |
| 172 | Phi 4 | `microsoft/phi-4` | n/a | n/a | n/a (no SWE-bench Verified score) | 16K | $0.07 | $0.14 | — |
| 173 | WizardLM-2 8x22B | `microsoft/wizardlm-2-8x22b` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $0.62 | $0.62 | — |
| 174 | MiniMax-01 | `minimax/minimax-01` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.20 | $1.10 | — |
| 175 | MiniMax M1 | `minimax/minimax-m1` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.55 | $2.20 | — |
| 176 | MiniMax M2-her | `minimax/minimax-m2-her` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $0.30 | $1.20 | — |
| 177 | MiniMax M2.1 | `minimax/minimax-m2.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 205K | $0.30 | $1.20 | — |
| 178 | MiniMax M2.7 | `minimax/minimax-m2.7` | n/a | n/a | n/a (no SWE-bench Verified score) | 205K | $0.30 | $1.20 | — |
| 179 | MiniMax M3 (batch) | `minimax/minimax-m3:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 524K | $0.30 | $1.20 | — |
| 180 | Mistral Codestral 2508 | `mistralai/codestral-2508` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 256K | $0.30 | $0.90 | implement |
| 181 | Mistral: Codestral 2508 (batch) | `mistralai/codestral-2508:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 256K | $0.15 | $0.45 | — |
| 182 | Mistral Devstral 2 | `mistralai/devstral-2512` | n/a | 72.2% [observation_only]s | n/a (observation only) | 262K | $0.40 | $2.00 | — |
| 183 | Mistral: Ministral 3 14B 2512 | `mistralai/ministral-14b-2512` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.20 | $0.20 | — |
| 184 | Mistral: Ministral 3 3B 2512 | `mistralai/ministral-3b-2512` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.10 | $0.10 | — |
| 185 | Mistral: Ministral 3 8B 2512 | `mistralai/ministral-8b-2512` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.15 | $0.15 | — |
| 186 | Mistral: Ministral 3 8B 2512 (batch) | `mistralai/ministral-8b-2512:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.08 | $0.08 | — |
| 187 | Mistral Large | `mistralai/mistral-large` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.00 | $6.00 | — |
| 188 | Mistral Large 2407 | `mistralai/mistral-large-2407` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $2.00 | $6.00 | — |
| 189 | Mistral: Mistral Large 3 2512 (batch) | `mistralai/mistral-large-2512:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.25 | $0.75 | — |
| 190 | Mistral: Mistral Medium 3 | `mistralai/mistral-medium-3` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.40 | $2.00 | — |
| 191 | Mistral: Mistral Medium 3.5 (batch) | `mistralai/mistral-medium-3-5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.75 | $3.75 | — |
| 192 | Mistral: Mistral Medium 3.1 | `mistralai/mistral-medium-3.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.40 | $2.00 | — |
| 193 | Mistral: Mistral Medium 3.1 (batch) | `mistralai/mistral-medium-3.1:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.20 | $1.00 | — |
| 194 | Mistral: Mistral Nemo | `mistralai/mistral-nemo` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.02 | $0.03 | — |
| 195 | Mistral: Saba | `mistralai/mistral-saba` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.20 | $0.60 | — |
| 196 | Mistral: Mistral Small 3 | `mistralai/mistral-small-24b-instruct-2501` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.05 | $0.08 | — |
| 197 | Mistral: Mistral Small 4 | `mistralai/mistral-small-2603` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.15 | $0.60 | — |
| 198 | Mistral: Mistral Small 4 (batch) | `mistralai/mistral-small-2603:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.08 | $0.30 | — |
| 199 | Mistral: Mistral Small 3.1 24B | `mistralai/mistral-small-3.1-24b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.35 | $0.56 | — |
| 200 | Mistral: Mistral Small 3.2 24B | `mistralai/mistral-small-3.2-24b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 256K | $0.08 | $0.20 | — |
| 201 | Mistral: Mixtral 8x22B Instruct | `mistralai/mixtral-8x22b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $2.00 | $6.00 | — |
| 202 | Mistral: Voxtral Small 24B 2507 | `mistralai/voxtral-small-24b-2507` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.10 | $0.30 | — |
| 203 | MoonshotAI: Kimi K2 0711 | `moonshotai/kimi-k2` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.57 | $2.30 | — |
| 204 | MoonshotAI: Kimi K2 0905 | `moonshotai/kimi-k2-0905` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.60 | $2.50 | — |
| 205 | MoonshotAI: Kimi K2 Thinking | `moonshotai/kimi-k2-thinking` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.60 | $2.50 | — |
| 206 | MoonshotAI: Kimi K2.6 | `moonshotai/kimi-k2.6` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.95 | $4.00 | — |
| 207 | MoonshotAI: Kimi K3 (batch) | `moonshotai/kimi-k3:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $3.00 | $15.00 | — |
| 208 | Morph V3 Fast | `morph/morph-v3-fast` | n/a | n/a | n/a (no SWE-bench Verified score) | 82K | $0.80 | $1.20 | — |
| 209 | Morph V3 Large | `morph/morph-v3-large` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.90 | $1.90 | — |
| 210 | Nex AGI: Nex-N2.5-Mini (free) | `nex-agi/nex-n2.5-mini:free` | n/a | n/a | n/a (free) | 262K | $0.00 | $0.00 | — |
| 211 | Nex AGI: Nex-N2.5-Pro (free) | `nex-agi/nex-n2.5-pro:free` | n/a | n/a | n/a (free) | 262K | $0.00 | $0.00 | — |
| 212 | Nous: Hermes 3 405B Instruct | `nousresearch/hermes-3-llama-3.1-405b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $1.00 | $1.00 | — |
| 213 | Nous: Hermes 3 70B Instruct | `nousresearch/hermes-3-llama-3.1-70b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.70 | $0.70 | — |
| 214 | Nous: Hermes 4 405B | `nousresearch/hermes-4-405b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $1.00 | $3.00 | — |
| 215 | Nemotron 3 Nano 30B A3B | `nvidia/nemotron-3-nano-30b-a3b` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 262K | $0.05 | $0.20 | implement + test |
| 216 | NVIDIA Nemotron 3 Nano Omni 30B-A3B (reasoning) | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 256K | $0.00 | $0.00 | — |
| 217 | Nemotron 3 Super | `nvidia/nemotron-3-super-120b-a12b` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 262K | $0.08 | $0.45 | implement + debug + refactor + test |
| 218 | NVIDIA Nemotron 3 Super | `nvidia/nemotron-3-super-120b-a12b:free` | <≈ Haiku 4.5 | 60.5% [observation_only]s | n/a (free) | 262K | $0.00 | $0.00 | implement + debug + refactor + test |
| 219 | NVIDIA Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b:free` | <≈ Haiku 4.5 | 69.0%v | n/a (free) | 1M | $0.00 | $0.00 | implement + plan + debug + refactor + test |
| 220 | Nemotron 3.5 Content Safety | `nvidia/nemotron-3.5-content-safety` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.20 | $0.20 | — |
| 221 | Nemotron 3.5 Content Safety (free) | `nvidia/nemotron-3.5-content-safety:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 128K | $0.00 | $0.00 | — |
| 222 | Nemotron 3.5 Lightning | `nvidia/nemotron-3.5-lightning` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.08 | $0.20 | — |
| 223 | NVIDIA Nemotron 3.5 Lightning | `nvidia/nemotron-3.5-lightning:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 1M | $0.00 | $0.00 | implement + debug + refactor + test |
| 224 | GPT-3.5 Turbo | `openai/gpt-3.5-turbo` | n/a | n/a | n/a (no SWE-bench Verified score) | 16K | $0.50 | $1.50 | — |
| 225 | GPT-3.5 Turbo (older v0613) | `openai/gpt-3.5-turbo-0613` | n/a | n/a | n/a (no SWE-bench Verified score) | 4K | $1.00 | $2.00 | — |
| 226 | GPT-3.5 Turbo 16k | `openai/gpt-3.5-turbo-16k` | n/a | n/a | n/a (no SWE-bench Verified score) | 16K | $3.00 | $4.00 | — |
| 227 | GPT-3.5 Turbo Instruct | `openai/gpt-3.5-turbo-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 4K | $1.50 | $2.00 | — |
| 228 | GPT-3.5 Turbo (batch) | `openai/gpt-3.5-turbo:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 16K | $0.25 | $0.75 | — |
| 229 | GPT-4 | `openai/gpt-4` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $30.00 | $60.00 | — |
| 230 | GPT-4 Turbo | `openai/gpt-4-turbo` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $10.00 | $30.00 | — |
| 231 | GPT-4 Turbo Preview | `openai/gpt-4-turbo-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $10.00 | $30.00 | — |
| 232 | GPT-4 Turbo (batch) | `openai/gpt-4-turbo:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $5.00 | $15.00 | — |
| 233 | GPT-4.1 | `openai/gpt-4.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.00 | $8.00 | — |
| 234 | GPT-4.1 Mini | `openai/gpt-4.1-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.40 | $1.60 | — |
| 235 | GPT-4.1 Mini (batch) | `openai/gpt-4.1-mini:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.20 | $0.80 | — |
| 236 | GPT-4.1 Nano | `openai/gpt-4.1-nano` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.10 | $0.40 | — |
| 237 | GPT-4.1 Nano (batch) | `openai/gpt-4.1-nano:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.05 | $0.20 | — |
| 238 | GPT-4.1 (batch) | `openai/gpt-4.1:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.00 | $4.00 | — |
| 239 | GPT-4o | `openai/gpt-4o` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.50 | $10.00 | — |
| 240 | GPT-4o (2024-05-13) | `openai/gpt-4o-2024-05-13` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $5.00 | $15.00 | — |
| 241 | GPT-4o (2024-08-06) | `openai/gpt-4o-2024-08-06` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.50 | $10.00 | — |
| 242 | GPT-4o (2024-11-20) | `openai/gpt-4o-2024-11-20` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.50 | $10.00 | — |
| 243 | GPT-4o-mini | `openai/gpt-4o-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.15 | $0.60 | — |
| 244 | GPT-4o-mini (2024-07-18) | `openai/gpt-4o-mini-2024-07-18` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.15 | $0.60 | — |
| 245 | GPT-4o-mini (batch) | `openai/gpt-4o-mini:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.08 | $0.30 | — |
| 246 | GPT-4o (batch) | `openai/gpt-4o:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $1.25 | $5.00 | — |
| 247 | GPT-5 | `openai/gpt-5` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $1.25 | $10.00 | — |
| 248 | GPT-5 Image | `openai/gpt-5-image` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $10.00 | $10.00 | — |
| 249 | GPT-5 Image Mini | `openai/gpt-5-image-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $2.50 | $2.00 | — |
| 250 | GPT-5 Mini (batch) | `openai/gpt-5-mini:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.13 | $1.00 | — |
| 251 | GPT-5 Nano | `openai/gpt-5-nano` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.05 | $0.40 | — |
| 252 | GPT-5 Nano (batch) | `openai/gpt-5-nano:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.03 | $0.20 | — |
| 253 | GPT-5 Pro | `openai/gpt-5-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $15.00 | $120.00 | — |
| 254 | GPT-5 Pro (batch) | `openai/gpt-5-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $7.50 | $60.00 | — |
| 255 | GPT-5.1 | `openai/gpt-5.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $1.25 | $10.00 | — |
| 256 | GPT-5.1-Codex | `openai/gpt-5.1-codex` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $1.25 | $10.00 | — |
| 257 | GPT-5.1-Codex-Max | `openai/gpt-5.1-codex-max` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $1.25 | $10.00 | — |
| 258 | GPT-5.1-Codex-Mini | `openai/gpt-5.1-codex-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.25 | $2.00 | — |
| 259 | GPT-5.1 (batch) | `openai/gpt-5.1:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.63 | $5.00 | — |
| 260 | GPT-5.2 | `openai/gpt-5.2` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $1.75 | $14.00 | — |
| 261 | GPT-5.2 Chat | `openai/gpt-5.2-chat` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $1.75 | $14.00 | — |
| 262 | GPT-5.2-Codex | `openai/gpt-5.2-codex` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $1.75 | $14.00 | — |
| 263 | GPT-5.2 Pro | `openai/gpt-5.2-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $21.00 | $168.00 | — |
| 264 | GPT-5.2 Pro (batch) | `openai/gpt-5.2-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $10.50 | $84.00 | — |
| 265 | GPT-5.2 (batch) | `openai/gpt-5.2:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.88 | $7.00 | — |
| 266 | GPT-5.3-Codex | `openai/gpt-5.3-codex` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $1.75 | $14.00 | — |
| 267 | GPT-5.4 | `openai/gpt-5.4` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $2.50 | $15.00 | — |
| 268 | GPT-5.4 Image 2 | `openai/gpt-5.4-image-2` | n/a | n/a | n/a (no SWE-bench Verified score) | 272K | $8.00 | $15.00 | — |
| 269 | GPT-5.4 Mini | `openai/gpt-5.4-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.75 | $4.50 | — |
| 270 | GPT-5.4 Mini (batch) | `openai/gpt-5.4-mini:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.38 | $2.25 | — |
| 271 | GPT-5.4 Nano | `openai/gpt-5.4-nano` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.20 | $1.25 | — |
| 272 | GPT-5.4 Nano (batch) | `openai/gpt-5.4-nano:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.10 | $0.63 | — |
| 273 | GPT-5.4 Pro | `openai/gpt-5.4-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $30.00 | $180.00 | — |
| 274 | GPT-5.4 Pro (batch) | `openai/gpt-5.4-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $15.00 | $90.00 | — |
| 275 | GPT-5.4 (batch) | `openai/gpt-5.4:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $1.25 | $7.50 | — |
| 276 | GPT-5.5 | `openai/gpt-5.5` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $5.00 | $30.00 | — |
| 277 | GPT-5.5 Pro | `openai/gpt-5.5-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $30.00 | $180.00 | — |
| 278 | GPT-5.5 Pro (batch) | `openai/gpt-5.5-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $15.00 | $90.00 | — |
| 279 | GPT-5.5 (batch) | `openai/gpt-5.5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $2.50 | $15.00 | — |
| 280 | GPT-5.6 Luna Pro | `openai/gpt-5.6-luna-pro` | >≈ Opus 5 | n/a | n/a (no SWE-bench Verified score) | 1.1M | $0.20 | $1.20 | implement + plan + research + debug + audit + refactor + test |
| 281 | GPT-5.6 Luna Pro (batch) | `openai/gpt-5.6-luna-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $0.10 | $0.60 | — |
| 282 | GPT-5.6 Luna (batch) | `openai/gpt-5.6-luna:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $0.10 | $0.60 | — |
| 283 | GPT-5.6 Sol Pro | `openai/gpt-5.6-sol-pro` | >≈ Opus 5 | n/a | n/a (no SWE-bench Verified score) | 1.1M | $2.00 | $10.00 | implement + plan + research + debug + audit + refactor + test |
| 284 | GPT-5.6 Sol Pro (batch) | `openai/gpt-5.6-sol-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $1.00 | $5.00 | — |
| 285 | GPT-5.6 Sol (batch) | `openai/gpt-5.6-sol:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $1.00 | $5.00 | — |
| 286 | GPT-5.6 Terra Pro | `openai/gpt-5.6-terra-pro` | >≈ Opus 5 | n/a | n/a (no SWE-bench Verified score) | 1.1M | $2.00 | $12.00 | implement + plan + research + debug + audit + refactor + test |
| 287 | GPT-5.6 Terra Pro (batch) | `openai/gpt-5.6-terra-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $1.00 | $6.00 | — |
| 288 | GPT-5.6 Terra (batch) | `openai/gpt-5.6-terra:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $1.00 | $6.00 | — |
| 289 | GPT-5 (batch) | `openai/gpt-5:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.63 | $5.00 | — |
| 290 | GPT-6 Astra | `openai/gpt-6-astra` | >≈ Opus 5 | n/a | n/a (no SWE-bench Verified score) | 1.1M | $10.00 | $50.00 | implement + plan + research + debug + audit + refactor + test |
| 291 | GPT-6 Astra Pro | `openai/gpt-6-astra-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $10.00 | $50.00 | — |
| 292 | GPT-6 Astra Pro (batch) | `openai/gpt-6-astra-pro:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $5.00 | $25.00 | — |
| 293 | GPT-6 Astra (batch) | `openai/gpt-6-astra:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $5.00 | $25.00 | — |
| 294 | GPT Audio | `openai/gpt-audio` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.50 | $10.00 | — |
| 295 | GPT Audio Mini | `openai/gpt-audio-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.60 | $2.40 | — |
| 296 | GPT Chat Latest | `openai/gpt-chat-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $5.00 | $30.00 | — |
| 297 | gpt-oss-120b | `openai/gpt-oss-120b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.04 | $0.17 | — |
| 298 | gpt-oss-120b (batch) | `openai/gpt-oss-120b:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.15 | $0.60 | — |
| 299 | gpt-oss-20b | `openai/gpt-oss-20b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.03 | $0.13 | — |
| 300 | gpt-oss-20b (batch) | `openai/gpt-oss-20b:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.05 | $0.20 | — |
| 301 | gpt-oss-safeguard-20b | `openai/gpt-oss-safeguard-20b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.08 | $0.30 | — |
| 302 | o1 | `openai/o1` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $15.00 | $60.00 | — |
| 303 | o1-pro | `openai/o1-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $150.00 | $600.00 | — |
| 304 | o3 | `openai/o3` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $2.00 | $8.00 | — |
| 305 | o3 Mini | `openai/o3-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $1.10 | $4.40 | — |
| 306 | o3 Mini High | `openai/o3-mini-high` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $1.10 | $4.40 | — |
| 307 | o3 Mini (batch) | `openai/o3-mini:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $0.55 | $2.20 | — |
| 308 | o3 Pro | `openai/o3-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $20.00 | $80.00 | — |
| 309 | o3 (batch) | `openai/o3:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $1.00 | $4.00 | — |
| 310 | o4 Mini | `openai/o4-mini` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $1.10 | $4.40 | — |
| 311 | o4 Mini High | `openai/o4-mini-high` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $1.10 | $4.40 | — |
| 312 | o4 Mini (batch) | `openai/o4-mini:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $0.55 | $2.20 | — |
| 313 | Auto Router | `openrouter/auto` | n/a | n/a | n/a (no SWE-bench Verified score) | 2M | $-1000000.00 | $-1000000.00 | — |
| 314 | Auto Router (Beta) | `openrouter/auto-beta` | n/a | n/a | n/a (no SWE-bench Verified score) | 2M | $-1000000.00 | $-1000000.00 | — |
| 315 | Body Builder (beta) | `openrouter/bodybuilder` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $-1000000.00 | $-1000000.00 | — |
| 316 | Free Models Router | `openrouter/free` | n/a | n/a | n/a (free) | 200K | $0.00 | $0.00 | — |
| 317 | Fusion | `openrouter/fusion` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $-1000000.00 | $-1000000.00 | — |
| 318 | Pareto Code Router | `openrouter/pareto-code` | n/a | n/a | n/a (no SWE-bench Verified score) | 2M | $-1000000.00 | $-1000000.00 | — |
| 319 | Perceptron Mk1 | `perceptron/perceptron-mk1` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.15 | $1.50 | — |
| 320 | Sonar | `perplexity/sonar` | n/a | n/a | n/a (no SWE-bench Verified score) | 127K | $1.00 | $1.00 | — |
| 321 | Sonar Deep Research | `perplexity/sonar-deep-research` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.00 | $8.00 | — |
| 322 | Sonar Pro | `perplexity/sonar-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $3.00 | $15.00 | — |
| 323 | Sonar Pro Search | `perplexity/sonar-pro-search` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $3.00 | $15.00 | — |
| 324 | Sonar Reasoning Pro | `perplexity/sonar-reasoning-pro` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $2.00 | $8.00 | — |
| 325 | Laguna S 2.1 | `poolside/laguna-s-2.1` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 1M | $0.09 | $0.18 | implement + plan + debug + refactor + test |
| 326 | Poolside Laguna S 2.1 | `poolside/laguna-s-2.1:free` | <≈ Haiku 4.5 | n/a | n/a (free) | 262K | $0.00 | $0.00 | implement + plan + debug + refactor + test |
| 327 | Laguna XS 2.1 | `poolside/laguna-xs-2.1` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 262K | $0.06 | $0.12 | implement + plan + debug + refactor + test |
| 328 | Poolside Laguna XS 2.1 | `poolside/laguna-xs-2.1:free` | <≈ Haiku 4.5 | 70.9% [observation_only]s | n/a (free) | 262K | $0.00 | $0.00 | implement + plan + debug + refactor + test |
| 329 | Qwen2.5 72B Instruct | `qwen/qwen-2.5-72b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.36 | $0.40 | — |
| 330 | Qwen2.5 7B Instruct | `qwen/qwen-2.5-7b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.10 | $0.20 | — |
| 331 | Qwen2.5 Coder 32B Instruct | `qwen/qwen-2.5-coder-32b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.66 | $1.00 | — |
| 332 | Qwen-Plus | `qwen/qwen-plus` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.26 | $0.78 | — |
| 333 | Qwen Plus 0728 | `qwen/qwen-plus-2025-07-28` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.26 | $0.78 | — |
| 334 | Qwen2.5 VL 72B Instruct | `qwen/qwen2.5-vl-72b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 128K | $0.80 | $1.00 | — |
| 335 | Qwen3 14B | `qwen/qwen3-14b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.12 | $0.24 | — |
| 336 | Qwen3 235B A22B | `qwen/qwen3-235b-a22b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.46 | $1.82 | — |
| 337 | Qwen3 235B A22B Instruct 2507 | `qwen/qwen3-235b-a22b-2507` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 262K | $0.09 | $0.35 | implement + debug + refactor + test |
| 338 | Qwen3 235B A22B Thinking 2507 | `qwen/qwen3-235b-a22b-thinking-2507` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.23 | $2.30 | — |
| 339 | Qwen3 30B A3B | `qwen/qwen3-30b-a3b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.12 | $0.50 | — |
| 340 | Qwen3 30B A3B Instruct 2507 | `qwen/qwen3-30b-a3b-instruct-2507` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.05 | $0.19 | — |
| 341 | Qwen3 30B A3B Thinking 2507 | `qwen/qwen3-30b-a3b-thinking-2507` | n/a | n/a | n/a (no SWE-bench Verified score) | 82K | $0.20 | $2.40 | — |
| 342 | Qwen3 32B | `qwen/qwen3-32b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.08 | $0.28 | — |
| 343 | Qwen3 8B | `qwen/qwen3-8b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.12 | $0.46 | — |
| 344 | Qwen3 Coder 30B A3B Instruct | `qwen/qwen3-coder-30b-a3b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.07 | $0.28 | — |
| 345 | Qwen3 Coder Flash | `qwen/qwen3-coder-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.20 | $0.98 | — |
| 346 | Qwen3 Coder Next | `qwen/qwen3-coder-next` | ≈ Haiku 4.5 | 70.6% [observation_only]s | n/a (observation only) | 262K | $0.12 | $0.80 | implement + debug + refactor + test |
| 347 | Qwen3 Coder Plus | `qwen/qwen3-coder-plus` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.65 | $3.25 | — |
| 348 | Qwen3 Max | `qwen/qwen3-max` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.78 | $3.90 | — |
| 349 | Qwen3 Max Thinking | `qwen/qwen3-max-thinking` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.78 | $3.90 | — |
| 350 | Qwen3 Next 80B A3B Instruct | `qwen/qwen3-next-80b-a3b-instruct` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 262K | $0.09 | $1.10 | implement + debug + refactor + test |
| 351 | Qwen3 Next 80B A3B Thinking | `qwen/qwen3-next-80b-a3b-thinking` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.15 | $1.20 | — |
| 352 | Qwen3 VL 235B A22B Instruct | `qwen/qwen3-vl-235b-a22b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.21 | $1.90 | — |
| 353 | Qwen3 VL 235B A22B Thinking | `qwen/qwen3-vl-235b-a22b-thinking` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.40 | $4.00 | — |
| 354 | Qwen3 VL 30B A3B Instruct | `qwen/qwen3-vl-30b-a3b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.15 | $0.60 | — |
| 355 | Qwen3 VL 30B A3B Thinking | `qwen/qwen3-vl-30b-a3b-thinking` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.20 | $2.40 | — |
| 356 | Qwen3 VL 32B Instruct | `qwen/qwen3-vl-32b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.10 | $0.42 | — |
| 357 | Qwen3 VL 8B Instruct | `qwen/qwen3-vl-8b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.12 | $0.46 | — |
| 358 | Qwen3 VL 8B Thinking | `qwen/qwen3-vl-8b-thinking` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.18 | $2.10 | — |
| 359 | Qwen3.5-122B-A10B | `qwen/qwen3.5-122b-a10b` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.26 | $2.08 | — |
| 360 | Qwen3.5-27B | `qwen/qwen3.5-27b` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.20 | $1.56 | — |
| 361 | Qwen3.5-35B-A3B | `qwen/qwen3.5-35b-a3b` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.31 | $1.25 | — |
| 362 | Qwen3.5 397B A17B | `qwen/qwen3.5-397b-a17b` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.55 | $3.50 | — |
| 363 | Qwen3.5-9B | `qwen/qwen3.5-9b` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.10 | $0.15 | — |
| 364 | Qwen3.5-9B (batch) | `qwen/qwen3.5-9b:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.17 | $0.25 | — |
| 365 | Qwen3.5-Flash | `qwen/qwen3.5-flash-02-23` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.07 | $0.26 | — |
| 366 | Qwen3.5 Plus 2026-02-15 | `qwen/qwen3.5-plus-02-15` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.26 | $1.56 | — |
| 367 | Qwen3.5 Plus 2026-04-20 | `qwen/qwen3.5-plus-20260420` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.30 | $1.80 | — |
| 368 | Qwen3.6 27B | `qwen/qwen3.6-27b` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.30 | $2.00 | — |
| 369 | Qwen3.6 35B A3B | `qwen/qwen3.6-35b-a3b` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.10 | $0.90 | — |
| 370 | Qwen3.6 Flash | `qwen/qwen3.6-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.19 | $1.13 | — |
| 371 | Qwen3.6 Max Preview | `qwen/qwen3.6-max-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $1.03 | $6.16 | — |
| 372 | Qwen3.6 Plus | `qwen/qwen3.6-plus` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.33 | $1.95 | — |
| 373 | Qwen3.7 Flash | `qwen/qwen3.7-flash` | ≈ Sonnet 5 | n/a | n/a (no SWE-bench Verified score) | 1M | $0.03 | $0.13 | implement + debug + refactor + test |
| 374 | Qwen3.7 Plus | `qwen/qwen3.7-plus` | ≈ Sonnet 5 | n/a | n/a (no SWE-bench Verified score) | 1M | $0.32 | $1.28 | implement + plan + debug + refactor + test |
| 375 | Qwen3.8 2.4T A95B | `qwen/qwen3.8-2.4t-a95b` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.00 | $6.00 | — |
| 376 | Qwen3.8 2.4T A95B (batch) | `qwen/qwen3.8-2.4t-a95b:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.00 | $6.00 | — |
| 377 | Qwen3.8 27B | `qwen/qwen3.8-27b` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.21 | $2.55 | — |
| 378 | Qwen3.8 Flash | `qwen/qwen3.8-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.15 | $0.47 | — |
| 379 | Reka Edge | `rekaai/reka-edge` | n/a | n/a | n/a (no SWE-bench Verified score) | 16K | $0.10 | $0.10 | — |
| 380 | Reka Flash 3 | `rekaai/reka-flash-3` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $0.10 | $0.20 | — |
| 381 | Relace Apply 3 | `relace/relace-apply-3` | n/a | n/a | n/a (no SWE-bench Verified score) | 256K | $0.85 | $1.25 | — |
| 382 | Relace Search | `relace/relace-search` | n/a | n/a | n/a (no SWE-bench Verified score) | 256K | $1.00 | $3.00 | — |
| 383 | Fugu Max | `sakana/fugu-max` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.00 | $6.00 | — |
| 384 | Fugu Ultra | `sakana/fugu-ultra` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $5.00 | $30.00 | — |
| 385 | Fugu Ultra v2 | `sakana/fugu-ultra-v2` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $5.00 | $30.00 | — |
| 386 | Sakana Namazu | `sakana/sakana-namazu` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.95 | $4.00 | — |
| 387 | Llama 3 8B Lunaris | `sao10k/l3-lunaris-8b` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.04 | $0.05 | — |
| 388 | Llama 3.1 Euryale 70B v2.2 | `sao10k/l3.1-euryale-70b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.85 | $0.85 | — |
| 389 | Llama 3.3 Euryale 70B | `sao10k/l3.3-euryale-70b` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.65 | $0.75 | — |
| 390 | Step 3.5 Flash | `stepfun/step-3.5-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.10 | $0.30 | — |
| 391 | Step 3.7 Flash | `stepfun/step-3.7-flash` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.20 | $1.15 | — |
| 392 | Hunyuan A13B Instruct | `tencent/hunyuan-a13b-instruct` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.14 | $0.57 | — |
| 393 | Hy-MT2-1.8B | `tencent/hy-mt2-1.8b` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.04 | $0.18 | — |
| 394 | Hy-MT2-30B-A3B | `tencent/hy-mt2-30b-a3b` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.07 | $0.30 | — |
| 395 | Hy-MT2-7B | `tencent/hy-mt2-7b` | n/a | n/a | n/a (no SWE-bench Verified score) | 8K | $0.07 | $0.30 | — |
| 396 | Tencent Hy3 | `tencent/hy3` | ≈ Haiku 4.5 | 78.0% [observation_only]s | n/a (observation only) | 262K | $0.13 | $0.53 | implement + plan + debug + refactor + test |
| 397 | Hy3 preview | `tencent/hy3-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 262K | $0.18 | $0.60 | — |
| 398 | Hy4 preview | `tencent/hy4-preview` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.83 | $2.50 | — |
| 399 | Cydonia 24B V4.1 | `thedrummer/cydonia-24b-v4.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.30 | $0.50 | — |
| 400 | Skyfall 36B V2 | `thedrummer/skyfall-36b-v2` | n/a | n/a | n/a (no SWE-bench Verified score) | 33K | $0.55 | $0.80 | — |
| 401 | UnslopNemo 12B | `thedrummer/unslopnemo-12b` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.40 | $0.40 | — |
| 402 | Thinking Machines: Inkling | `thinkingmachines/inkling` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.00 | $4.05 | — |
| 403 | Thinking Machines: Inkling Small | `thinkingmachines/inkling-small` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.45 | $1.20 | — |
| 404 | Thinking Machines: Inkling Small (batch) | `thinkingmachines/inkling-small:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 524K | $0.50 | $1.20 | — |
| 405 | Thinking Machines: Inkling Small (free) | `thinkingmachines/inkling-small:free` | n/a | n/a | n/a (free) | 1M | $0.00 | $0.00 | — |
| 406 | Thinking Machines: Inkling (batch) | `thinkingmachines/inkling:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 524K | $1.00 | $4.05 | — |
| 407 | Thinking Machines: Inkling (free) | `thinkingmachines/inkling:free` | n/a | n/a | n/a (free) | 1M | $0.00 | $0.00 | — |
| 408 | ReMM SLERP 13B | `undi95/remm-slerp-l2-13b` | n/a | n/a | n/a (no SWE-bench Verified score) | 6K | $0.35 | $0.65 | — |
| 409 | Solar Pro 3 | `upstage/solar-pro-3` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.15 | $0.60 | — |
| 410 | Solar Pro 4 | `upstage/solar-pro4` | n/a | n/a | n/a (no SWE-bench Verified score) | 524K | $0.09 | $0.36 | — |
| 411 | Palmyra X5 | `writer/palmyra-x5` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.60 | $6.00 | — |
| 412 | SpaceXAI: Grok 4.20 | `x-ai/grok-4.20` | n/a | n/a | n/a (no SWE-bench Verified score) | 2M | $1.25 | $2.50 | — |
| 413 | SpaceXAI: Grok 4.20 Multi-Agent | `x-ai/grok-4.20-multi-agent` | n/a | n/a | n/a (no SWE-bench Verified score) | 2M | $1.25 | $2.50 | — |
| 414 | SpaceXAI: Grok 4.3 | `x-ai/grok-4.3` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.25 | $2.50 | — |
| 415 | SpaceXAI: Grok 4.3 (batch) | `x-ai/grok-4.3:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $1.00 | $2.00 | — |
| 416 | SpaceXAI: Grok Build 0.1 | `x-ai/grok-build-0.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 256K | $1.00 | $2.00 | — |
| 417 | GLM 4.5 | `z-ai/glm-4.5` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.60 | $2.20 | — |
| 418 | GLM 4.5 Air | `z-ai/glm-4.5-air` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.13 | $0.85 | — |
| 419 | GLM 4.5V | `z-ai/glm-4.5v` | n/a | n/a | n/a (no SWE-bench Verified score) | 66K | $0.60 | $1.80 | — |
| 420 | GLM 4.6 | `z-ai/glm-4.6` | n/a | n/a | n/a (no SWE-bench Verified score) | 205K | $0.43 | $1.75 | — |
| 421 | GLM 4.6V | `z-ai/glm-4.6v` | n/a | n/a | n/a (no SWE-bench Verified score) | 131K | $0.30 | $0.90 | — |
| 422 | GLM 4.7 Flash | `z-ai/glm-4.7-flash` | ≈ Haiku 4.5 | n/a | n/a (no SWE-bench Verified score) | 200K | $0.06 | $0.40 | implement + test |
| 423 | GLM 5 | `z-ai/glm-5` | n/a | n/a | n/a (no SWE-bench Verified score) | 205K | $0.60 | $1.92 | — |
| 424 | GLM 5 Turbo | `z-ai/glm-5-turbo` | n/a | n/a | n/a (no SWE-bench Verified score) | 203K | $1.20 | $4.00 | — |
| 425 | GLM 5.1 | `z-ai/glm-5.1` | n/a | n/a | n/a (no SWE-bench Verified score) | 205K | $0.97 | $3.04 | — |
| 426 | GLM 5.2 (batch) | `z-ai/glm-5.2:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.70 | $2.20 | — |
| 427 | GLM 5.3 Flash (batch) | `z-ai/glm-5.3-flash:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.08 | $0.25 | — |
| 428 | GLM 5.3 (batch) | `z-ai/glm-5.3:batch` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.70 | $2.20 | — |
| 429 | GLM 5V Turbo | `z-ai/glm-5v-turbo` | n/a | n/a | n/a (no SWE-bench Verified score) | 203K | $1.20 | $4.00 | — |
| 430 | Anthropic: Claude Fable Latest | `~anthropic/claude-fable-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $10.00 | $50.00 | — |
| 431 | Anthropic: Claude Haiku Latest | `~anthropic/claude-haiku-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 200K | $1.00 | $5.00 | — |
| 432 | Anthropic: Claude Opus Latest | `~anthropic/claude-opus-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $5.00 | $25.00 | — |
| 433 | Anthropic: Claude Sonnet Latest | `~anthropic/claude-sonnet-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.00 | $10.00 | — |
| 434 | DeepSeek: DeepSeek V4 Flash Latest | `~deepseek/deepseek-v4-flash-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.3M | $0.04 | $0.10 | — |
| 435 | Google: Gemini Flash Latest | `~google/gemini-flash-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $0.75 | $3.75 | — |
| 436 | Google: Gemini Pro Latest | `~google/gemini-pro-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.00 | $12.00 | — |
| 437 | MoonshotAI: Kimi Latest | `~moonshotai/kimi-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1M | $2.10 | $10.95 | — |
| 438 | OpenAI: GPT Astra Latest | `~openai/gpt-astra-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $10.00 | $50.00 | — |
| 439 | OpenAI: GPT Luna Latest | `~openai/gpt-luna-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $0.20 | $1.20 | — |
| 440 | OpenAI: GPT Mini Latest | `~openai/gpt-mini-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 400K | $0.75 | $4.50 | — |
| 441 | OpenAI: GPT Sol Latest | `~openai/gpt-sol-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $2.00 | $10.00 | — |
| 442 | OpenAI: GPT Terra Latest | `~openai/gpt-terra-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.1M | $2.00 | $12.00 | — |
| 443 | xAI: Grok Latest | `~x-ai/grok-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 500K | $2.00 | $6.00 | — |
| 444 | Z.ai: GLM Flash Latest | `~z-ai/glm-flash-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.3M | $0.08 | $0.25 | — |
| 445 | Z.ai: GLM Latest | `~z-ai/glm-latest` | n/a | n/a | n/a (no SWE-bench Verified score) | 1.3M | $0.92 | $3.14 | — |

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

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Copyright |
|---|---|---|---|---|---|---|---|---|---|
| GPT-5.6 Luna | `openai/gpt-5.6-luna` | $0.20 ($0.40 от 272K+) | $1.20 ($1.80 от 272K+) | 1.1M | 93.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 207 | OpenAI (C) | нет | unknown |
| GPT-5.6 Sol | `openai/gpt-5.6-sol` | $2.00 ($4.00 от 272K+) | $10.00 ($15.00 от 272K+) | 1.1M | 96.2% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 24.1 | OpenAI (C) | нет | unknown |
| GPT-5.6 Terra | `openai/gpt-5.6-terra` | $2.00 ($4.00 от 272K+) | $12.00 ($18.00 от 272K+) | 1.1M | 95.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 21.2 | _нужен обзор_ | _нужен обзор_ | unknown |
| Claude Opus 5 | `anthropic/claude-opus-5` | $5.00 | $25.00 | 1M | 97.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 9.7 | _нужен обзор_ | _нужен обзор_ | unknown |
| Claude Opus 4.8 | `anthropic/claude-opus-4.8` | $5.00 | $25.00 | 1M | 88.6% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 8.9 | _нужен обзор_ | _нужен обзор_ | unknown |
| Claude Opus 4.7 | `anthropic/claude-opus-4.7` | $5.00 | $25.00 | 1M | 82.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 8.2 | _нужен обзор_ | _нужен обзор_ | unknown |
| Claude Opus 4.6 | `anthropic/claude-opus-4.6` | $5.00 | $25.00 | 1M | 75.6% · [swebench.com](https://www.swebench.com/), 2026-02-17 | 7.6 | _нужен обзор_ | _нужен обзор_ | unknown |
| Claude Fable 5 | `anthropic/claude-fable-5` | $10.00 | $50.00 | 1M | 95.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 4.8 | _нужен обзор_ | _нужен обзор_ | unknown |
| GPT-5.6 Luna Pro | `openai/gpt-5.6-luna-pro` | $0.20 ($0.40 от 272K+) | $1.20 ($1.80 от 272K+) | 1.1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| GPT-5.6 Sol Pro | `openai/gpt-5.6-sol-pro` | $2.00 ($4.00 от 272K+) | $10.00 ($15.00 от 272K+) | 1.1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| GPT-5.6 Terra Pro | `openai/gpt-5.6-terra-pro` | $2.00 ($4.00 от 272K+) | $12.00 ($18.00 от 272K+) | 1.1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Claude Fable 5.1 | `anthropic/claude-fable-5.1` | $10.00 | $50.00 | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| GPT-6 Astra | `openai/gpt-6-astra` | $10.00 ($20.00 от 272K+) | $50.00 ($75.00 от 272K+) | 1.1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |

**Заметки**

- **GPT-5.6 Luna** (`openai/gpt-5.6-luna`) — Оценка независимая, vals.ai. Сейчас лучшая цена/качество в Opus-тире.
- **GPT-5.6 Sol** (`openai/gpt-5.6-sol`) — Ближе всего к Opus 5 (97.0%) по сырой оценке, 96.2% подтверждены на vals.ai. Оговорка про METR уточнена: его находка про эксплуатацию багов eval'ов относится к **другому** бенчмарку — собственному agentic time-horizon eval'у METR на харнессе Terminal-Bench 2.1/ReAct, где у Sol самый высокий когда-либо измеренный уровень «читинга»; ни один отчёт не связывает это конкретно с числом 96.2% SWE-bench Verified. Отдельный аудит UC Berkeley RDI (апрель 2026, до выхода Sol) показал, что бенчмарки типа SWE-bench Verified в принципе поддаются накрутке харнесс-трюком — это не про Sol персонально. Общая осторожность к eval-результатам Sol сохраняется.

### ≈ Sonnet 5

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Copyright |
|---|---|---|---|---|---|---|---|---|---|
| MiniMax M3 | `minimax/minimax-m3` | $0.30 | $1.20 | 1M | 75.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 143 | MiniMax (n/a) | **да** (кастомная лицензия, коммерч. ограничения) | unknown |
| GLM-5.2 | `z-ai/glm-5.2` | $0.68 | $2.15 | 1M | 82.8% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 78.9 | Z.ai / Zhipu AI (D−) | **да, MIT** | unknown |
| Gemini 3.7 Flash | `google/gemini-3.7-flash` | $0.75 | $3.75 | 1M | 80.8% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 53.9 | _нужен обзор_ | _нужен обзор_ | unknown |
| Gemini 3.8 Flash | `google/gemini-3.8-flash` | $0.75 | $3.75 | 1M | 80.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 53.3 | _нужен обзор_ | _нужен обзор_ | unknown |
| Gemini 3.6 Flash | `google/gemini-3.6-flash` | $0.75 | $3.75 | 1M | 79.6% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 53.1 | Google DeepMind (C) | нет | unknown |
| GLM 5.3 | `z-ai/glm-5.3` | $1.40 | $4.40 | 1.3M | 95.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 44.4 | _нужен обзор_ | _нужен обзор_ | unknown |
| Meta Muse Spark 1.1 | `meta/muse-spark-1.1` | $1.25 | $4.25 | 1M | 82.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 41.0 | Meta (D+) | нет | unknown |
| DeepSeek V4 Pro | `deepseek/deepseek-v4-pro` | $1.60 | $3.20 | 1M | 77.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 38.7 | DeepSeek (F) | **да, MIT** | unknown |
| SpaceXAI: Grok 4.6 | `x-ai/grok-4.6` | $2.00 ($4.00 от 200K+) | $6.00 ($12.00 от 200K+) | 500K | 95.6% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 31.9 | _нужен обзор_ | _нужен обзор_ | unknown |
| Qwen3.7 Max | `qwen/qwen3.7-max` | $1.48 | $4.43 | 1M | 68.8% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 31.1 | Alibaba Cloud / Qwen (D−) | нет | unknown |
| Grok 4.5 | `x-ai/grok-4.5` | $2.00 ($4.00 от 200K+) | $6.00 ($12.00 от 200K+) | 500K | 86.6% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 28.9 | xAI (F) | нет | unknown |
| Qwen3.8 Max (0902) | `qwen/qwen3.8-max-0902` | $2.00 | $6.00 | 1M | 85.6% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 28.5 | _нужен обзор_ | _нужен обзор_ | unknown |
| Gemini 3.5 Flash | `google/gemini-3.5-flash` | $1.50 | $9.00 | 1M | 78.8% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 23.3 | _нужен обзор_ | _нужен обзор_ | unknown |
| Mistral Medium 3.5 | `mistralai/mistral-medium-3-5` | $1.50 | $7.50 | 262K | 66.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 22.1 | Mistral AI (F) | **да** (модиф. MIT, лицензия нужна при выручке >$20M/мес) | unknown |
| Claude Sonnet 5 | `anthropic/claude-sonnet-5` | $2.00 | $10.00 | 1M | 79.6% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 19.9 | _нужен обзор_ | _нужен обзор_ | unknown |
| Kimi K3 | `moonshotai/kimi-k3` | $2.65 | $13.28 | 1M | 93.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 17.6 | Moonshot AI (n/a) | **да** (кастом. лицензия, свободно до 100M MAU) | unknown |
| Gemini 3.1 Pro Preview | `google/gemini-3.1-pro-preview` | $2.00 ($4.00 от 200K+) | $12.00 ($18.00 от 200K+) | 1M | 78.8% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 17.5 | Google DeepMind (C) | нет | unknown |
| Claude Sonnet 4.6 | `anthropic/claude-sonnet-4.6` | $3.00 | $15.00 | 1M | 77.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 12.9 | _нужен обзор_ | _нужен обзор_ | unknown |
| Claude Sonnet 4.5 | `anthropic/claude-sonnet-4.5` | $3.00 ($6.00 от 200K+) | $15.00 ($22.50 от 200K+) | 1M | 74.8% · [swebench.com](https://www.swebench.com/), 2025-11-03 | 12.5 | _нужен обзор_ | _нужен обзор_ | unknown |
| Qwen3.7 Flash | `qwen/qwen3.7-flash` | $0.03 ($0.10 от 32K+) | $0.13 ($0.40 от 32K+) | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Qwen3.7 Plus | `qwen/qwen3.7-plus` | $0.32 ($0.96 от 256K+) | $1.28 ($3.84 от 256K+) | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |

**Заметки**

- **MiniMax M3** (`minimax/minimax-m3`) — Лучшая цена/качество в этом тире. Оценка теперь независимая — vals.ai показывает 75.0%, заметно ниже прежней вендорской заявки блога minimax.io (80.5%), которая снята. У GMICloud есть маршрут дешевле типовой цены каталога.
- **GLM-5.2** (`z-ai/glm-5.2`) — Официальная документация Z.AI SWE-bench Verified не публикует — там только SWE-bench Pro 62.1% и Terminal-Bench 2.1 81.0% (другие метрики). Независимая оценка SWE-bench Verified есть на vals.ai (82.8%). Модель по-прежнему сильна в агентных задачах.
- **Gemini 3.6 Flash** (`google/gemini-3.6-flash`) — Независимая оценка SWE-bench Verified теперь есть на vals.ai (79.6%) — на официальной странице Google DeepMind по-прежнему только **SWE-bench Pro 58.7% (другая метрика)**. Прошлая версия использовала прокси-оценку ~78%, взятую у предшественника (Gemini 3 Flash Preview, замену которого 3.6 Flash собой представляет): по правилу «оценка другого продукта не переносится» она в ранжирование не пошла — в «Качество/цена» теперь идёт независимое число с vals.ai. GitHub Copilot отключает старый slug (Gemini 3 Flash, вместе с Gemini 2.5 Pro) 2026-07-31 — подтверждено по changelog GitHub.
- **Meta Muse Spark 1.1** (`meta/muse-spark-1.1`) — Флагман Meta вместо бренда Llama; 82.0% теперь подтверждены независимо на vals.ai — выше прежней заявки самой Meta (77.4%), которая снята. Цифру SWE-bench Verified Hard 42.9% **не удалось проверить на 2026-07-30** (исходный PDF не читается, часть обзоров её вообще не упоминает) — оставлена с прошлой версии; сторонние обзоры дают только SWE-bench Pro 52.4–61.5% и Terminal-Bench.
- **DeepSeek V4 Pro** (`deepseek/deepseek-v4-pro`) — Заявленная оценка ~80.6% измерена для варианта «V4-Pro-Max», которого на OpenRouter не существует — это не тот продукт, что продаётся под `deepseek/deepseek-v4-pro`. Для реального продукта теперь есть независимая оценка на vals.ai (77.4%).
- **Qwen3.7 Max** (`qwen/qwen3.7-max`) — Независимая оценка на vals.ai теперь есть — 68.8%, заметно ниже прежней вендорской заявки 80.4% (снята). Qwen 3.8-Max, анонсированный превью 2026-07-19, с тех пор доехал до продукта на OpenRouter (слаг переименован в `qwen/qwen3.8-max-0902`) с собственной независимой оценкой vals.ai 85.6% — заметно выше 3.7 Max, стоит рассмотреть на замену рекомендации.
- **Grok 4.5** (`x-ai/grok-4.5`) — 86.6% подтверждены независимо (vals.ai), высокая достоверность. Реально близко к Opus по агентным задачам (SWE-bench Pro 64.7% — другая метрика; лидирует в SWE Marathon); свыше 200K токенов — $4/$12, кэш-чтение $0.60.
- **Mistral Medium 3.5** (`mistralai/mistral-medium-3-5`) — Теперь основная рекомендация Mistral для агентного кодинга — заменила Devstral 2 по умолчанию в их Vibe CLI. Независимая оценка на vals.ai теперь есть — 66.4%, заметно ниже прежней заявки самой Mistral в пересказе прессы (77.6%), которая снята.
- **Kimi K3** (`moonshotai/kimi-k3`) — **Основное число изменено в этом обновлении**: 93.4% взяты с живого независимого лидерборда vals.ai (ранг #4, обновление 2026-07-22, продукт совпадает точно). Прошлая версия документа показывала 76.8%, но источник этого числа **не удалось отследить на 2026-07-30** — нашлись только другие метрики (Toolathlon-Verified 76.5%, FrontierSWE 81.2%, DeepSWE 67.5%, SWE Marathon 42.0%, Program Bench 77.8%), ни одна из них не «76.8% SWE-bench Verified». По правилу «независимое измерение приоритетнее вендорского/неотслеживаемого» в ранжирование пошло 93.4% (качество/цена 15.6 вместо прежних 12.8); 76.8% сохранено здесь как альтернативная цифра с неподтверждённым источником. Расхождение такого размера у этой модели правдоподобно объясняется скаффолдом: на Terminal-Bench 2.1 у неё же 88.3% на харнессе Moonshot против 80.9% на прогоне vals.ai. По сырой оценке 93.4% модель уже на уровне тира >≈ Claude Opus 5 (у Luna — 93.0%), но оценка спорная, а цена равна полному прайсу Sonnet 5 без скидки — строка оставлена в этом тире; если 93.4% подтвердится вторым независимым источником, её следует перенести в >≈ Claude Opus 5.
- **Gemini 3.1 Pro Preview** (`google/gemini-3.1-pro-preview`) — Всё ещё preview — стабильного Gemini Pro на замену по-прежнему нет: Google подтвердила 2026-07-21, что релиз не готов (цель 17 июля сорвана), и выпускает вместо него Flash-модели. Независимая оценка на vals.ai — 78.8%, немного ниже прежнего числа 80.6%, которое снято.

### <≈ Haiku 4.5

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark score | Quality / price | Владелец (FLI) | Открытые веса | Copyright |
|---|---|---|---|---|---|---|---|---|---|
| Xiaomi MiMo-V2.5 | `xiaomi/mimo-v2.5` | $0.14 | $0.28 | 1.1M | 71.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 406 | Xiaomi (n/a) | **да, MIT** | unknown |
| GLM 5.3 Flash | `z-ai/glm-5.3-flash` | $0.15 | $0.50 | 1.3M | 92.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 387 | _нужен обзор_ | _нужен обзор_ | unknown |
| DeepSeek V3.2 | `deepseek/deepseek-v3.2` | $0.27 | $0.40 | 164K | 70.0% · [swebench.com](https://www.swebench.com/), 2026-02-17 | 232 | DeepSeek (F) | **да, MIT** | unknown |
| MiniMax M2.5 | `minimax/minimax-m2.5` | $0.27 | $1.08 | 205K | 74.2% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 157 | _нужен обзор_ | _нужен обзор_ | unknown |
| MiniMax M2 | `minimax/minimax-m2` | $0.26 | $1.02 | 205K | 61.0% · [swebench.com](https://www.swebench.com/), 2025-11-24 | 137 | _нужен обзор_ | _нужен обзор_ | unknown |
| Xiaomi MiMo-V2.5-Pro | `xiaomi/mimo-v2.5-pro` | $0.44 | $0.87 | 1.1M | 74.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 136 | Xiaomi (n/a) | **да, MIT** | unknown |
| Qwen3 Coder (480B) | `qwen/qwen3-coder` | $0.30 | $1.00 | 262K | 55.4% · [swebench.com](https://www.swebench.com/), 2025-08-02 | 117 | Alibaba Cloud / Qwen (D−) | **да, Apache 2.0** | unknown |
| GLM 4.7 | `z-ai/glm-4.7` | $0.40 | $1.75 | 205K | 69.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 94.1 | _нужен обзор_ | _нужен обзор_ | unknown |
| GPT-5 Mini | `openai/gpt-5-mini` | $0.25 | $2.00 | 400K | 58.0% · [swebench.com](https://www.swebench.com/), 2025-08-07..2026-02-17 | 84.4 | OpenAI (C) | нет | unknown |
| Kimi K2.5 | `moonshotai/kimi-k2.5` | $0.45 | $2.25 | 262K | 70.8% · [swebench.com](https://www.swebench.com/), 2026-02-17 | 78.7 | Moonshot AI (n/a) | **да** (модиф. MIT) | unknown |
| Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b` | $0.60 | $2.40 | 262K | 69.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 65.7 | _нужен обзор_ | _нужен обзор_ | unknown |
| Llama 4 Maverick | `meta-llama/llama-4-maverick` | $0.20 | $0.70 | 1M | 21.0% · [swebench.com](https://www.swebench.com/), 2025-07-20 | 64.9 | Meta (D+) | **да** (Llama 4 Community License, ограничения при >700M MAU) | unknown |
| Llama 4 Scout | `meta-llama/llama-4-scout` | $0.10 | $0.30 | 1.3M | 9.1% · [swebench.com](https://www.swebench.com/), 2025-07-20 | 60.4 | _нужен обзор_ | _нужен обзор_ | unknown |
| Kimi K2.7 Code | `moonshotai/kimi-k2.7-code` | $0.71 | $3.50 | 262K | 78.2% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 55.6 | Moonshot AI (n/a) | **да** (модиф. MIT) | unknown |
| Mistral Large 3 | `mistralai/mistral-large-2512` | $0.50 | $1.50 | 262K | 41.4% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | 55.2 | Mistral AI (F) | **да, Apache 2.0** | unknown |
| Claude Haiku 4.5 | `anthropic/claude-haiku-4.5` | $1.00 | $5.00 | 200K | 66.6% · [swebench.com](https://www.swebench.com/), 2026-02-17 | 33.3 | _нужен обзор_ | _нужен обзор_ | unknown |
| Laguna XS 2.1 | `poolside/laguna-xs-2.1` | $0.06 | $0.12 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Nemotron 3 Nano 30B A3B | `nvidia/nemotron-3-nano-30b-a3b` | $0.05 | $0.20 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| DeepSeek V4 Flash | `deepseek/deepseek-v4-flash` | $0.09 | $0.18 | 1M | 88.8% [variant_mismatch] · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | n/a (variant mismatch) | DeepSeek (F) | **да, MIT** | unknown |
| Laguna S 2.1 | `poolside/laguna-s-2.1` | $0.09 | $0.18 | 1M | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Gemma 4 26B A4B | `google/gemma-4-26b-a4b-it` | $0.09 | $0.30 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| GLM 4.7 Flash | `z-ai/glm-4.7-flash` | $0.06 | $0.40 | 200K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Gemma 4 31B | `google/gemma-4-31b-it` | $0.09 | $0.34 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Qwen3 235B A22B Instruct 2507 | `qwen/qwen3-235b-a22b-2507` | $0.09 | $0.35 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Nemotron 3 Super | `nvidia/nemotron-3-super-120b-a12b` | $0.08 | $0.45 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Tencent Hy3 | `tencent/hy3` | $0.13 | $0.53 | 262K | 78.0% [observation_only] · [swebench.com](https://hunyuan.tencent.com/), n/a | n/a (observation only) | Tencent / Hunyuan (n/a) | **да, Apache 2.0** | unknown |
| Qwen3 Coder Next | `qwen/qwen3-coder-next` | $0.12 | $0.80 | 262K | 70.6% [observation_only] · [swebench.com](https://qwenlm.github.io/), n/a | n/a (observation only) | Alibaba Cloud / Qwen (D−) | **да, Apache 2.0** | unknown |
| Qwen3 Next 80B A3B Instruct | `qwen/qwen3-next-80b-a3b-instruct` | $0.09 | $1.10 | 262K | n/a | n/a (no SWE-bench Verified score) | _нужен обзор_ | _нужен обзор_ | unknown |
| Mistral Codestral 2508 | `mistralai/codestral-2508` | $0.30 | $0.90 | 256K | n/a | n/a (no SWE-bench Verified score) | Mistral AI (F) | частично (Mistral AI Non-Production License — только research/testing) | unknown |
| Meituan LongCat 2.0 | `meituan/longcat-2.0` | $0.30 | $1.20 | 1M | n/a | n/a (no SWE-bench Verified score) | Meituan (n/a) | статус не подтверждён | unknown |
| KAT-Coder V2.5 Pro | `kwaipilot/kat-coder-pro-v2.5` | $0.74 | $2.96 | 262K | n/a | n/a (variant mismatch) | Kwaipilot / Kuaishou (n/a) | нет | unknown |

**Заметки**

- **Xiaomi MiMo-V2.5** (`xiaomi/mimo-v2.5`) — Базовая версия MiMo (311B — отдельная модель, не «дешёвый режим» 1T-варианта Pro). Оценки по SWE-bench Verified по-прежнему нет; есть SWE-bench Pro 56.1% (другая метрика). Цена: типовая $0.14/$0.28 — прошлая версия показывала $0.112/$0.224, это маршрут GMICloud (скидка 20%).
- **DeepSeek V3.2** (`deepseek/deepseek-v3.2`) — Независимая медиана на swebench.com теперь есть — 70.0%, внутри прежнего вендорского диапазона по вариантам (67.8% V3.2-Exp — 73.1% V3.2-Speciale), но это уже реальное измерение, а не ручная середина диапазона, которая снята. Цена: типовая в каталоге $0.269/$0.40 — прошлая версия показывала $0.21/$0.31, это скидочный маршрут Baidu ($0.2072/$0.3108, скидка 26%).
- **Xiaomi MiMo-V2.5-Pro** (`xiaomi/mimo-v2.5-pro`) — Независимая оценка на vals.ai теперь есть — 74.0%, что заменяет прежнее число 78.9% (снято), взятое из PR «community evaluation results» в HF-репозитории самой модели — вендор-хостинг чужого прогона, а не чистый сторонний лидерборд. Прежняя цифра SWE-bench Pro 57.2% никуда не делась и остаётся в силе как дополнительная точка (другая метрика, в ранжирование не идёт). Цена: типовая $0.435/$0.87 — прошлая версия показывала $0.348/$0.696, это маршрут GMICloud (скидка 20%). По данным на 2026-07-28 — #1 модель по доле трафика в категории "coding" на OpenRouter (18%).
- **Qwen3 Coder (480B)** (`qwen/qwen3-coder`) — Независимая медиана на swebench.com теперь есть — 55.4%, заметно ниже прежнего вручную взятого диапазона по расходящимся источникам (66.5–72.5%), который снят. Цена: разобрано расхождение прошлой версии — типовая цена каталога $0.30/$1.00, а показанные ранее $0.22/$1.80 относятся к отдельному маршруту Google Vertex (us-south1, без скидки, проверено 2026-07-30). Отсюда же прошлая заметка про «почти удвоившийся выход» — она описывала переход на этот маршрут, а не изменение прайса.
- **GPT-5 Mini** (`openai/gpt-5-mini`) — Старое поколение — актуального "mini" в линейке 5.6 нет; ближайший бюджетный аналог сейчас — GPT-5.6 Luna в тире Opus выше. Сводимой оценки нет и в этой проверке (38% и 48% из источников без ссылок).
- **Kimi K2.5** (`moonshotai/kimi-k2.5`) — Независимая медиана на swebench.com теперь есть — 70.8%, ниже прежней вендорской цифры с официальной карточки Hugging Face (76.8%, non-thinking mode, внутренний eval-фреймворк), которая снята. Путаницы с Kimi K3 (отдельная модель с независимой оценкой 93.4% на vals.ai) по-прежнему нет — это два разных реальных числа для двух разных моделей. Цена реально выросла: $0.375/$2.03 → $0.57/$2.85 (+52% вход / +40% выход); самый дешёвый маршрут сейчас — StreamLake $0.54/$2.70 (скидка 10%).
- **Llama 4 Maverick** (`meta-llama/llama-4-maverick`) — Старое поколение (апрель 2025), больше не флагман Meta (теперь — Muse Spark). Независимая оценка SWE-bench Verified теперь есть на swebench.com (21.0%) — заметно ниже циркулировавших ранее непроверенных чисел (~24%, 49.2%, а 8.0% — вообще по SWE-bench **Lite**).
- **Kimi K2.7 Code** (`moonshotai/kimi-k2.7-code`) — Независимая оценка на vals.ai теперь есть и разрешает прежнюю неоднозначность: 78.2% — это ровно то число, что параллельно циркулировало (automatio.ai), а не конкурирующая цифра 60.4% с неотслеживаемым источником, которая снята. Цена: типовая в каталоге $0.73/$3.50 (два независимых замера каталога 2026-07-30), прямая проверка страницы модели в тот же день дала $0.71/$3.50 — расхождение $0.02, уровня округления/маршрута; ещё один источник ранее указывал $0.95/$4.00 — сверьтесь на странице модели.
- **Mistral Large 3** (`mistralai/mistral-large-2512`) — Универсальный флагман Mistral; преемника (Large 4) на дату проверки не существует, анонсов нет. Независимая оценка SWE-bench Verified есть на vals.ai (41.4%).
- **DeepSeek V4 Flash** (`deepseek/deepseek-v4-flash`) — Старые vendor claims по базовой модели и V4-Flash-Max сохранены как provenance. Живое независимое наблюдение 88.8% относится к deepseek-v4-flash-0731, а не к каталожному deepseek-v4-flash-20260423/0423, поэтому качество этого OpenRouter продукта неизвестно и число не ранжируется.
- **Tencent Hy3** (`tencent/hy3`) — **Строка переехала сюда из таблицы «без независимой оценки»**: оценка появилась впервые — 78.0% для полного релиза (анонс 2026-07-06) против 74.4% у апрельского preview. Обе цифры — **самоотчёт Tencent, сторонней проверки (Artificial Analysis и др.) не найдено, достоверность низкая**; прошлая формулировка «бенчмарков нет» этим отменяется, но принимать 78.0% как измеренную независимо нельзя. #2 по объёму трафика и #1 по числу tool-calls в категории "coding" на OpenRouter — судя по всему уже широко используется в проде. Полный релиз без региональных ограничений — не путать с более ранним ограниченным "Hy3 Preview".
- **Qwen3 Coder Next** (`qwen/qwen3-coder-next`) — Все три числа перепроверены и не изменились. SWE-bench Pro 44.3%, Terminal-Bench 2.0 36.2 — другие метрики, модель слабее в общих терминал-агентных задачах. Цена реально изменилась: вход $0.11 → $0.12 (у StreamLake есть маршрут со «скидкой 40%» — $0.18/$0.90, то есть дороже типовой цены; смысла в нём нет).
- **Mistral Codestral 2508** (`mistralai/codestral-2508`) — Специализация на автодополнении кода (FIM), не general-purpose — сравнивать с остальными некорректно. Единственное встреченное «52%» — низкоавторитетный источник без подтверждения, в таблицу не берётся.
- **Meituan LongCat 2.0** (`meituan/longcat-2.0`) — **Новая строка в этом обновлении** — модель вышла 2026-07-20, то есть до прошлого обновления, но в подборку тогда не попала; slug живой (проверен в каталоге 2026-07-30). Оценок SWE-bench не найдено. По сообщениям прессы (Decrypt, VentureBeat) в июле выходила в топ трафика OpenRouter, но **эта проверка популярность независимо не подтвердила** — относитесь к «широко используется» как к непроверенному. Добавлена по тому же принципу, по которому в подборке раньше держалась Tencent Hy3: живой slug + заявленная заметная популярность при отсутствии бенчмарков.
- **KAT-Coder V2.5 Pro** (`kwaipilot/kat-coder-pro-v2.5`) — **Оценка снята в этом обновлении**, по той же причине, что у Air: 69.4% — это Dev-вариант, своей оценки SWE-bench Verified у Pro-V2.5 нет. Побочная загадка прошлой версии разрешена: вендорская фраза «уступает только Opus 4.8» относится к SWE-bench **Pro** (V2.5: 65.2 против 69.2 у Opus 4.8), а не к Verified — то есть аргументом за какую-либо Verified-оценку она никогда не была. Строка **не участвует в ранжировании**.

## Сколько токенов даст $10

Смешанное соотношение 3:1 (вход:выход) — грубое приближение к типичной нагрузке кодинг-агента, а не гарантия реальных трат. Набор моделей и порядок строк здесь ровно те же, что в разделе выше.

**>≈ Opus 5**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| GPT-5.6 Luna | 50.0 | 8.33 | 22.2 |
| GPT-5.6 Sol | 5.00 | 1.00 | 2.50 |
| GPT-5.6 Terra | 5.00 | 0.83 | 2.22 |
| Claude Opus 5 | 2.00 | 0.40 | 1.00 |
| Claude Opus 4.8 | 2.00 | 0.40 | 1.00 |
| Claude Opus 4.7 | 2.00 | 0.40 | 1.00 |
| Claude Opus 4.6 | 2.00 | 0.40 | 1.00 |
| Claude Fable 5 | 1.00 | 0.20 | 0.50 |
| GPT-5.6 Luna Pro | 50.0 | 8.33 | 22.2 |
| GPT-5.6 Sol Pro | 5.00 | 1.00 | 2.50 |
| GPT-5.6 Terra Pro | 5.00 | 0.83 | 2.22 |
| Claude Fable 5.1 | 1.00 | 0.20 | 0.50 |
| GPT-6 Astra | 1.00 | 0.20 | 0.50 |

**≈ Sonnet 5**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| MiniMax M3 | 33.3 | 8.33 | 19.0 |
| GLM-5.2 | 14.6 | 4.66 | 9.53 |
| Gemini 3.7 Flash | 13.3 | 2.67 | 6.67 |
| Gemini 3.8 Flash | 13.3 | 2.67 | 6.67 |
| Gemini 3.6 Flash | 13.3 | 2.67 | 6.67 |
| GLM 5.3 | 7.14 | 2.27 | 4.65 |
| Meta Muse Spark 1.1 | 8.00 | 2.35 | 5.00 |
| DeepSeek V4 Pro | 6.25 | 3.12 | 5.00 |
| SpaceXAI: Grok 4.6 | 5.00 | 1.67 | 3.33 |
| Qwen3.7 Max | 6.78 | 2.26 | 4.52 |
| Grok 4.5 | 5.00 | 1.67 | 3.33 |
| Qwen3.8 Max (0902) | 5.00 | 1.67 | 3.33 |
| Gemini 3.5 Flash | 6.67 | 1.11 | 2.96 |
| Mistral Medium 3.5 | 6.67 | 1.33 | 3.33 |
| Claude Sonnet 5 | 5.00 | 1.00 | 2.50 |
| Kimi K3 | 3.78 | 0.75 | 1.88 |
| Gemini 3.1 Pro Preview | 5.00 | 0.83 | 2.22 |
| Claude Sonnet 4.6 | 3.33 | 0.67 | 1.67 |
| Claude Sonnet 4.5 | 3.33 | 0.67 | 1.67 |
| Qwen3.7 Flash | 333 | 76.9 | 182 |
| Qwen3.7 Plus | 31.2 | 7.81 | 17.9 |

**<≈ Haiku 4.5**

| Модель | Чисто вход (M на $10) | Чисто выход (M на $10) | Смешанный (M на $10) |
|---|---|---|---|
| Xiaomi MiMo-V2.5 | 71.4 | 35.7 | 57.1 |
| GLM 5.3 Flash | 66.7 | 20.0 | 42.1 |
| DeepSeek V3.2 | 37.2 | 25.0 | 33.1 |
| MiniMax M2.5 | 37.0 | 9.26 | 21.2 |
| MiniMax M2 | 39.2 | 9.80 | 22.4 |
| Xiaomi MiMo-V2.5-Pro | 23.0 | 11.5 | 18.4 |
| Qwen3 Coder (480B) | 33.3 | 10.0 | 21.1 |
| GLM 4.7 | 25.0 | 5.71 | 13.6 |
| GPT-5 Mini | 40.0 | 5.00 | 14.5 |
| Kimi K2.5 | 22.2 | 4.44 | 11.1 |
| Nemotron 3 Ultra | 16.7 | 4.17 | 9.52 |
| Llama 4 Maverick | 50.0 | 14.4 | 30.9 |
| Llama 4 Scout | 100 | 33.3 | 66.7 |
| Kimi K2.7 Code | 14.1 | 2.86 | 7.10 |
| Mistral Large 3 | 20.0 | 6.67 | 13.3 |
| Claude Haiku 4.5 | 10.0 | 2.00 | 5.00 |
| Laguna XS 2.1 | 167 | 83.3 | 133 |
| Nemotron 3 Nano 30B A3B | 200 | 50.0 | 114 |
| DeepSeek V4 Flash | 113 | 56.4 | 90.3 |
| Laguna S 2.1 | 111 | 55.6 | 88.9 |
| Gemma 4 26B A4B | 111 | 33.3 | 70.2 |
| GLM 4.7 Flash | 165 | 25.0 | 68.8 |
| Gemma 4 31B | 111 | 29.4 | 65.6 |
| Qwen3 235B A22B Instruct 2507 | 114 | 28.6 | 65.3 |
| Nemotron 3 Super | 125 | 22.2 | 58.0 |
| Tencent Hy3 | 75.8 | 18.9 | 43.3 |
| Qwen3 Coder Next | 83.3 | 12.5 | 34.5 |
| Qwen3 Next 80B A3B Instruct | 111 | 9.09 | 29.2 |
| Mistral Codestral 2508 | 33.3 | 11.1 | 22.2 |
| Meituan LongCat 2.0 | 33.3 | 8.33 | 19.0 |
| KAT-Coder V2.5 Pro | 13.5 | 3.38 | 7.72 |

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
- Значительная часть оценок в таблицах — **вендорские** (помечено «только вендор»). Независимо подтверждены только те строки, число которых пришло с vals.ai или swebench.com (в model-map.tsv 34 записи vals= и 10 записей swebench=).
- SWE-bench Verified и подобные бенчмарки не идеально предсказывают реальное качество в конкретном воркфлоу — протестируйте 1-2 кандидата на своих задачах перед переключением.
- OpenRouter обычно даёт скидку на закешированный контекст (порядка 60-90% по большинству моделей), но точный процент варьируется по провайдеру.
- В таблицах всюду **типовая цена каталога** OpenRouter (`/api/v1/models`), а не самый дешёвый маршрут конкретного провайдера; актуальный список промо-цен — https://openrouter.ai/collections/discounted-models.

## Бесплатные модели (рейтинг по качеству)

Модели с ценой $0/$0 (не путать с дешёвыми платными) — из каталога OpenRouter (`pricing.prompt == "0"` и `pricing.completion == "0"`). Лучшая по качеству вынесена в «Фавориты по категориям» выше. Две оговорки: **все оценки здесь вендорские** — ни одна бесплатная модель в подборке не имеет независимого прогона SWE-bench Verified; и **каталог бесплатных моделей волатилен** — проверяйте наличие slug'а перед тем, как на него закладываться.

| Модель | Slug на OpenRouter | Контекст | Benchmark score | Capability estimate | Владелец | Открытые веса | Copyright |
|---|---|---|---|---|---|---|---|
| NVIDIA Nemotron 3 Ultra | `nvidia/nemotron-3-ultra-550b-a55b:free` | 1M | 69.0% · [vals.ai](https://www.vals.ai/benchmarks/swebench), 2026-09-01 | <≈ Haiku 4.5 (середина диапазона) | NVIDIA | да, OpenMDW-1.1 | unknown |
| Cohere North Mini Code | `cohere/north-mini-code:free` | 256K | 67.6% [observation_only] · [swebench.com](https://cohere.com/), n/a | <≈ Haiku 4.5 (середина диапазона) | Cohere (Cohere Labs) | да, Apache 2.0 | unknown |
| Dots Studio: Dots3-Note Preview (free) | `dots-studio/dots-3-note-preview:free` | 512K | n/a | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ | unknown |
| Google Gemma 4 26B A4B | `google/gemma-4-26b-a4b-it:free` | 262K | n/a | <≈ Haiku 4.5 (меньше активных параметров, чем у 31B-версии) — не подтверждено | Google DeepMind | да, Apache 2.0 | unknown |
| Google Gemma 4 31B | `google/gemma-4-31b-it:free` | 262K | n/a | не определить по этой метрике | Google DeepMind | да, Apache 2.0 | unknown |
| LiquidAI: LFM2.5-2.6B (free) | `liquid/lfm-2.5-2.6b:free` | 66K | n/a | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ | unknown |
| NVIDIA Nemotron 3 Nano Omni 30B-A3B (reasoning) | `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | 256K | n/a | не применимо — не кодинг-модель | NVIDIA | да | unknown |
| NVIDIA Nemotron 3 Super | `nvidia/nemotron-3-super-120b-a12b:free` | 262K | 60.5% [observation_only] · [swebench.com](https://developer.nvidia.com/), n/a | <≈ Haiku 4.5 (нижняя граница диапазона) | NVIDIA | да, OpenMDW-1.1 | unknown |
| Nemotron 3.5 Content Safety (free) | `nvidia/nemotron-3.5-content-safety:free` | 128K | n/a | _нужен обзор_ | _нужен обзор_ | _нужен обзор_ | unknown |
| NVIDIA Nemotron 3.5 Lightning | `nvidia/nemotron-3.5-lightning:free` | 1M | n/a | <≈ Haiku 4.5 — небольшая agentic-MoE (3B активных из 30B); по SWE-bench Verified не измерена | NVIDIA | да, OpenMDW-1.1 | unknown |
| Poolside Laguna S 2.1 | `poolside/laguna-s-2.1:free` | 262K | n/a | вероятно <≈ Haiku 4.5 — но по не-Verified метрикам, не сравнивайте напрямую | Poolside | да, OpenMDW-1.1 | unknown |
| Poolside Laguna XS 2.1 | `poolside/laguna-xs-2.1:free` | 262K | 70.9% [observation_only] · [swebench.com](https://poolside.ai/), n/a | <≈ Haiku 4.5 (верх диапазона) | Poolside | да, OpenMDW-1.1 | unknown |

**Заметки**

- **NVIDIA Nemotron 3 Ultra** (`nvidia/nemotron-3-ultra-550b-a55b:free`) — 550B/55B-active MoE. Прогон на 5 разных агентных скаффолдах делала сама NVIDIA — это демонстрация устойчивости к харнессу, а не независимое подтверждение. Независимая оценка на vals.ai теперь есть и используется в таблице (69.0%) вместо прежнего вендорского диапазона 65–70.4% (тот же 5-скаффолдный прогон NVIDIA), который снят.
- **Cohere North Mini Code** (`cohere/north-mini-code:free`) — 30B/3B-active MoE; SWE-bench Pro 40.2% (другая метрика). Оценки вендорские, независимого подтверждения не найдено. Единственная компания из этой таблицы, попавшая в трекер SaferAI — 8% (последнее место среди 12).
- **Google Gemma 4 26B A4B** (`google/gemma-4-26b-a4b-it:free`) — 26B/4B-active MoE. Утверждение прошлой версии про «реальный лимит эндпоинта 131K» **ослаблено**: каталог OpenRouter сейчас показывает 262144, а известный кейс с обрезкой до 131072 документирован для конкретного бэкенда (Cloudflare Workers AI) — маршрутизирует ли OpenRouter этот `:free`-slug через него, выяснить не удалось. ELO 1441 в этой проверке независимо не переподтверждён, оставлен с прошлой версии.
- **Google Gemma 4 31B** (`google/gemma-4-31b-it:free`) — Бенчмарка SWE-bench Verified не нашли. Есть LMArena ELO ~1452 (#3 среди открытых) и Codeforces ELO 2150 — другие метрики, напрямую несопоставимы.
- **NVIDIA Nemotron 3 Nano Omni 30B-A3B (reasoning)** (`nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free`) — Не для сравнения по кодингу — иная специализация (мультимодальность подтверждена в этой проверке).
- **NVIDIA Nemotron 3 Super** (`nvidia/nemotron-3-super-120b-a12b:free`) — 120B/12B-active MoE.
- **NVIDIA Nemotron 3.5 Lightning** (`nvidia/nemotron-3.5-lightning:free`) — Независимой оценки SWE-bench Verified нет ни на vals.ai, ни на swebench.com (проверено 2026-08-17 — ни один из двух источников модель не упоминает). Оценка LMArena (отдельная метрика, Elo) нашлась под анонимным кодовым именем august26-chatbot1-fmme: modelDisplayName на странице арены ("nvidia-nemotron-3.5-lightning-30b-a3b-nvfp4") и modelUrl (официальный блог NVIDIA именно про эту модель, releaseType "co_release") независимо раскрывают личность модели — это не догадка по значению Elo, а прямое указание самого источника.
- **Poolside Laguna S 2.1** (`poolside/laguna-s-2.1:free`) — 118B/8B-active. SWE-bench Verified нет; есть SWE-bench Multilingual 78.5%, Pro 59.4%, Terminal-Bench 2.1 70.2%, DeepSWE v1.1 40.4% — метрики не Verified, напрямую с остальными строками несопоставимы.
- **Poolside Laguna XS 2.1** (`poolside/laguna-xs-2.1:free`) — 33B/3B-active, узкая специализация — агентный кодинг; независимо не подтверждено (Poolside прямо пишет в карточке, что публикует «official scores» из своих релизных постов). Фактически вровень с Nemotron 3 Ultra, уступает по контексту. Курьёз: у той же модели есть отдельный **платный** slug без `:free` — $0.06/$0.12 (скидка 40%), там лимитов бесплатного тира нет.

Практические оговорки для всех `:free`-моделей: rate-limit 20 запросов/мин; 50 запросов/день на аккаунте с пополнениями меньше $10 за всё время, 1000/день после разового пополнения от $10 (openrouter.ai/docs/api-reference/limits). Часть провайдеров бесплатных эндпоинтов может использовать запросы для обучения; управляется это настройками приватности аккаунта — openrouter.ai/settings/privacy.
## Актуальные модели без ручного сопоставления

Актуальные строки каталога без ручной benchmark identity.

Эти строки пришли из актуального каталога OpenRouter. Отсутствие ручной identity-записи не даёт benchmark identity, tier или ranking.

| Модель | Slug на OpenRouter | Вход $/M | Выход $/M | Контекст | Benchmark | Ranking |
|---|---|---:|---:|---:|---|---|
| AionLabs: Aion-2.0 | `aion-labs/aion-2.0` | $0.80 | $1.60 | 131K | n/a (missing identity) | n/a |
| AionLabs: Aion-3.0 | `aion-labs/aion-3.0` | $3.00 | $6.00 | 131K | n/a (missing identity) | n/a |
| AionLabs: Aion-3.0-Mini | `aion-labs/aion-3.0-mini` | $0.70 | $1.40 | 131K | n/a (missing identity) | n/a |
| AionLabs: Aion-RP 1.0 (8B) | `aion-labs/aion-rp-llama-3.1-8b` | $0.80 | $1.60 | 33K | n/a (missing identity) | n/a |
| Nova 2 Lite | `amazon/nova-2-lite-v1` | $0.30 | $2.50 | 1M | n/a (missing identity) | n/a |
| Nova Lite 1.0 | `amazon/nova-lite-v1` | $0.06 | $0.24 | 300K | n/a (missing identity) | n/a |
| Nova Micro 1.0 | `amazon/nova-micro-v1` | $0.04 | $0.14 | 128K | n/a (missing identity) | n/a |
| Nova Premier 1.0 | `amazon/nova-premier-v1` | $2.50 | $12.50 | 1M | n/a (missing identity) | n/a |
| Nova Pro 1.0 | `amazon/nova-pro-v1` | $0.80 | $3.20 | 300K | n/a (missing identity) | n/a |
| Magnum v4 72B | `anthracite-org/magnum-v4-72b` | $2.50 | $5.00 | 33K | n/a (missing identity) | n/a |
| Claude 3 Haiku | `anthropic/claude-3-haiku` | $0.25 | $1.25 | 200K | n/a (missing identity) | n/a |
| Claude Fable 5.1 (batch) | `anthropic/claude-fable-5.1:batch` | $5.00 | $25.00 | 1M | n/a (missing identity) | n/a |
| Claude Fable 5 (batch) | `anthropic/claude-fable-5:batch` | $5.00 | $25.00 | 1M | n/a (missing identity) | n/a |
| Claude Haiku 4.5 (batch) | `anthropic/claude-haiku-4.5:batch` | $0.50 | $2.50 | 200K | n/a (missing identity) | n/a |
| Claude Opus 4 | `anthropic/claude-opus-4` | $15.00 | $75.00 | 200K | n/a (missing identity) | n/a |
| Claude Opus 4.1 | `anthropic/claude-opus-4.1` | $15.00 | $75.00 | 200K | n/a (missing identity) | n/a |
| Claude Opus 4.1 (batch) | `anthropic/claude-opus-4.1:batch` | $7.50 | $37.50 | 200K | n/a (missing identity) | n/a |
| Claude Opus 4.5 | `anthropic/claude-opus-4.5` | $5.00 | $25.00 | 200K | n/a (missing identity) | n/a |
| Claude Opus 4.5 (batch) | `anthropic/claude-opus-4.5:batch` | $2.50 | $12.50 | 200K | n/a (missing identity) | n/a |
| Claude Opus 4.6 (batch) | `anthropic/claude-opus-4.6:batch` | $2.50 | $12.50 | 1M | n/a (missing identity) | n/a |
| Claude Opus 4.7 (batch) | `anthropic/claude-opus-4.7:batch` | $2.50 | $12.50 | 1M | n/a (missing identity) | n/a |
| Claude Opus 4.8 (batch) | `anthropic/claude-opus-4.8:batch` | $2.50 | $12.50 | 1M | n/a (missing identity) | n/a |
| Claude Opus 5 (batch) | `anthropic/claude-opus-5:batch` | $2.50 | $12.50 | 1M | n/a (missing identity) | n/a |
| Claude Sonnet 4 | `anthropic/claude-sonnet-4` | $3.00 | $15.00 | 1M | n/a (missing identity) | n/a |
| Claude Sonnet 4.5 (batch) | `anthropic/claude-sonnet-4.5:batch` | $1.50 | $7.50 | 1M | n/a (missing identity) | n/a |
| Claude Sonnet 4.6 (batch) | `anthropic/claude-sonnet-4.6:batch` | $1.50 | $7.50 | 1M | n/a (missing identity) | n/a |
| Claude Sonnet 5 (batch) | `anthropic/claude-sonnet-5:batch` | $1.00 | $5.00 | 1M | n/a (missing identity) | n/a |
| Arcee AI: Trinity Large Thinking | `arcee-ai/trinity-large-thinking` | $0.25 | $0.80 | 262K | n/a (missing identity) | n/a |
| ERNIE 4.5 VL 424B A47B | `baidu/ernie-4.5-vl-424b-a47b` | $0.42 | $1.25 | 123K | n/a (missing identity) | n/a |
| ByteDance Seed: Seed 1.6 | `bytedance-seed/seed-1.6` | $0.25 | $2.00 | 262K | n/a (missing identity) | n/a |
| ByteDance Seed: Seed 1.6 Flash | `bytedance-seed/seed-1.6-flash` | $0.08 | $0.30 | 262K | n/a (missing identity) | n/a |
| ByteDance Seed: Seed 2.1 Turbo | `bytedance-seed/seed-2-1-turbo` | $0.50 | $2.50 | 262K | n/a (missing identity) | n/a |
| ByteDance Seed: Seed-2.0-Code | `bytedance-seed/seed-2.0-code` | $0.50 | $3.00 | 262K | n/a (missing identity) | n/a |
| ByteDance Seed: Seed-2.0-Lite | `bytedance-seed/seed-2.0-lite` | $0.25 | $2.00 | 262K | n/a (missing identity) | n/a |
| ByteDance Seed: Seed-2.0-Mini | `bytedance-seed/seed-2.0-mini` | $0.10 | $0.40 | 262K | n/a (missing identity) | n/a |
| UI-TARS 7B | `bytedance/ui-tars-1.5-7b` | $0.10 | $0.20 | 128K | n/a (missing identity) | n/a |
| Venice: Uncensored | `cognitivecomputations/dolphin-mistral-24b-venice-edition` | $0.20 | $0.90 | 128K | n/a (missing identity) | n/a |
| Command A | `cohere/command-a` | $2.50 | $10.00 | 256K | n/a (missing identity) | n/a |
| Command R (08-2024) | `cohere/command-r-08-2024` | $0.15 | $0.60 | 128K | n/a (missing identity) | n/a |
| Command R+ (08-2024) | `cohere/command-r-plus-08-2024` | $2.50 | $10.00 | 128K | n/a (missing identity) | n/a |
| Command R7B (12-2024) | `cohere/command-r7b-12-2024` | $0.04 | $0.15 | 128K | n/a (missing identity) | n/a |
| DeepSeek V3 | `deepseek/deepseek-chat` | $0.26 | $1.03 | 164K | n/a (missing identity) | n/a |
| DeepSeek V3 0324 | `deepseek/deepseek-chat-v3-0324` | $0.25 | $1.00 | 164K | n/a (missing identity) | n/a |
| DeepSeek V3.1 | `deepseek/deepseek-chat-v3.1` | $0.25 | $0.95 | 164K | n/a (missing identity) | n/a |
| R1 | `deepseek/deepseek-r1` | $0.70 | $2.50 | 64K | n/a (missing identity) | n/a |
| R1 0528 | `deepseek/deepseek-r1-0528` | $0.50 | $2.15 | 164K | n/a (missing identity) | n/a |
| R1 Distill Llama 70B | `deepseek/deepseek-r1-distill-llama-70b` | $0.80 | $0.80 | 8K | n/a (missing identity) | n/a |
| DeepSeek V3.1 Terminus | `deepseek/deepseek-v3.1-terminus` | $0.27 | $1.00 | 164K | n/a (missing identity) | n/a |
| DeepSeek V3.2 Exp | `deepseek/deepseek-v3.2-exp` | $0.27 | $0.41 | 164K | n/a (missing identity) | n/a |
| DeepSeek V4 Flash 0731 | `deepseek/deepseek-v4-flash-0731` | $0.06 | $0.12 | 1.3M | n/a (missing identity) | n/a |
| DeepSeek V4 Flash 0731 (batch) | `deepseek/deepseek-v4-flash-0731:batch` | $0.11 | $0.33 | 1M | n/a (missing identity) | n/a |
| DeepSeek V4 Flash Vision Exp | `deepseek/deepseek-v4-flash-vision-exp` | $0.22 | $0.66 | 1M | n/a (missing identity) | n/a |
| DeepSeek V4 Flash Vision Exp (batch) | `deepseek/deepseek-v4-flash-vision-exp:batch` | $0.11 | $0.33 | 1M | n/a (missing identity) | n/a |
| DeepSeek V4 Pro 0813 | `deepseek/deepseek-v4-pro-0813` | $0.98 | $2.95 | 1M | n/a (missing identity) | n/a |
| DeepSeek V4 Pro 0813 (batch) | `deepseek/deepseek-v4-pro-0813:batch` | $0.66 | $1.98 | 1M | n/a (missing identity) | n/a |
| DeepSeek V4.1 Flash | `deepseek/deepseek-v4.1-flash` | $0.15 | $0.60 | 1M | n/a (missing identity) | n/a |
| Gemini 2.5 Flash | `google/gemini-2.5-flash` | $0.30 | $2.50 | 1M | n/a (missing identity) | n/a |
| Nano Banana (Gemini 2.5 Flash Image) | `google/gemini-2.5-flash-image` | $0.30 | $2.50 | 33K | n/a (missing identity) | n/a |
| Gemini 2.5 Flash Lite | `google/gemini-2.5-flash-lite` | $0.10 | $0.40 | 1M | n/a (missing identity) | n/a |
| Gemini 2.5 Flash Lite (batch) | `google/gemini-2.5-flash-lite:batch` | $0.05 | $0.20 | 1M | n/a (missing identity) | n/a |
| Gemini 2.5 Flash (batch) | `google/gemini-2.5-flash:batch` | $0.15 | $1.25 | 1M | n/a (missing identity) | n/a |
| Gemini 2.5 Pro | `google/gemini-2.5-pro` | $1.25 | $10.00 | 1M | n/a (missing identity) | n/a |
| Gemini 2.5 Pro Preview 06-05 | `google/gemini-2.5-pro-preview` | $1.25 | $10.00 | 1M | n/a (missing identity) | n/a |
| Gemini 2.5 Pro Preview 05-06 | `google/gemini-2.5-pro-preview-05-06` | $1.25 | $10.00 | 1M | n/a (missing identity) | n/a |
| Gemini 2.5 Pro (batch) | `google/gemini-2.5-pro:batch` | $0.63 | $5.00 | 1M | n/a (missing identity) | n/a |
| Gemini 3 Flash Preview | `google/gemini-3-flash-preview` | $0.50 | $3.00 | 1M | n/a (missing identity) | n/a |
| Gemini 3 Flash Preview (batch) | `google/gemini-3-flash-preview:batch` | $0.25 | $1.50 | 1M | n/a (missing identity) | n/a |
| Nano Banana Pro (Gemini 3 Pro Image) | `google/gemini-3-pro-image` | $2.00 | $12.00 | 131K | n/a (missing identity) | n/a |
| Nano Banana Pro (Gemini 3 Pro Image Preview) | `google/gemini-3-pro-image-preview` | $2.00 | $12.00 | 66K | n/a (missing identity) | n/a |
| Nano Banana 2 (Gemini 3.1 Flash Image) | `google/gemini-3.1-flash-image` | $0.50 | $3.00 | 131K | n/a (missing identity) | n/a |
| Nano Banana 2 (Gemini 3.1 Flash Image Preview) | `google/gemini-3.1-flash-image-preview` | $0.50 | $3.00 | 66K | n/a (missing identity) | n/a |
| Gemini 3.1 Flash Lite | `google/gemini-3.1-flash-lite` | $0.25 | $1.50 | 1M | n/a (missing identity) | n/a |
| Nano Banana 2 Lite (Gemini 3.1 Flash Lite Image) | `google/gemini-3.1-flash-lite-image` | $0.25 | $1.50 | 66K | n/a (missing identity) | n/a |
| Gemini 3.1 Flash Lite Preview | `google/gemini-3.1-flash-lite-preview` | $0.25 | $1.50 | 1M | n/a (missing identity) | n/a |
| Gemini 3.1 Flash Lite (batch) | `google/gemini-3.1-flash-lite:batch` | $0.13 | $0.75 | 1M | n/a (missing identity) | n/a |
| Gemini 3.1 Pro Preview Custom Tools | `google/gemini-3.1-pro-preview-customtools` | $2.00 | $12.00 | 1M | n/a (missing identity) | n/a |
| Gemini 3.1 Pro Preview (batch) | `google/gemini-3.1-pro-preview:batch` | $1.00 | $6.00 | 1M | n/a (missing identity) | n/a |
| Gemini 3.5 Flash Lite | `google/gemini-3.5-flash-lite` | $0.30 | $2.50 | 1M | n/a (missing identity) | n/a |
| Gemini 3.5 Flash Lite (batch) | `google/gemini-3.5-flash-lite:batch` | $0.15 | $1.25 | 1M | n/a (missing identity) | n/a |
| Gemini 3.5 Flash (batch) | `google/gemini-3.5-flash:batch` | $0.75 | $4.50 | 1M | n/a (missing identity) | n/a |
| Gemini 3.6 Flash (batch) | `google/gemini-3.6-flash:batch` | $0.38 | $1.88 | 1M | n/a (missing identity) | n/a |
| Gemini 3.7 Flash (batch) | `google/gemini-3.7-flash:batch` | $0.38 | $1.88 | 1M | n/a (missing identity) | n/a |
| Gemini 3.8 Flash (batch) | `google/gemini-3.8-flash:batch` | $0.38 | $1.88 | 1M | n/a (missing identity) | n/a |
| Gemma 2 27B | `google/gemma-2-27b-it` | $0.65 | $0.65 | 8K | n/a (missing identity) | n/a |
| Gemma 3 12B | `google/gemma-3-12b-it` | $0.05 | $0.15 | 131K | n/a (missing identity) | n/a |
| Gemma 3 27B | `google/gemma-3-27b-it` | $0.08 | $0.45 | 131K | n/a (missing identity) | n/a |
| Gemma 3 4B | `google/gemma-3-4b-it` | $0.05 | $0.10 | 131K | n/a (missing identity) | n/a |
| Gemma 4 31B (batch) | `google/gemma-4-31b-it:batch` | $0.39 | $0.97 | 262K | n/a (missing identity) | n/a |
| Lyria 3 Clip Preview | `google/lyria-3-clip-preview` | $0.00 | $0.00 | 1M | n/a (missing identity) | n/a |
| Lyria 3 Pro Preview | `google/lyria-3-pro-preview` | $0.00 | $0.00 | 1M | n/a (missing identity) | n/a |
| MythoMax 13B | `gryphe/mythomax-l2-13b` | $0.06 | $0.06 | 8K | n/a (missing identity) | n/a |
| IBM: Granite 4.0 Micro | `ibm-granite/granite-4.0-h-micro` | $0.02 | $0.11 | 131K | n/a (missing identity) | n/a |
| IBM: Granite 4.2 8B | `ibm-granite/granite-4.2-8b` | $0.06 | $0.25 | 131K | n/a (missing identity) | n/a |
| Mercury 2 | `inception/mercury-2` | $0.25 | $0.75 | 128K | n/a (missing identity) | n/a |
| Mercury 2.5 | `inception/mercury-2.5` | $0.04 | $0.15 | 260K | n/a (missing identity) | n/a |
| Ling 3.0 Flash | `inclusionai/ling-3.0-flash` | $0.02 | $0.06 | 262K | n/a (missing identity) | n/a |
| Ling 3.0 Flash Fin | `inclusionai/ling-3.0-flash-fin` | $0.06 | $0.18 | 262K | n/a (missing identity) | n/a |
| Ling 3.0 Flash Fin (free) | `inclusionai/ling-3.0-flash-fin:free` | $0.00 | $0.00 | 262K | n/a (missing identity) | n/a |
| Ling 3.0 Flash Sante (free) | `inclusionai/ling-3.0-flash-sante:free` | $0.00 | $0.00 | 262K | n/a (missing identity) | n/a |
| Ling 3.0 Flash VL | `inclusionai/ling-3.0-flash-vl` | $0.06 | $0.18 | 131K | n/a (missing identity) | n/a |
| Ling 3.0 Flash VL (free) | `inclusionai/ling-3.0-flash-vl:free` | $0.00 | $0.00 | 262K | n/a (missing identity) | n/a |
| Inference.net: Schematron V2 Small | `inference-net/schematron-v2-small` | $0.05 | $0.23 | 128K | n/a (missing identity) | n/a |
| Inference.net: Schematron V2 Turbo | `inference-net/schematron-v2-turbo` | $0.03 | $0.15 | 128K | n/a (missing identity) | n/a |
| KAT-Coder-Pro V2 | `kwaipilot/kat-coder-pro-v2` | $0.30 | $1.20 | 262K | n/a (missing identity) | n/a |
| Weaver (alpha) | `mancer/weaver` | $0.40 | $0.75 | 8K | n/a (missing identity) | n/a |
| Llama 3.1 70B Instruct | `meta-llama/llama-3.1-70b-instruct` | $0.40 | $0.40 | 131K | n/a (missing identity) | n/a |
| Llama 3.1 8B Instruct | `meta-llama/llama-3.1-8b-instruct` | $0.05 | $0.08 | 131K | n/a (missing identity) | n/a |
| Llama 3.2 1B Instruct | `meta-llama/llama-3.2-1b-instruct` | $0.03 | $0.20 | 60K | n/a (missing identity) | n/a |
| Llama 3.2 3B Instruct | `meta-llama/llama-3.2-3b-instruct` | $0.05 | $0.33 | 131K | n/a (missing identity) | n/a |
| Llama 3.3 70B Instruct | `meta-llama/llama-3.3-70b-instruct` | $0.10 | $0.32 | 131K | n/a (missing identity) | n/a |
| Llama Guard 4 12B | `meta-llama/llama-guard-4-12b` | $0.18 | $0.18 | 164K | n/a (missing identity) | n/a |
| Muse Glimmer 30B | `meta/muse-glimmer-30b` | $0.35 | $1.50 | 131K | n/a (missing identity) | n/a |
| Muse Glimmer 30B (batch) | `meta/muse-glimmer-30b:batch` | $0.18 | $0.75 | 131K | n/a (missing identity) | n/a |
| Muse Spark 1.2 | `meta/muse-spark-1.2` | $1.25 | $4.25 | 1M | n/a (missing identity) | n/a |
| Muse Spark 1.2 Contributor | `meta/muse-spark-1.2-contributor` | $0.10 | $0.20 | 1M | n/a (missing identity) | n/a |
| Muse Spark 1.3 | `meta/muse-spark-1.3` | $1.25 | $4.25 | 1M | n/a (missing identity) | n/a |
| Muse Spark 1.3 Contributor | `meta/muse-spark-1.3-contributor` | $0.10 | $0.20 | 1M | n/a (missing identity) | n/a |
| Phi 4 | `microsoft/phi-4` | $0.07 | $0.14 | 16K | n/a (missing identity) | n/a |
| WizardLM-2 8x22B | `microsoft/wizardlm-2-8x22b` | $0.62 | $0.62 | 66K | n/a (missing identity) | n/a |
| MiniMax-01 | `minimax/minimax-01` | $0.20 | $1.10 | 1M | n/a (missing identity) | n/a |
| MiniMax M1 | `minimax/minimax-m1` | $0.55 | $2.20 | 1M | n/a (missing identity) | n/a |
| MiniMax M2-her | `minimax/minimax-m2-her` | $0.30 | $1.20 | 66K | n/a (missing identity) | n/a |
| MiniMax M2.1 | `minimax/minimax-m2.1` | $0.30 | $1.20 | 205K | n/a (missing identity) | n/a |
| MiniMax M2.7 | `minimax/minimax-m2.7` | $0.30 | $1.20 | 205K | n/a (missing identity) | n/a |
| MiniMax M3 (batch) | `minimax/minimax-m3:batch` | $0.30 | $1.20 | 524K | n/a (missing identity) | n/a |
| Mistral: Codestral 2508 (batch) | `mistralai/codestral-2508:batch` | $0.15 | $0.45 | 256K | n/a (missing identity) | n/a |
| Mistral Devstral 2 | `mistralai/devstral-2512` | $0.40 | $2.00 | 262K | n/a (missing identity) | n/a |
| Mistral: Ministral 3 14B 2512 | `mistralai/ministral-14b-2512` | $0.20 | $0.20 | 262K | n/a (missing identity) | n/a |
| Mistral: Ministral 3 3B 2512 | `mistralai/ministral-3b-2512` | $0.10 | $0.10 | 131K | n/a (missing identity) | n/a |
| Mistral: Ministral 3 8B 2512 | `mistralai/ministral-8b-2512` | $0.15 | $0.15 | 262K | n/a (missing identity) | n/a |
| Mistral: Ministral 3 8B 2512 (batch) | `mistralai/ministral-8b-2512:batch` | $0.08 | $0.08 | 262K | n/a (missing identity) | n/a |
| Mistral Large | `mistralai/mistral-large` | $2.00 | $6.00 | 128K | n/a (missing identity) | n/a |
| Mistral Large 2407 | `mistralai/mistral-large-2407` | $2.00 | $6.00 | 131K | n/a (missing identity) | n/a |
| Mistral: Mistral Large 3 2512 (batch) | `mistralai/mistral-large-2512:batch` | $0.25 | $0.75 | 262K | n/a (missing identity) | n/a |
| Mistral: Mistral Medium 3 | `mistralai/mistral-medium-3` | $0.40 | $2.00 | 131K | n/a (missing identity) | n/a |
| Mistral: Mistral Medium 3.5 (batch) | `mistralai/mistral-medium-3-5:batch` | $0.75 | $3.75 | 262K | n/a (missing identity) | n/a |
| Mistral: Mistral Medium 3.1 | `mistralai/mistral-medium-3.1` | $0.40 | $2.00 | 131K | n/a (missing identity) | n/a |
| Mistral: Mistral Medium 3.1 (batch) | `mistralai/mistral-medium-3.1:batch` | $0.20 | $1.00 | 131K | n/a (missing identity) | n/a |
| Mistral: Mistral Nemo | `mistralai/mistral-nemo` | $0.02 | $0.03 | 131K | n/a (missing identity) | n/a |
| Mistral: Saba | `mistralai/mistral-saba` | $0.20 | $0.60 | 33K | n/a (missing identity) | n/a |
| Mistral: Mistral Small 3 | `mistralai/mistral-small-24b-instruct-2501` | $0.05 | $0.08 | 33K | n/a (missing identity) | n/a |
| Mistral: Mistral Small 4 | `mistralai/mistral-small-2603` | $0.15 | $0.60 | 262K | n/a (missing identity) | n/a |
| Mistral: Mistral Small 4 (batch) | `mistralai/mistral-small-2603:batch` | $0.08 | $0.30 | 262K | n/a (missing identity) | n/a |
| Mistral: Mistral Small 3.1 24B | `mistralai/mistral-small-3.1-24b-instruct` | $0.35 | $0.56 | 128K | n/a (missing identity) | n/a |
| Mistral: Mistral Small 3.2 24B | `mistralai/mistral-small-3.2-24b-instruct` | $0.08 | $0.20 | 256K | n/a (missing identity) | n/a |
| Mistral: Mixtral 8x22B Instruct | `mistralai/mixtral-8x22b-instruct` | $2.00 | $6.00 | 66K | n/a (missing identity) | n/a |
| Mistral: Voxtral Small 24B 2507 | `mistralai/voxtral-small-24b-2507` | $0.10 | $0.30 | 33K | n/a (missing identity) | n/a |
| MoonshotAI: Kimi K2 0711 | `moonshotai/kimi-k2` | $0.57 | $2.30 | 131K | n/a (missing identity) | n/a |
| MoonshotAI: Kimi K2 0905 | `moonshotai/kimi-k2-0905` | $0.60 | $2.50 | 262K | n/a (missing identity) | n/a |
| MoonshotAI: Kimi K2 Thinking | `moonshotai/kimi-k2-thinking` | $0.60 | $2.50 | 262K | n/a (missing identity) | n/a |
| MoonshotAI: Kimi K2.6 | `moonshotai/kimi-k2.6` | $0.95 | $4.00 | 262K | n/a (missing identity) | n/a |
| MoonshotAI: Kimi K3 (batch) | `moonshotai/kimi-k3:batch` | $3.00 | $15.00 | 1M | n/a (missing identity) | n/a |
| Morph V3 Fast | `morph/morph-v3-fast` | $0.80 | $1.20 | 82K | n/a (missing identity) | n/a |
| Morph V3 Large | `morph/morph-v3-large` | $0.90 | $1.90 | 262K | n/a (missing identity) | n/a |
| Nex AGI: Nex-N2.5-Mini (free) | `nex-agi/nex-n2.5-mini:free` | $0.00 | $0.00 | 262K | n/a (missing identity) | n/a |
| Nex AGI: Nex-N2.5-Pro (free) | `nex-agi/nex-n2.5-pro:free` | $0.00 | $0.00 | 262K | n/a (missing identity) | n/a |
| Nous: Hermes 3 405B Instruct | `nousresearch/hermes-3-llama-3.1-405b` | $1.00 | $1.00 | 131K | n/a (missing identity) | n/a |
| Nous: Hermes 3 70B Instruct | `nousresearch/hermes-3-llama-3.1-70b` | $0.70 | $0.70 | 131K | n/a (missing identity) | n/a |
| Nous: Hermes 4 405B | `nousresearch/hermes-4-405b` | $1.00 | $3.00 | 131K | n/a (missing identity) | n/a |
| Nemotron 3.5 Content Safety | `nvidia/nemotron-3.5-content-safety` | $0.20 | $0.20 | 131K | n/a (missing identity) | n/a |
| Nemotron 3.5 Lightning | `nvidia/nemotron-3.5-lightning` | $0.08 | $0.20 | 262K | n/a (missing identity) | n/a |
| GPT-3.5 Turbo | `openai/gpt-3.5-turbo` | $0.50 | $1.50 | 16K | n/a (missing identity) | n/a |
| GPT-3.5 Turbo (older v0613) | `openai/gpt-3.5-turbo-0613` | $1.00 | $2.00 | 4K | n/a (missing identity) | n/a |
| GPT-3.5 Turbo 16k | `openai/gpt-3.5-turbo-16k` | $3.00 | $4.00 | 16K | n/a (missing identity) | n/a |
| GPT-3.5 Turbo Instruct | `openai/gpt-3.5-turbo-instruct` | $1.50 | $2.00 | 4K | n/a (missing identity) | n/a |
| GPT-3.5 Turbo (batch) | `openai/gpt-3.5-turbo:batch` | $0.25 | $0.75 | 16K | n/a (missing identity) | n/a |
| GPT-4 | `openai/gpt-4` | $30.00 | $60.00 | 8K | n/a (missing identity) | n/a |
| GPT-4 Turbo | `openai/gpt-4-turbo` | $10.00 | $30.00 | 128K | n/a (missing identity) | n/a |
| GPT-4 Turbo Preview | `openai/gpt-4-turbo-preview` | $10.00 | $30.00 | 128K | n/a (missing identity) | n/a |
| GPT-4 Turbo (batch) | `openai/gpt-4-turbo:batch` | $5.00 | $15.00 | 128K | n/a (missing identity) | n/a |
| GPT-4.1 | `openai/gpt-4.1` | $2.00 | $8.00 | 1M | n/a (missing identity) | n/a |
| GPT-4.1 Mini | `openai/gpt-4.1-mini` | $0.40 | $1.60 | 1M | n/a (missing identity) | n/a |
| GPT-4.1 Mini (batch) | `openai/gpt-4.1-mini:batch` | $0.20 | $0.80 | 1M | n/a (missing identity) | n/a |
| GPT-4.1 Nano | `openai/gpt-4.1-nano` | $0.10 | $0.40 | 1M | n/a (missing identity) | n/a |
| GPT-4.1 Nano (batch) | `openai/gpt-4.1-nano:batch` | $0.05 | $0.20 | 1M | n/a (missing identity) | n/a |
| GPT-4.1 (batch) | `openai/gpt-4.1:batch` | $1.00 | $4.00 | 1M | n/a (missing identity) | n/a |
| GPT-4o | `openai/gpt-4o` | $2.50 | $10.00 | 128K | n/a (missing identity) | n/a |
| GPT-4o (2024-05-13) | `openai/gpt-4o-2024-05-13` | $5.00 | $15.00 | 128K | n/a (missing identity) | n/a |
| GPT-4o (2024-08-06) | `openai/gpt-4o-2024-08-06` | $2.50 | $10.00 | 128K | n/a (missing identity) | n/a |
| GPT-4o (2024-11-20) | `openai/gpt-4o-2024-11-20` | $2.50 | $10.00 | 128K | n/a (missing identity) | n/a |
| GPT-4o-mini | `openai/gpt-4o-mini` | $0.15 | $0.60 | 128K | n/a (missing identity) | n/a |
| GPT-4o-mini (2024-07-18) | `openai/gpt-4o-mini-2024-07-18` | $0.15 | $0.60 | 128K | n/a (missing identity) | n/a |
| GPT-4o-mini (batch) | `openai/gpt-4o-mini:batch` | $0.08 | $0.30 | 128K | n/a (missing identity) | n/a |
| GPT-4o (batch) | `openai/gpt-4o:batch` | $1.25 | $5.00 | 128K | n/a (missing identity) | n/a |
| GPT-5 | `openai/gpt-5` | $1.25 | $10.00 | 400K | n/a (missing identity) | n/a |
| GPT-5 Image | `openai/gpt-5-image` | $10.00 | $10.00 | 400K | n/a (missing identity) | n/a |
| GPT-5 Image Mini | `openai/gpt-5-image-mini` | $2.50 | $2.00 | 400K | n/a (missing identity) | n/a |
| GPT-5 Mini (batch) | `openai/gpt-5-mini:batch` | $0.13 | $1.00 | 400K | n/a (missing identity) | n/a |
| GPT-5 Nano | `openai/gpt-5-nano` | $0.05 | $0.40 | 400K | n/a (missing identity) | n/a |
| GPT-5 Nano (batch) | `openai/gpt-5-nano:batch` | $0.03 | $0.20 | 400K | n/a (missing identity) | n/a |
| GPT-5 Pro | `openai/gpt-5-pro` | $15.00 | $120.00 | 400K | n/a (missing identity) | n/a |
| GPT-5 Pro (batch) | `openai/gpt-5-pro:batch` | $7.50 | $60.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.1 | `openai/gpt-5.1` | $1.25 | $10.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.1-Codex | `openai/gpt-5.1-codex` | $1.25 | $10.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.1-Codex-Max | `openai/gpt-5.1-codex-max` | $1.25 | $10.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.1-Codex-Mini | `openai/gpt-5.1-codex-mini` | $0.25 | $2.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.1 (batch) | `openai/gpt-5.1:batch` | $0.63 | $5.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.2 | `openai/gpt-5.2` | $1.75 | $14.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.2 Chat | `openai/gpt-5.2-chat` | $1.75 | $14.00 | 128K | n/a (missing identity) | n/a |
| GPT-5.2-Codex | `openai/gpt-5.2-codex` | $1.75 | $14.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.2 Pro | `openai/gpt-5.2-pro` | $21.00 | $168.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.2 Pro (batch) | `openai/gpt-5.2-pro:batch` | $10.50 | $84.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.2 (batch) | `openai/gpt-5.2:batch` | $0.88 | $7.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.3-Codex | `openai/gpt-5.3-codex` | $1.75 | $14.00 | 400K | n/a (missing identity) | n/a |
| GPT-5.4 | `openai/gpt-5.4` | $2.50 | $15.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.4 Image 2 | `openai/gpt-5.4-image-2` | $8.00 | $15.00 | 272K | n/a (missing identity) | n/a |
| GPT-5.4 Mini | `openai/gpt-5.4-mini` | $0.75 | $4.50 | 400K | n/a (missing identity) | n/a |
| GPT-5.4 Mini (batch) | `openai/gpt-5.4-mini:batch` | $0.38 | $2.25 | 400K | n/a (missing identity) | n/a |
| GPT-5.4 Nano | `openai/gpt-5.4-nano` | $0.20 | $1.25 | 400K | n/a (missing identity) | n/a |
| GPT-5.4 Nano (batch) | `openai/gpt-5.4-nano:batch` | $0.10 | $0.63 | 400K | n/a (missing identity) | n/a |
| GPT-5.4 Pro | `openai/gpt-5.4-pro` | $30.00 | $180.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.4 Pro (batch) | `openai/gpt-5.4-pro:batch` | $15.00 | $90.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.4 (batch) | `openai/gpt-5.4:batch` | $1.25 | $7.50 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.5 | `openai/gpt-5.5` | $5.00 | $30.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.5 Pro | `openai/gpt-5.5-pro` | $30.00 | $180.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.5 Pro (batch) | `openai/gpt-5.5-pro:batch` | $15.00 | $90.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.5 (batch) | `openai/gpt-5.5:batch` | $2.50 | $15.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.6 Luna Pro (batch) | `openai/gpt-5.6-luna-pro:batch` | $0.10 | $0.60 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.6 Luna (batch) | `openai/gpt-5.6-luna:batch` | $0.10 | $0.60 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.6 Sol Pro (batch) | `openai/gpt-5.6-sol-pro:batch` | $1.00 | $5.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.6 Sol (batch) | `openai/gpt-5.6-sol:batch` | $1.00 | $5.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.6 Terra Pro (batch) | `openai/gpt-5.6-terra-pro:batch` | $1.00 | $6.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5.6 Terra (batch) | `openai/gpt-5.6-terra:batch` | $1.00 | $6.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-5 (batch) | `openai/gpt-5:batch` | $0.63 | $5.00 | 400K | n/a (missing identity) | n/a |
| GPT-6 Astra Pro | `openai/gpt-6-astra-pro` | $10.00 | $50.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-6 Astra Pro (batch) | `openai/gpt-6-astra-pro:batch` | $5.00 | $25.00 | 1.1M | n/a (missing identity) | n/a |
| GPT-6 Astra (batch) | `openai/gpt-6-astra:batch` | $5.00 | $25.00 | 1.1M | n/a (missing identity) | n/a |
| GPT Audio | `openai/gpt-audio` | $2.50 | $10.00 | 128K | n/a (missing identity) | n/a |
| GPT Audio Mini | `openai/gpt-audio-mini` | $0.60 | $2.40 | 128K | n/a (missing identity) | n/a |
| GPT Chat Latest | `openai/gpt-chat-latest` | $5.00 | $30.00 | 400K | n/a (missing identity) | n/a |
| gpt-oss-120b | `openai/gpt-oss-120b` | $0.04 | $0.17 | 131K | n/a (missing identity) | n/a |
| gpt-oss-120b (batch) | `openai/gpt-oss-120b:batch` | $0.15 | $0.60 | 131K | n/a (missing identity) | n/a |
| gpt-oss-20b | `openai/gpt-oss-20b` | $0.03 | $0.13 | 131K | n/a (missing identity) | n/a |
| gpt-oss-20b (batch) | `openai/gpt-oss-20b:batch` | $0.05 | $0.20 | 131K | n/a (missing identity) | n/a |
| gpt-oss-safeguard-20b | `openai/gpt-oss-safeguard-20b` | $0.08 | $0.30 | 131K | n/a (missing identity) | n/a |
| o1 | `openai/o1` | $15.00 | $60.00 | 200K | n/a (missing identity) | n/a |
| o1-pro | `openai/o1-pro` | $150.00 | $600.00 | 200K | n/a (missing identity) | n/a |
| o3 | `openai/o3` | $2.00 | $8.00 | 200K | n/a (missing identity) | n/a |
| o3 Mini | `openai/o3-mini` | $1.10 | $4.40 | 200K | n/a (missing identity) | n/a |
| o3 Mini High | `openai/o3-mini-high` | $1.10 | $4.40 | 200K | n/a (missing identity) | n/a |
| o3 Mini (batch) | `openai/o3-mini:batch` | $0.55 | $2.20 | 200K | n/a (missing identity) | n/a |
| o3 Pro | `openai/o3-pro` | $20.00 | $80.00 | 200K | n/a (missing identity) | n/a |
| o3 (batch) | `openai/o3:batch` | $1.00 | $4.00 | 200K | n/a (missing identity) | n/a |
| o4 Mini | `openai/o4-mini` | $1.10 | $4.40 | 200K | n/a (missing identity) | n/a |
| o4 Mini High | `openai/o4-mini-high` | $1.10 | $4.40 | 200K | n/a (missing identity) | n/a |
| o4 Mini (batch) | `openai/o4-mini:batch` | $0.55 | $2.20 | 200K | n/a (missing identity) | n/a |
| Auto Router | `openrouter/auto` | $-1000000.00 | $-1000000.00 | 2M | n/a (missing identity) | n/a |
| Auto Router (Beta) | `openrouter/auto-beta` | $-1000000.00 | $-1000000.00 | 2M | n/a (missing identity) | n/a |
| Body Builder (beta) | `openrouter/bodybuilder` | $-1000000.00 | $-1000000.00 | 128K | n/a (missing identity) | n/a |
| Free Models Router | `openrouter/free` | $0.00 | $0.00 | 200K | n/a (missing identity) | n/a |
| Fusion | `openrouter/fusion` | $-1000000.00 | $-1000000.00 | 1M | n/a (missing identity) | n/a |
| Pareto Code Router | `openrouter/pareto-code` | $-1000000.00 | $-1000000.00 | 2M | n/a (missing identity) | n/a |
| Perceptron Mk1 | `perceptron/perceptron-mk1` | $0.15 | $1.50 | 33K | n/a (missing identity) | n/a |
| Sonar | `perplexity/sonar` | $1.00 | $1.00 | 127K | n/a (missing identity) | n/a |
| Sonar Deep Research | `perplexity/sonar-deep-research` | $2.00 | $8.00 | 128K | n/a (missing identity) | n/a |
| Sonar Pro | `perplexity/sonar-pro` | $3.00 | $15.00 | 200K | n/a (missing identity) | n/a |
| Sonar Pro Search | `perplexity/sonar-pro-search` | $3.00 | $15.00 | 200K | n/a (missing identity) | n/a |
| Sonar Reasoning Pro | `perplexity/sonar-reasoning-pro` | $2.00 | $8.00 | 128K | n/a (missing identity) | n/a |
| Qwen2.5 72B Instruct | `qwen/qwen-2.5-72b-instruct` | $0.36 | $0.40 | 33K | n/a (missing identity) | n/a |
| Qwen2.5 7B Instruct | `qwen/qwen-2.5-7b-instruct` | $0.10 | $0.20 | 33K | n/a (missing identity) | n/a |
| Qwen2.5 Coder 32B Instruct | `qwen/qwen-2.5-coder-32b-instruct` | $0.66 | $1.00 | 33K | n/a (missing identity) | n/a |
| Qwen-Plus | `qwen/qwen-plus` | $0.26 | $0.78 | 1M | n/a (missing identity) | n/a |
| Qwen Plus 0728 | `qwen/qwen-plus-2025-07-28` | $0.26 | $0.78 | 1M | n/a (missing identity) | n/a |
| Qwen2.5 VL 72B Instruct | `qwen/qwen2.5-vl-72b-instruct` | $0.80 | $1.00 | 128K | n/a (missing identity) | n/a |
| Qwen3 14B | `qwen/qwen3-14b` | $0.12 | $0.24 | 131K | n/a (missing identity) | n/a |
| Qwen3 235B A22B | `qwen/qwen3-235b-a22b` | $0.46 | $1.82 | 131K | n/a (missing identity) | n/a |
| Qwen3 235B A22B Thinking 2507 | `qwen/qwen3-235b-a22b-thinking-2507` | $0.23 | $2.30 | 131K | n/a (missing identity) | n/a |
| Qwen3 30B A3B | `qwen/qwen3-30b-a3b` | $0.12 | $0.50 | 131K | n/a (missing identity) | n/a |
| Qwen3 30B A3B Instruct 2507 | `qwen/qwen3-30b-a3b-instruct-2507` | $0.05 | $0.19 | 262K | n/a (missing identity) | n/a |
| Qwen3 30B A3B Thinking 2507 | `qwen/qwen3-30b-a3b-thinking-2507` | $0.20 | $2.40 | 82K | n/a (missing identity) | n/a |
| Qwen3 32B | `qwen/qwen3-32b` | $0.08 | $0.28 | 131K | n/a (missing identity) | n/a |
| Qwen3 8B | `qwen/qwen3-8b` | $0.12 | $0.46 | 131K | n/a (missing identity) | n/a |
| Qwen3 Coder 30B A3B Instruct | `qwen/qwen3-coder-30b-a3b-instruct` | $0.07 | $0.28 | 262K | n/a (missing identity) | n/a |
| Qwen3 Coder Flash | `qwen/qwen3-coder-flash` | $0.20 | $0.98 | 1M | n/a (missing identity) | n/a |
| Qwen3 Coder Plus | `qwen/qwen3-coder-plus` | $0.65 | $3.25 | 1M | n/a (missing identity) | n/a |
| Qwen3 Max | `qwen/qwen3-max` | $0.78 | $3.90 | 262K | n/a (missing identity) | n/a |
| Qwen3 Max Thinking | `qwen/qwen3-max-thinking` | $0.78 | $3.90 | 262K | n/a (missing identity) | n/a |
| Qwen3 Next 80B A3B Thinking | `qwen/qwen3-next-80b-a3b-thinking` | $0.15 | $1.20 | 262K | n/a (missing identity) | n/a |
| Qwen3 VL 235B A22B Instruct | `qwen/qwen3-vl-235b-a22b-instruct` | $0.21 | $1.90 | 262K | n/a (missing identity) | n/a |
| Qwen3 VL 235B A22B Thinking | `qwen/qwen3-vl-235b-a22b-thinking` | $0.40 | $4.00 | 131K | n/a (missing identity) | n/a |
| Qwen3 VL 30B A3B Instruct | `qwen/qwen3-vl-30b-a3b-instruct` | $0.15 | $0.60 | 262K | n/a (missing identity) | n/a |
| Qwen3 VL 30B A3B Thinking | `qwen/qwen3-vl-30b-a3b-thinking` | $0.20 | $2.40 | 262K | n/a (missing identity) | n/a |
| Qwen3 VL 32B Instruct | `qwen/qwen3-vl-32b-instruct` | $0.10 | $0.42 | 131K | n/a (missing identity) | n/a |
| Qwen3 VL 8B Instruct | `qwen/qwen3-vl-8b-instruct` | $0.12 | $0.46 | 262K | n/a (missing identity) | n/a |
| Qwen3 VL 8B Thinking | `qwen/qwen3-vl-8b-thinking` | $0.18 | $2.10 | 131K | n/a (missing identity) | n/a |
| Qwen3.5-122B-A10B | `qwen/qwen3.5-122b-a10b` | $0.26 | $2.08 | 262K | n/a (missing identity) | n/a |
| Qwen3.5-27B | `qwen/qwen3.5-27b` | $0.20 | $1.56 | 262K | n/a (missing identity) | n/a |
| Qwen3.5-35B-A3B | `qwen/qwen3.5-35b-a3b` | $0.31 | $1.25 | 262K | n/a (missing identity) | n/a |
| Qwen3.5 397B A17B | `qwen/qwen3.5-397b-a17b` | $0.55 | $3.50 | 262K | n/a (missing identity) | n/a |
| Qwen3.5-9B | `qwen/qwen3.5-9b` | $0.10 | $0.15 | 262K | n/a (missing identity) | n/a |
| Qwen3.5-9B (batch) | `qwen/qwen3.5-9b:batch` | $0.17 | $0.25 | 262K | n/a (missing identity) | n/a |
| Qwen3.5-Flash | `qwen/qwen3.5-flash-02-23` | $0.07 | $0.26 | 1M | n/a (missing identity) | n/a |
| Qwen3.5 Plus 2026-02-15 | `qwen/qwen3.5-plus-02-15` | $0.26 | $1.56 | 1M | n/a (missing identity) | n/a |
| Qwen3.5 Plus 2026-04-20 | `qwen/qwen3.5-plus-20260420` | $0.30 | $1.80 | 1M | n/a (missing identity) | n/a |
| Qwen3.6 27B | `qwen/qwen3.6-27b` | $0.30 | $2.00 | 262K | n/a (missing identity) | n/a |
| Qwen3.6 35B A3B | `qwen/qwen3.6-35b-a3b` | $0.10 | $0.90 | 262K | n/a (missing identity) | n/a |
| Qwen3.6 Flash | `qwen/qwen3.6-flash` | $0.19 | $1.13 | 1M | n/a (missing identity) | n/a |
| Qwen3.6 Max Preview | `qwen/qwen3.6-max-preview` | $1.03 | $6.16 | 262K | n/a (missing identity) | n/a |
| Qwen3.6 Plus | `qwen/qwen3.6-plus` | $0.33 | $1.95 | 1M | n/a (missing identity) | n/a |
| Qwen3.8 2.4T A95B | `qwen/qwen3.8-2.4t-a95b` | $2.00 | $6.00 | 1M | n/a (missing identity) | n/a |
| Qwen3.8 2.4T A95B (batch) | `qwen/qwen3.8-2.4t-a95b:batch` | $2.00 | $6.00 | 1M | n/a (missing identity) | n/a |
| Qwen3.8 27B | `qwen/qwen3.8-27b` | $0.21 | $2.55 | 1M | n/a (missing identity) | n/a |
| Qwen3.8 Flash | `qwen/qwen3.8-flash` | $0.15 | $0.47 | 1M | n/a (missing identity) | n/a |
| Reka Edge | `rekaai/reka-edge` | $0.10 | $0.10 | 16K | n/a (missing identity) | n/a |
| Reka Flash 3 | `rekaai/reka-flash-3` | $0.10 | $0.20 | 66K | n/a (missing identity) | n/a |
| Relace Apply 3 | `relace/relace-apply-3` | $0.85 | $1.25 | 256K | n/a (missing identity) | n/a |
| Relace Search | `relace/relace-search` | $1.00 | $3.00 | 256K | n/a (missing identity) | n/a |
| Fugu Max | `sakana/fugu-max` | $2.00 | $6.00 | 1M | n/a (missing identity) | n/a |
| Fugu Ultra | `sakana/fugu-ultra` | $5.00 | $30.00 | 1M | n/a (missing identity) | n/a |
| Fugu Ultra v2 | `sakana/fugu-ultra-v2` | $5.00 | $30.00 | 1M | n/a (missing identity) | n/a |
| Sakana Namazu | `sakana/sakana-namazu` | $0.95 | $4.00 | 262K | n/a (missing identity) | n/a |
| Llama 3 8B Lunaris | `sao10k/l3-lunaris-8b` | $0.04 | $0.05 | 8K | n/a (missing identity) | n/a |
| Llama 3.1 Euryale 70B v2.2 | `sao10k/l3.1-euryale-70b` | $0.85 | $0.85 | 131K | n/a (missing identity) | n/a |
| Llama 3.3 Euryale 70B | `sao10k/l3.3-euryale-70b` | $0.65 | $0.75 | 131K | n/a (missing identity) | n/a |
| Step 3.5 Flash | `stepfun/step-3.5-flash` | $0.10 | $0.30 | 262K | n/a (missing identity) | n/a |
| Step 3.7 Flash | `stepfun/step-3.7-flash` | $0.20 | $1.15 | 262K | n/a (missing identity) | n/a |
| Hunyuan A13B Instruct | `tencent/hunyuan-a13b-instruct` | $0.14 | $0.57 | 131K | n/a (missing identity) | n/a |
| Hy-MT2-1.8B | `tencent/hy-mt2-1.8b` | $0.04 | $0.18 | 8K | n/a (missing identity) | n/a |
| Hy-MT2-30B-A3B | `tencent/hy-mt2-30b-a3b` | $0.07 | $0.30 | 8K | n/a (missing identity) | n/a |
| Hy-MT2-7B | `tencent/hy-mt2-7b` | $0.07 | $0.30 | 8K | n/a (missing identity) | n/a |
| Hy3 preview | `tencent/hy3-preview` | $0.18 | $0.60 | 262K | n/a (missing identity) | n/a |
| Hy4 preview | `tencent/hy4-preview` | $0.83 | $2.50 | 1M | n/a (missing identity) | n/a |
| Cydonia 24B V4.1 | `thedrummer/cydonia-24b-v4.1` | $0.30 | $0.50 | 131K | n/a (missing identity) | n/a |
| Skyfall 36B V2 | `thedrummer/skyfall-36b-v2` | $0.55 | $0.80 | 33K | n/a (missing identity) | n/a |
| UnslopNemo 12B | `thedrummer/unslopnemo-12b` | $0.40 | $0.40 | 1M | n/a (missing identity) | n/a |
| Thinking Machines: Inkling | `thinkingmachines/inkling` | $1.00 | $4.05 | 1M | n/a (missing identity) | n/a |
| Thinking Machines: Inkling Small | `thinkingmachines/inkling-small` | $0.45 | $1.20 | 1M | n/a (missing identity) | n/a |
| Thinking Machines: Inkling Small (batch) | `thinkingmachines/inkling-small:batch` | $0.50 | $1.20 | 524K | n/a (missing identity) | n/a |
| Thinking Machines: Inkling Small (free) | `thinkingmachines/inkling-small:free` | $0.00 | $0.00 | 1M | n/a (missing identity) | n/a |
| Thinking Machines: Inkling (batch) | `thinkingmachines/inkling:batch` | $1.00 | $4.05 | 524K | n/a (missing identity) | n/a |
| Thinking Machines: Inkling (free) | `thinkingmachines/inkling:free` | $0.00 | $0.00 | 1M | n/a (missing identity) | n/a |
| ReMM SLERP 13B | `undi95/remm-slerp-l2-13b` | $0.35 | $0.65 | 6K | n/a (missing identity) | n/a |
| Solar Pro 3 | `upstage/solar-pro-3` | $0.15 | $0.60 | 131K | n/a (missing identity) | n/a |
| Solar Pro 4 | `upstage/solar-pro4` | $0.09 | $0.36 | 524K | n/a (missing identity) | n/a |
| Palmyra X5 | `writer/palmyra-x5` | $0.60 | $6.00 | 1M | n/a (missing identity) | n/a |
| SpaceXAI: Grok 4.20 | `x-ai/grok-4.20` | $1.25 | $2.50 | 2M | n/a (missing identity) | n/a |
| SpaceXAI: Grok 4.20 Multi-Agent | `x-ai/grok-4.20-multi-agent` | $1.25 | $2.50 | 2M | n/a (missing identity) | n/a |
| SpaceXAI: Grok 4.3 | `x-ai/grok-4.3` | $1.25 | $2.50 | 1M | n/a (missing identity) | n/a |
| SpaceXAI: Grok 4.3 (batch) | `x-ai/grok-4.3:batch` | $1.00 | $2.00 | 1M | n/a (missing identity) | n/a |
| SpaceXAI: Grok Build 0.1 | `x-ai/grok-build-0.1` | $1.00 | $2.00 | 256K | n/a (missing identity) | n/a |
| GLM 4.5 | `z-ai/glm-4.5` | $0.60 | $2.20 | 131K | n/a (missing identity) | n/a |
| GLM 4.5 Air | `z-ai/glm-4.5-air` | $0.13 | $0.85 | 131K | n/a (missing identity) | n/a |
| GLM 4.5V | `z-ai/glm-4.5v` | $0.60 | $1.80 | 66K | n/a (missing identity) | n/a |
| GLM 4.6 | `z-ai/glm-4.6` | $0.43 | $1.75 | 205K | n/a (missing identity) | n/a |
| GLM 4.6V | `z-ai/glm-4.6v` | $0.30 | $0.90 | 131K | n/a (missing identity) | n/a |
| GLM 5 | `z-ai/glm-5` | $0.60 | $1.92 | 205K | n/a (missing identity) | n/a |
| GLM 5 Turbo | `z-ai/glm-5-turbo` | $1.20 | $4.00 | 203K | n/a (missing identity) | n/a |
| GLM 5.1 | `z-ai/glm-5.1` | $0.97 | $3.04 | 205K | n/a (missing identity) | n/a |
| GLM 5.2 (batch) | `z-ai/glm-5.2:batch` | $0.70 | $2.20 | 1M | n/a (missing identity) | n/a |
| GLM 5.3 Flash (batch) | `z-ai/glm-5.3-flash:batch` | $0.08 | $0.25 | 1M | n/a (missing identity) | n/a |
| GLM 5.3 (batch) | `z-ai/glm-5.3:batch` | $0.70 | $2.20 | 1M | n/a (missing identity) | n/a |
| GLM 5V Turbo | `z-ai/glm-5v-turbo` | $1.20 | $4.00 | 203K | n/a (missing identity) | n/a |
| Anthropic: Claude Fable Latest | `~anthropic/claude-fable-latest` | $10.00 | $50.00 | 1M | n/a (missing identity) | n/a |
| Anthropic: Claude Haiku Latest | `~anthropic/claude-haiku-latest` | $1.00 | $5.00 | 200K | n/a (missing identity) | n/a |
| Anthropic: Claude Opus Latest | `~anthropic/claude-opus-latest` | $5.00 | $25.00 | 1M | n/a (missing identity) | n/a |
| Anthropic: Claude Sonnet Latest | `~anthropic/claude-sonnet-latest` | $2.00 | $10.00 | 1M | n/a (missing identity) | n/a |
| DeepSeek: DeepSeek V4 Flash Latest | `~deepseek/deepseek-v4-flash-latest` | $0.04 | $0.10 | 1.3M | n/a (missing identity) | n/a |
| Google: Gemini Flash Latest | `~google/gemini-flash-latest` | $0.75 | $3.75 | 1M | n/a (missing identity) | n/a |
| Google: Gemini Pro Latest | `~google/gemini-pro-latest` | $2.00 | $12.00 | 1M | n/a (missing identity) | n/a |
| MoonshotAI: Kimi Latest | `~moonshotai/kimi-latest` | $2.10 | $10.95 | 1M | n/a (missing identity) | n/a |
| OpenAI: GPT Astra Latest | `~openai/gpt-astra-latest` | $10.00 | $50.00 | 1.1M | n/a (missing identity) | n/a |
| OpenAI: GPT Luna Latest | `~openai/gpt-luna-latest` | $0.20 | $1.20 | 1.1M | n/a (missing identity) | n/a |
| OpenAI: GPT Mini Latest | `~openai/gpt-mini-latest` | $0.75 | $4.50 | 400K | n/a (missing identity) | n/a |
| OpenAI: GPT Sol Latest | `~openai/gpt-sol-latest` | $2.00 | $10.00 | 1.1M | n/a (missing identity) | n/a |
| OpenAI: GPT Terra Latest | `~openai/gpt-terra-latest` | $2.00 | $12.00 | 1.1M | n/a (missing identity) | n/a |
| xAI: Grok Latest | `~x-ai/grok-latest` | $2.00 | $6.00 | 500K | n/a (missing identity) | n/a |
| Z.ai: GLM Flash Latest | `~z-ai/glm-flash-latest` | $0.08 | $0.25 | 1.3M | n/a (missing identity) | n/a |
| Z.ai: GLM Latest | `~z-ai/glm-latest` | $0.92 | $3.14 | 1.3M | n/a (missing identity) | n/a |

## Приложение: происхождение оценок

Служебный раздел для аудита: полная provenance каждого опубликованного
наблюдения. Для чтения и выбора модели он не нужен — смотрите таблицы выше.

- `xiaomi/mimo-v2.5` — raw=71; metric=SWE-bench Verified; unit=%; variant=xiaomi/mimo-v2.5; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Xiaomi; configuration=n/a; configured_identity=xiaomi/mimo-v2.5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `z-ai/glm-5.3-flash` — raw=92; metric=SWE-bench Verified; unit=%; variant=zai/glm-5.3-flash; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Zhipu AI; configuration=n/a; configured_identity=zai/glm-5.3-flash; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `deepseek/deepseek-v3.2` — raw=70; metric=SWE-bench Verified; unit=%; variant=DeepSeek V3.2 (high); identity=exact_product; checked=2026-02-17; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (DeepSeek V3.2 (high)); provider=n/a; configuration=n/a; configured_identity=Model: deepseek-v3.2; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `openai/gpt-5.6-luna` — raw=93; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-luna; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a; configured_identity=openai/gpt-5.6-luna; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `minimax/minimax-m2.5` — raw=74.2; metric=SWE-bench Verified; unit=%; variant=minimax/MiniMax-M2.5; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=MiniMax; configuration=n/a; configured_identity=minimax/MiniMax-M2.5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `minimax/minimax-m3` — raw=75; metric=SWE-bench Verified; unit=%; variant=minimax/MiniMax-M3; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=MiniMax; configuration=n/a; configured_identity=minimax/MiniMax-M3; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `minimax/minimax-m2` — raw=61; metric=SWE-bench Verified; unit=%; variant=MiniMax M2; identity=exact_product; checked=2025-11-24; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (MiniMax M2); provider=n/a; configuration=n/a; configured_identity=Model: minimax-m2; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `xiaomi/mimo-v2.5-pro` — raw=74; metric=SWE-bench Verified; unit=%; variant=xiaomi/mimo-v2.5-pro; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Xiaomi; configuration=n/a; configured_identity=xiaomi/mimo-v2.5-pro; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `qwen/qwen3-coder` — raw=55.4; metric=SWE-bench Verified; unit=%; variant=Qwen3-Coder 480B/A35B Instruct; identity=exact_product; checked=2025-08-02; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (Qwen3-Coder 480B/A35B Instruct); provider=n/a; configuration=n/a; configured_identity=Model: Qwen3-Coder-480B-A35B-Instruct; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `z-ai/glm-4.7` — raw=69.4; metric=SWE-bench Verified; unit=%; variant=zai/glm-4.7; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Zhipu AI; configuration=n/a; configured_identity=zai/glm-4.7; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `openai/gpt-5-mini` — raw=58; metric=SWE-bench Verified; unit=%; variant=gpt-5-mini-2025-08-07 (median of 2 scaffolds); identity=exact_product; checked=2025-08-07..2026-02-17; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=median of 2 scaffolds (highest individual: GPT 5 mini (2025-08-07) (medium)); provider=n/a; configuration=n/a; configured_identity=Model: gpt-5-mini-2025-08-07; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `z-ai/glm-5.2` — raw=82.8; metric=SWE-bench Verified; unit=%; variant=zai/glm-5.2; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Zhipu AI; configuration=n/a; configured_identity=zai/glm-5.2; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `moonshotai/kimi-k2.5` — raw=70.8; metric=SWE-bench Verified; unit=%; variant=Kimi K2.5 (high); identity=exact_product; checked=2026-02-17; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (Kimi K2.5 (high)); provider=n/a; configuration=n/a; configured_identity=Model: kimi-k2.5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `nvidia/nemotron-3-ultra-550b-a55b` — raw=69; metric=SWE-bench Verified; unit=%; variant=nvidia/nemotron-3-ultra-550b-a55b; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Nvidia; configuration=n/a; configured_identity=nvidia/nemotron-3-ultra-550b-a55b; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `meta-llama/llama-4-maverick` — raw=21.04; metric=SWE-bench Verified; unit=%; variant=Llama 4 Maverick Instruct; identity=exact_product; checked=2025-07-20; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (Llama 4 Maverick Instruct); provider=n/a; configuration=n/a; configured_identity=Model: llama-4-maverick-instruct; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `meta-llama/llama-4-scout` — raw=9.06; metric=SWE-bench Verified; unit=%; variant=Llama 4 Scout Instruct; identity=exact_product; checked=2025-07-20; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (Llama 4 Scout Instruct); provider=n/a; configuration=n/a; configured_identity=Model: llama-4-scout-instruct; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `moonshotai/kimi-k2.7-code` — raw=78.2; metric=SWE-bench Verified; unit=%; variant=kimi/kimi-k2.7-code; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Moonshot AI; configuration=n/a; configured_identity=kimi/kimi-k2.7-code; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `mistralai/mistral-large-2512` — raw=41.4; metric=SWE-bench Verified; unit=%; variant=mistralai/mistral-large-2512; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Mistral AI; configuration=n/a; configured_identity=mistralai/mistral-large-2512; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `google/gemini-3.7-flash` — raw=80.8; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.7-flash; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a; configured_identity=google/gemini-3.7-flash; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `google/gemini-3.8-flash` — raw=80; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.8-flash; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a; configured_identity=google/gemini-3.8-flash; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `google/gemini-3.6-flash` — raw=79.6; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.6-flash; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a; configured_identity=google/gemini-3.6-flash; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `z-ai/glm-5.3` — raw=95.4; metric=SWE-bench Verified; unit=%; variant=zai/glm-5.3; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Zhipu AI; configuration=n/a; configured_identity=zai/glm-5.3; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `meta/muse-spark-1.1` — raw=82; metric=SWE-bench Verified; unit=%; variant=meta/muse_spark_1_1; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Meta; configuration=n/a; configured_identity=meta/muse_spark_1_1; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `deepseek/deepseek-v4-pro` — raw=77.4; metric=SWE-bench Verified; unit=%; variant=deepseek/deepseek-v4-pro; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=DeepSeek; configuration=n/a; configured_identity=deepseek/deepseek-v4-pro; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `anthropic/claude-haiku-4.5` — raw=66.6; metric=SWE-bench Verified; unit=%; variant=Claude 4.5 Haiku (high); identity=exact_product; checked=2026-02-17; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (Claude 4.5 Haiku (high)); provider=n/a; configuration=n/a; configured_identity=Model: claude-haiku-4-5-20251001; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `x-ai/grok-4.6` — raw=95.6; metric=SWE-bench Verified; unit=%; variant=grok/grok-4.6; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=SpaceXAI; configuration=n/a; configured_identity=grok/grok-4.6; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `qwen/qwen3.7-max` — raw=68.8; metric=SWE-bench Verified; unit=%; variant=alibaba/qwen3.7-max; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Alibaba; configuration=n/a; configured_identity=alibaba/qwen3.7-max; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `x-ai/grok-4.5` — raw=86.6; metric=SWE-bench Verified; unit=%; variant=grok/grok-4.5; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=SpaceXAI; configuration=n/a; configured_identity=grok/grok-4.5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `qwen/qwen3.8-max-0902` — raw=85.6; metric=SWE-bench Verified; unit=%; variant=alibaba/qwen3.8-max; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Alibaba; configuration=n/a; configured_identity=alibaba/qwen3.8-max; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `openai/gpt-5.6-sol` — raw=96.2; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-sol; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a; configured_identity=openai/gpt-5.6-sol; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `google/gemini-3.5-flash` — raw=78.8; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.5-flash; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a; configured_identity=google/gemini-3.5-flash; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `mistralai/mistral-medium-3-5` — raw=66.4; metric=SWE-bench Verified; unit=%; variant=mistralai/mistral-medium-3.5; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Mistral AI; configuration=n/a; configured_identity=mistralai/mistral-medium-3.5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `openai/gpt-5.6-terra` — raw=95.4; metric=SWE-bench Verified; unit=%; variant=openai/gpt-5.6-terra; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=OpenAI; configuration=n/a; configured_identity=openai/gpt-5.6-terra; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `anthropic/claude-sonnet-5` — raw=79.6; metric=SWE-bench Verified; unit=%; variant=anthropic/claude-sonnet-5; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Anthropic; configuration=n/a; configured_identity=anthropic/claude-sonnet-5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `moonshotai/kimi-k3` — raw=93.4; metric=SWE-bench Verified; unit=%; variant=kimi/kimi-k3; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Moonshot AI; configuration=n/a; configured_identity=kimi/kimi-k3; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `google/gemini-3.1-pro-preview` — raw=78.8; metric=SWE-bench Verified; unit=%; variant=google/gemini-3.1-pro-preview; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Google; configuration=n/a; configured_identity=google/gemini-3.1-pro-preview; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `anthropic/claude-sonnet-4.6` — raw=77.4; metric=SWE-bench Verified; unit=%; variant=anthropic/claude-sonnet-4-6; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Anthropic; configuration=n/a; configured_identity=anthropic/claude-sonnet-4-6; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `anthropic/claude-sonnet-4.5` — raw=74.8; metric=SWE-bench Verified; unit=%; variant=Sonar Foundation Agent + Claude 4.5 Sonnet; identity=exact_product; checked=2025-11-03; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (Sonar Foundation Agent + Claude 4.5 Sonnet); provider=n/a; configuration=n/a; configured_identity=Model: claude-sonnet-4-5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `anthropic/claude-opus-5` — raw=97; metric=SWE-bench Verified; unit=%; variant=anthropic/claude-opus-5; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Anthropic; configuration=n/a; configured_identity=anthropic/claude-opus-5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `anthropic/claude-opus-4.8` — raw=88.6; metric=SWE-bench Verified; unit=%; variant=anthropic/claude-opus-4-8; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Anthropic; configuration=n/a; configured_identity=anthropic/claude-opus-4-8; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `anthropic/claude-opus-4.7` — raw=82; metric=SWE-bench Verified; unit=%; variant=anthropic/claude-opus-4-7; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Anthropic; configuration=n/a; configured_identity=anthropic/claude-opus-4-7; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `anthropic/claude-opus-4.6` — raw=75.6; metric=SWE-bench Verified; unit=%; variant=Claude 4.6 Opus; identity=exact_product; checked=2026-02-17; source=https://www.swebench.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=single scaffold (Claude 4.6 Opus); provider=n/a; configuration=n/a; configured_identity=Model: claude-opus-4-6; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.swebench.com/
- `anthropic/claude-fable-5` — raw=95; metric=SWE-bench Verified; unit=%; variant=anthropic/claude-fable-5; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Anthropic; configuration=n/a; configured_identity=anthropic/claude-fable-5; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `cohere/north-mini-code:free` — raw=67.6; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://cohere.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://cohere.com/
- `deepseek/deepseek-v4-flash` — raw=88.8; metric=SWE-bench Verified; unit=%; variant=deepseek/deepseek-v4-flash-0731; identity=variant_mismatch; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=DeepSeek; configuration=n/a; configured_identity=deepseek/deepseek-v4-flash-0731; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `mistralai/devstral-2512` — raw=72.2; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://mistral.ai/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://mistral.ai/
- `nvidia/nemotron-3-super-120b-a12b:free` — raw=60.47; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://developer.nvidia.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://developer.nvidia.com/
- `nvidia/nemotron-3-ultra-550b-a55b:free` — raw=69; metric=SWE-bench Verified; unit=%; variant=nvidia/nemotron-3-ultra-550b-a55b; identity=exact_product; checked=2026-09-01; source=https://www.vals.ai/benchmarks/swebench; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=Nvidia; configuration=n/a; configured_identity=nvidia/nemotron-3-ultra-550b-a55b; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://www.vals.ai/benchmarks/swebench
- `poolside/laguna-xs-2.1:free` — raw=70.9; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://poolside.ai/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://poolside.ai/
- `qwen/qwen3-coder-next` — raw=70.6; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://qwenlm.github.io/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://qwenlm.github.io/
- `tencent/hy3` — raw=78; metric=SWE-bench Verified; unit=%; variant=vendor-claimed; identity=observation_only; checked=n/a; source=https://hunyuan.tencent.com/; uncertainty=n/a; sample=n/a; harness=n/a; scaffold=n/a; provider=n/a; configuration=n/a; configured_identity=n/a; canonical_id=n/a; release_variant=n/a; model_variant=n/a; reasoning=n/a; provenance=https://hunyuan.tencent.com/
