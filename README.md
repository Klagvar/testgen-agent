# testgen-agent

**LLM-агент для автоматической генерации Go-юнит-тестов на изменённых
функциях с гибридным статическим анализом, мутационным тестированием и
JSON-отчётами для воспроизводимых экспериментов.**

Агент анализирует `git diff` между текущей и базовой ветками, выполняет
типизированный AST-анализ изменённого кода (`go/types` + `golang.org/x/tools/go/packages`),
формирует структурированный промпт и вызывает LLM через любой
OpenAI-совместимый HTTP API. Сгенерированные тесты компилируются,
прогоняются с парсингом `go test -json`, итеративно дотачиваются под
target coverage, и при необходимости проверяются мутационным тестированием.
Каждый прогон сохраняет машиночитаемый JSON-отчёт со всеми метриками,
параметрами модели и конфигурацией ablation, что делает результаты
повторяемыми. Поставляется как самостоятельный CLI и как
GitHub Action, оставляющая комментарии в Pull Request с агрегированной
сводкой.

## Архитектура

```
                              ┌──────────────────────────────┐
                              │         git diff             │
                              └─────────────┬────────────────┘
                                            ▼
   ┌───────────────────────────────────────────────────────────────────┐
   │  Static analysis (internal/typed, analyzer, patterns)             │
   │   ├── go/types: cross-file resolution, interface satisfaction     │
   │   ├── concurrency / generics / receiver / package-level vars      │
   │   └── idiomatic pattern detector (HTTP, context, errors, …)       │
   └───────────────────────────────────────────────────────────────────┘
                                            ▼
   ┌───────────────────────────────────────────────────────────────────┐
   │  Prompt builder (internal/prompt) → LLM (internal/llm)            │
   │   ├── token budget, mock injection, structured pattern hints      │
   │   └── temperature / seed for reproducibility                      │
   └───────────────────────────────────────────────────────────────────┘
                                            ▼
   ┌───────────────────────────────────────────────────────────────────┐
   │  Validate (internal/validator + testjson) → Repair → Prune        │
   │   ├── go test -json structured feedback                           │
   │   ├── retry with parsed expected/got                              │
   │   └── AST pruning of incurably failing tests                      │
   └───────────────────────────────────────────────────────────────────┘
                                            ▼
   ┌───────────────────────────────────────────────────────────────────┐
   │  AST transforms (internal/merger, dedup) → write *_test.go        │
   │   ├── merge with existing test file (no overwrites)               │
   │   └── deduplicate equivalent table-driven cases                   │
   └───────────────────────────────────────────────────────────────────┘
                                            ▼
   ┌───────────────────────────────────────────────────────────────────┐
   │  Metrics                                                          │
   │   ├── coverage / branchcov / error-path coverage                  │
   │   ├── mutation testing (binary/logical/return/arithmetic)         │
   │   ├── naturalness suite (assertions, naming, duplicates)          │
   │   └── token efficiency                                            │
   └───────────────────────────────────────────────────────────────────┘
                                            ▼
   ┌───────────────────────────────────────────────────────────────────┐
   │  Reporting                                                        │
   │   ├── JSON record (internal/report) — machine-readable, all knobs │
   │   ├── HTML dashboard (artifact)                                   │
   │   └── GitHub PR comment (internal/github)                         │
   └───────────────────────────────────────────────────────────────────┘
```

## Модули

| Модуль | Назначение |
|--------|------------|
| `cmd/agent` | CLI-оркестратор: парсинг флагов, вызов pipeline, запись отчётов |
| `cmd/ablate` | Раннер ablation-экспериментов: одна конфигурация × `--runs N` |
| `cmd/ablate-report` | Агрегатор JSON-отчётов в сводную CSV/Markdown-таблицу |
| `cmd/benchmark` | Multi-repository raнер: dataset.yaml × ablation × runs |
| `cmd/benchmark-report` | Агрегатор по репозиториям + кросс-модельные таблицы |
| `internal/diff` | Парсер `git diff` — файлы, хунки, изменённые строки |
| `internal/gitdiff` | Per-function git-сравнение с AST-нормализацией тел функций |
| `internal/typed` | Кэш `go/types`-проанализированных пакетов (через `golang.org/x/tools/go/packages`) |
| `internal/analyzer` | AST-анализ функций, типов, embedded-интерфейсов, generics, concurrency |
| `internal/patterns` | Детектор Go-идиом (HTTP, context, time, env, file I/O, SQL, errors, JSON, io.Reader/Writer) |
| `internal/prompt` | Построение промптов и token budget |
| `internal/llm` | Клиент OpenAI-совместимых API с retry/backoff, поддержкой `temperature` и `seed` |
| `internal/validator` | `go build` + `go test -race` + `go test -json` с парсингом результатов |
| `internal/testjson` | Парсер `go test -json` событий → структурированный feedback для repair-prompt |
| `internal/pruner` | AST-удаление неисправимо падающих тестов |
| `internal/merger` | AST-слияние сгенерированных тестов с существующим файлом |
| `internal/dedup` | AST-дедупликация семантически эквивалентных тест-кейсов |
| `internal/mockgen` | Детерминированная генерация моков для интерфейсов + авто-инжект |
| `internal/coverage` | `go test -coverprofile`, расчёт diff coverage с фильтрацией неисполняемых строк |
| `internal/branchcov` | Branch coverage и error-path coverage (специфичная для Go метрика `if err != nil`) |
| `internal/mutation` | Мутационное тестирование на изменённых строках (4 группы операторов) |
| `internal/naturalness` | Метрики «естественности» сгенерированных тестов (assertions, наименования) |
| `internal/cache` | Функционально-уровневый кэш по SHA-256 для пропуска LLM-вызовов на неизменённых функциях |
| `internal/config` | Конфигурация `.testgen.yml` (модель, threshold, exclude/include_only) |
| `internal/report` | JSON и HTML отчёты, агрегатор `BuildTotals` |
| `internal/github` | PR-комментарии через GitHub API с привязкой к коммиту |
| `internal/ablation` | Раннер ablation-конфигураций (`full`, `no-types`, …) с поддержкой повторных запусков |
| `internal/benchmark` | Multi-repo датасет, клонирование с фиксированными `base`/`head` SHA, сброс рабочей копии между прогонами |

**24 пакета (5 CLI + 19 внутренних), ≈330 юнит-тестов.**

## Ключевые возможности

- **Diff-ориентированность.** Анализируется только то, что изменилось в
  `base..head`; LLM получает контекст ровно по затронутым функциям.
- **Гибридный статический анализ.** `go/types` + `golang.org/x/tools/go/packages`
  для resolution типов, интерфейсов и кросс-пакетных вызовов; синтаксический
  fallback, когда type-checking не доступен.
- **Структурированная обратная связь.** `go test -json` парсится в
  событийную модель; в repair-prompt уходит конкретное `expected/got`,
  а не сырое stderr.
- **AST-преобразования вместо текстовых склеек.** Слияние с существующим
  файлом, обрезка падающих тестов, дедупликация — всё через AST.
- **Идиоматические паттерны.** Детектор Go-идиом (HTTP-handler, context,
  errors.Is/As, io.Reader/Writer и др.) подмешивает прицельные подсказки
  в промпт.
- **Мутационное тестирование.** Четыре группы операторов (бинарные
  отношения, логические связки, возвращаемое значение, арифметика) на
  изменённых строках, в безопасной temp-копии.
- **Метрики качества.** Diff coverage, branch coverage, error-path
  coverage (`if err != nil`), mutation score, pass-rate, token efficiency,
  naturalness suite.
- **Кэш для CI/CD.** Функциональный кэш (SHA-256 от сигнатуры + тела +
  типов) на повторные прогоны одного PR — экономит до ~80% токенов.
- **Воспроизводимость.** `--temperature` и `--seed` пробрасываются в
  тело HTTP-запроса; все параметры (модель, seed, temperature, run-index,
  ablation config, тэги репозитория) сохраняются в JSON-отчёте.
- **Экспериментальная инфраструктура.** `cmd/ablate` и `cmd/benchmark`
  с `--runs N` и `--seed-base` для статистических экспериментов на
  множестве моделей и репозиториев. Между прогонами рабочая копия
  принудительно сбрасывается через `git reset --hard && git clean -fdx`,
  чтобы конфигурации не загрязняли друг друга.
- **Модель-агностичность.** Любой OpenAI-совместимый HTTP API: OpenAI,
  Ollama, OpenRouter, Groq, vLLM, локальный llama.cpp.
- **GitHub-интеграция.** Reusable Action с PR-комментарием и загрузкой
  HTML/JSON артефактов.

## Быстрый старт

### Установка

```bash
git clone https://github.com/Klagvar/testgen-agent.git
cd testgen-agent
go build -o testgen-agent ./cmd/agent
go install golang.org/x/tools/cmd/goimports@latest
```

### Запуск локально

```bash
# Локальная Ollama
./testgen-agent --repo /path/to/project --base main \
  --api-url http://localhost:11434/v1 --model qwen2.5-coder:32b

# OpenAI
./testgen-agent --repo /path/to/project --base main \
  --api-key sk-... --model gpt-4o-mini

# OpenRouter
./testgen-agent --repo /path/to/project --base main \
  --api-url https://openrouter.ai/api/v1 \
  --api-key sk-or-... --model openai/gpt-4o-mini

# Полная конфигурация для эксперимента
./testgen-agent --repo /path/to/project --base main \
  --api-key $TESTGEN_API_KEY --model openai/gpt-4o-mini \
  --temperature 0 --seed 42 --run-index 1 \
  --mutation --report json --ablation-config full

# Dry-run: показать промпт, не вызывая LLM
./testgen-agent --repo /path/to/project --base main --dry-run
```

### Конфиг-файл `.testgen.yml`

```yaml
model: qwen2.5-coder:32b
api_url: http://localhost:11434/v1
coverage_threshold: 80
max_retries: 3
max_context_tokens: 16000

exclude:
  - "vendor/**"
  - "generated/**"

include_only: "src/**"   # строка или список

race_detection: true
mutation: true
report_format: json

custom_prompt: |
  Always use table-driven tests.
  Do not use external libraries.
```

### Переменные окружения (`.env`)

```bash
# LLM
TESTGEN_API_KEY=sk-your-key-here
TESTGEN_API_URL=https://openrouter.ai/api/v1
TESTGEN_MODEL=openai/gpt-4o-mini
TESTGEN_TEMPERATURE=0      # опционально, переопределяется --temperature
TESTGEN_SEED=42            # опционально, переопределяется --seed

# GitHub Actions
GITHUB_TOKEN=github_pat_...
GITHUB_REPOSITORY=owner/repo
TESTGEN_PR_NUMBER=1
```

### CLI-флаги (cmd/agent)

| Флаг | Описание | По умолчанию |
|------|----------|--------------|
| `--repo` | Путь к Git-репозиторию | `.` |
| `--base` | Базовая ветка / SHA для `git diff` | `main` |
| `--api-key` | API-ключ LLM (или env `TESTGEN_API_KEY`) | — |
| `--api-url` | URL API (или env `TESTGEN_API_URL`) | OpenAI |
| `--model` | Модель (или env `TESTGEN_MODEL`) | `gpt-4o-mini` |
| `--temperature` | LLM sampling temperature (`<0` ⇒ default бэкенда) | `-1` |
| `--seed` | LLM sampling seed (`0` ⇒ unset) | `0` |
| `--run-index` | Индекс прогона в multi-run эксперименте | `0` |
| `--out` | Каталог для сгенерированных тестов | рядом с исходниками |
| `--dry-run` | Показать промпт без вызова LLM | `false` |
| `--no-validate` | Пропустить валидацию | `false` |
| `--coverage` | Целевой diff coverage (%) | `80` |
| `--mutation` | Запустить мутационное тестирование | `false` |
| `--race` | `go test -race` для concurrent-функций | `false` |
| `--report` | `html` или `json` (пусто ⇒ только PR-комментарий) | пусто |
| `--no-cache` | Отключить функциональный кэш | `false` |
| `--no-smart-diff` | Отключить per-function git-сравнение | `false` |
| `--no-types` | Ablation: только синтаксический анализ (без `go/types`) | `false` |
| `--no-structured-feedback` | Ablation: сырое stderr вместо `go test -json` | `false` |
| `--no-pruning` | Ablation: не отбрасывать падающие тесты | `false` |
| `--no-coverage` | Ablation: пропустить итеративный coverage-loop | `false` |
| `--no-mutation` | Ablation: принудительно отключить мутации | `false` |
| `--no-naturalness` | Ablation: пропустить naturalness-метрики | `false` |
| `--ablation-config` | Метка конфигурации в JSON-отчёте (`full`, `no-types`, …) | пусто |
| `--github-token` | Токен для PR-комментария | — |
| `--github-repo` | Репозиторий (`owner/repo`) | — |
| `--pr-number` | Номер Pull Request | — |

## CI/CD — GitHub Actions

```yaml
name: Testgen
on:
  pull_request:
    paths: ['**.go', '!**_test.go']

jobs:
  generate-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.head_ref }}
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.26'

      - run: go build -o testgen-agent ./cmd/agent
      - run: git fetch origin ${{ github.base_ref }} --depth=1

      - run: |
          ./testgen-agent --repo . --base "origin/${{ github.base_ref }}" \
            --api-url ${{ secrets.LLM_API_URL }} \
            --api-key ${{ secrets.LLM_API_KEY }} \
            --model ${{ vars.LLM_MODEL }} \
            --temperature 0 \
            --mutation --race --report json \
            --github-token ${{ secrets.GITHUB_TOKEN }} \
            --github-repo ${{ github.repository }} \
            --pr-number ${{ github.event.pull_request.number }}

      - name: Upload JSON report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: testgen-json-report
          path: testgen-report-*.json

      - name: Commit generated tests
        if: success()
        run: |
          git add '**/*_test.go'
          git diff --cached --quiet || git commit -m "🤖 testgen-agent: auto-generated tests"
          git push
```

## Экспериментальный стенд

Для воспроизводимой оценки метода поставляются два orchestrator'а:

```bash
# Ablation на одном репозитории: 6 конфигов × 3 повтора
./testgen-ablate \
  --agent ./testgen-agent --repo /path/to/project --base main \
  --model openai/gpt-4o-mini --runs 3 --seed-base 42 \
  --out ./ablation-results

# Multi-repository куб по датасету
./testgen-bench \
  --agent ./testgen-agent --dataset dataset.yaml \
  --model openai/gpt-4o-mini --runs 3 --seed-base 42 \
  --out ./benchmark-results
```

Между прогонами рабочая копия каждого репозитория принудительно
сбрасывается через `git reset --hard <head> && git clean -fdx`, чтобы
сгенерированные тесты, кэш-файл и артефакты `merger`-а одной
конфигурации не влияли на следующую (см. `internal/benchmark/clone.go`,
`internal/ablation/runner.go`).

Готовый dataset из 8 Go-репозиториев, скрипты orchestration и сырые
JSON-отчёты прогона: [Klagvar/testgen-agent-experiments](https://github.com/Klagvar/testgen-agent-experiments).

## Пример работы

```
📂 Repository: .
🔀 Base branch: origin/main
📝 Changed files: 4

  📄 counter.go
     🔍 Affected functions (3):
        • func (*SafeCounter) Inc()
        • func (*SafeCounter) Add(delta int)
        • func FanOut(items []int, workers int, transform func(...)) []int
     📦 Types: SafeCounter, AtomicCounter
     ⚡ Inc: concurrent (struct field: sync.Mutex)
     ⚡ FanOut: concurrent (channel, WaitGroup, goroutine)
     🎯 Patterns: context, HTTP handler
     🤖 Generating tests via openai/gpt-4o-mini (T=0, seed=42)…
     ✅ Generated (2603 prompt + 2160 completion tokens)
     🔬 Validating… ✅ 17 passed
     💾 Tests saved: counter_test.go
     📈 Diff coverage: 92.3%
     🌿 Branch coverage: 88.9%
     🧬 Mutation score: 85.7% (6/7 detected)
     📐 Naturalness: assertions=2.1/test, names=0.84

  📄 helpers.go
     ✂️  Pruning: removed 3 failing sub-tests, kept 58 passing
     📈 Diff coverage: 88.0% ✅

═══════════════════════════════════
📊 Total: generated 3, validated 3, cached 1
📈 Avg diff coverage: 90.1% / 80% target
🧬 Mutation score: 85.7%
⏱️  Duration: 2m15s
🗒️  JSON report: testgen-report-2026-05-03-204512.json
```

## Структура проекта

```
testgen-agent/
├── cmd/
│   ├── agent/             # основной CLI: pipeline.go + main.go
│   ├── ablate/            # ablation на одном репо
│   ├── ablate-report/     # агрегатор ablation-отчётов
│   ├── benchmark/         # multi-repo orchestrator
│   └── benchmark-report/  # агрегатор по репозиториям
├── internal/
│   ├── diff/              # git diff parser
│   ├── gitdiff/           # per-function git compare
│   ├── typed/             # type-checked package cache (go/types)
│   ├── analyzer/          # AST: функции, типы, generics, concurrency
│   ├── patterns/          # AST-детектор Go-идиом
│   ├── prompt/            # сборка промптов + token budget
│   ├── llm/               # OpenAI-совместимый HTTP client
│   ├── validator/         # build + test + race
│   ├── testjson/          # парсер go test -json
│   ├── pruner/            # AST-удаление падающих тестов
│   ├── merger/            # AST-слияние с существующим тест-файлом
│   ├── dedup/             # AST-дедупликация
│   ├── mockgen/           # генерация моков
│   ├── coverage/          # diff coverage
│   ├── branchcov/         # branch + error-path coverage
│   ├── mutation/          # mutation testing
│   ├── naturalness/       # naturalness suite
│   ├── cache/             # SHA-256 функциональный кэш
│   ├── config/            # .testgen.yml
│   ├── report/            # JSON / HTML / агрегатор
│   ├── github/            # PR commenter
│   ├── ablation/          # ablation-конфиги + raннер с --runs
│   └── benchmark/         # dataset loader + clone manager + runner
├── testdata/sample-project/
├── .github/workflows/testgen.yml
├── action.yml             # reusable GitHub Action
├── Dockerfile
├── .testgen.yml
├── .env.example
└── go.mod
```

## Требования

- Go 1.26+
- Git
- `goimports` (`go install golang.org/x/tools/cmd/goimports@latest`)
- LLM API (OpenAI, Ollama, OpenRouter, Groq, vLLM или любой
  OpenAI-совместимый эндпоинт)
- GCC (для `go test -race` на Windows — race detector требует CGO)

## Лицензия

MIT
