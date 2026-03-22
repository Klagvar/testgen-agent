# testgen-agent

**AI-агент для автоматической генерации юнит-тестов в Go-проектах на основе анализа diff.**

Агент анализирует `git diff`, выполняет AST-анализ изменённого кода, формирует структурированный промпт и генерирует тесты через LLM. Встраивается в CI/CD (GitHub Actions) — тесты генерируются автоматически при открытии Pull Request.

## Архитектура

```
git diff → Diff Parser → AST Analyzer → Prompt Builder → LLM → Validator → _test.go
                              ↓                                      ↓
                    Type/Interface collector              Pruner (remove failing)
                    Cross-file context                   Merger (AST merge)
                    Concurrency detector                 Dedup (remove duplicates)
                    Pattern detector                     Coverage analysis
                    Mock generator                       Mutation testing
                    Token budget manager                 Race detection
```

## Модули

| Модуль | Описание | Тесты |
|--------|----------|:-----:|
| `internal/diff` | Парсинг `git diff` — файлы, хунки, строки | 5 |
| `internal/analyzer` | AST-анализ: функции, типы, интерфейсы, ресиверы, кросс-файловые вызовы, concurrency, generics | 33 |
| `internal/prompt` | Построение промптов: контекст, типы, моки, concurrency hints, token budget, extraction функций по имени | 18 |
| `internal/llm` | Клиент OpenAI-совместимых API (OpenAI, Ollama, OpenRouter, Groq) с retry и backoff | 5 |
| `internal/validator` | Валидация: `goimports` + `go build` + `go test` + `go test -race` (с проверкой CGO) | — |
| `internal/coverage` | Парсинг `cover.out`, расчёт diff coverage, фильтрация неисполняемых строк | 20 |
| `internal/pruner` | Парсинг `go test -v` вывода, AST-удаление падающих тестов и сабтестов | 8 |
| `internal/merger` | AST-слияние новых тестов с существующими без перезаписи | 6 |
| `internal/mockgen` | Детерминистическая генерация моков для Go-интерфейсов | 5 |
| `internal/mutation` | Мутационное тестирование: подмена операторов, расчёт mutation score | 6 |
| `internal/dedup` | AST-дедупликация table-driven тест-кейсов | 7 |
| `internal/cache` | Кэш: хэш (сигнатура + тело) → пропуск LLM для неизменённых функций | 10 |
| `internal/gitdiff` | Git-based сравнение: AST-нормализация тел функций vs base branch | 12 |
| `internal/config` | Конфиг `.testgen.yml`: модель, threshold, exclude/include_only (string или список) | 15 |
| `internal/patterns` | AST-детекция паттернов: HTTP handlers, context, time, env, file I/O, SQL/DB | 18 |
| `internal/report` | HTML-дашборд: coverage bar chart, mutation score, статистика | 7 |
| `internal/github` | PR-комментарии через GitHub API — отдельный отчёт на каждый запуск с привязкой к коммиту | 12 |
| `cmd/agent` | CLI-оркестратор: пайплайн генерации, валидации, coverage loop | — |

**Итого: 187 юнит-тестов, 18 модулей.**

## Ключевые фичи

- **Diff-ориентированный подход** — анализируются только изменённые функции, тесты генерируются только для них
- **AST-анализ** — сигнатуры, ветвления, типы/интерфейсы, кросс-файловые зависимости, generics
- **Модель-агностичность** — любой OpenAI-совместимый API (облачный или локальный)
- **Self-healing** — retry с фидбеком LLM + pruning падающих тестов; при фейле отправляются только упавшие тесты + ошибка
- **AST Merge** — LLM генерирует только новые тесты, которые автоматически вливаются в существующие
- **Diff Coverage** — итеративная догенерация тестов для непокрытых строк с фильтрацией неисполняемых строк (импорты, комментарии)
- **Concurrency-aware** — детекция goroutines/mutex/channels/atomic, генерация concurrent-тестов, `-race` flag
- **Pattern detection** — распознавание HTTP handlers, context.Context, time.Now(), os.Getenv(), file I/O, SQL/DB для подсказок в промпте
- **Token budget** — эвристическая оценка токенов и урезание контекста для вписывания в окно модели
- **Mutation testing** — подмена операторов для оценки качества тестов
- **Git-based skip** — сравнение AST с base branch, пропуск неизменённых функций
- **Кэширование** — хэш сигнатуры + тела функции, пропуск LLM при повторных запусках
- **Конфиг-файл** — `.testgen.yml` для настроек на уровне проекта
- **HTML-дашборд** — визуальный отчёт с coverage bar chart, загружается как артефакт GitHub Actions
- **PR-комментарии** — каждый запуск оставляет отдельный комментарий с привязкой к коммиту (полная история)
- **Дедупликация** — удаление дублирующихся table-driven тест-кейсов
- **Поддержка .env** — автоматическая загрузка переменных окружения из `.env` файла

## Быстрый старт

### Установка

```bash
git clone https://github.com/Klagvar/testgen-agent.git
cd testgen-agent
go build -o testgen-agent ./cmd/agent/
```

### Запуск локально

```bash
# С локальной Ollama
./testgen-agent --repo /path/to/project --base main \
  --api-url http://localhost:11434/v1 --model qwen2.5-coder:32b

# С OpenAI API
./testgen-agent --repo /path/to/project --base main \
  --api-key sk-... --model gpt-4o-mini

# Dry-run (только показать промпт)
./testgen-agent --repo /path/to/project --base main --dry-run

# С мутационным тестированием и HTML-отчётом
./testgen-agent --repo /path/to/project --base main \
  --mutation --report html --race
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

# Можно указать строку или список
include_only: "src/**"

race_detection: true
mutation: true
report_format: html

custom_prompt: |
  Always use table-driven tests.
  Do not use external libraries.
```

### Переменные окружения (`.env`)

```bash
# LLM
TESTGEN_API_KEY=sk-your-key-here
TESTGEN_API_URL=http://localhost:11434/v1
TESTGEN_MODEL=qwen2.5-coder:32b

# GitHub (в GitHub Actions задаются автоматически)
GITHUB_TOKEN=github_pat_...
GITHUB_REPOSITORY=owner/repo
TESTGEN_PR_NUMBER=1
```

### CLI-флаги

| Флаг | Описание | По умолчанию |
|------|----------|--------------|
| `--repo` | Путь к Git-репозиторию | `.` |
| `--base` | Базовая ветка для `git diff` | `main` |
| `--api-key` | API-ключ LLM (или env `TESTGEN_API_KEY`) | — |
| `--api-url` | URL LLM API (или env `TESTGEN_API_URL`) | `https://api.openai.com/v1` |
| `--model` | Модель LLM (или env `TESTGEN_MODEL`) | `gpt-4o-mini` |
| `--dry-run` | Не вызывать LLM, только показать промпт | `false` |
| `--mutation` | Запустить мутационное тестирование | `false` |
| `--race` | Запустить тесты с `-race` для concurrent-функций | `false` |
| `--report html` | Сгенерировать HTML-дашборд | — |
| `--no-cache` | Отключить кэширование | `false` |
| `--no-smart-diff` | Отключить git-based сравнение функций | `false` |
| `--coverage-target` | Целевой порог diff coverage (%) | `80` |
| `--github-token` | Токен для PR-комментария | — |
| `--github-repo` | Репозиторий (owner/repo) | — |
| `--pr-number` | Номер Pull Request | — |

## CI/CD — GitHub Actions

### Self-hosted runner + Ollama (бесплатно)

```yaml
name: Testgen
on:
  pull_request:
    paths: ['**.go', '!**_test.go']

jobs:
  generate-tests:
    runs-on: self-hosted
    steps:
      - uses: actions/checkout@v4
        with:
          ref: ${{ github.head_ref }}
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - run: go build -o testgen-agent ./cmd/agent/
      - run: git fetch origin ${{ github.base_ref }} --depth=1

      - run: |
          ./testgen-agent --repo . --base "origin/${{ github.base_ref }}" \
            --api-url http://localhost:11434/v1 \
            --model qwen2.5-coder:32b \
            --mutation --race --report html \
            --github-token ${{ secrets.GITHUB_TOKEN }} \
            --github-repo ${{ github.repository }} \
            --pr-number ${{ github.event.pull_request.number }}

      - name: Upload HTML report
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: testgen-report
          path: testgen-report.html

      - name: Commit generated tests
        if: success()
        run: |
          git add '**/*_test.go'
          git diff --cached --quiet || git commit -m "🤖 testgen-agent: auto-generated tests"
          git push
```

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
     🔗 Dependencies: Add
     ⚡ Inc: concurrent (struct field: sync.Mutex)
     ⚡ FanOut: concurrent (channel, WaitGroup, goroutine)
     🎯 Patterns: context, HTTP handler
     🤖 Generating tests via qwen2.5-coder:32b...
     ✅ Generated (2603 prompt + 2160 completion tokens)
     🔬 Validating... ✅ 17 passed
     💾 Tests saved: counter_test.go
     📈 Diff coverage: 92.3%
     🧬 Mutation score: 85.7% (6/7 killed)

  📄 helpers.go
     ✂️ Pruning: removed 3 failing sub-tests, kept 58 passing
     📈 Diff coverage: 88.0% ✅

═══════════════════════════════════
📊 Total: generated 3, validated 3, cached 1
📈 Avg diff coverage: 90.1% / 80% target
🧬 Mutation score: 85.7%
⏱️ Duration: 2m15s
```

## Структура проекта

```
testgen-agent/
├── cmd/agent/
│   ├── main.go                      # CLI-оркестратор
│   └── pipeline.go                  # Пайплайн генерации per-file
├── internal/
│   ├── diff/parser.go               # Парсер git diff
│   ├── analyzer/
│   │   ├── analyzer.go              # AST: функции, типы, кросс-файл, generics
│   │   └── concurrency.go           # Детекция concurrency-паттернов
│   ├── prompt/
│   │   ├── builder.go               # Конструктор промптов + extraction функций
│   │   └── tokenizer.go             # Token budget manager
│   ├── patterns/detector.go         # AST-детекция паттернов (HTTP, context, time...)
│   ├── llm/client.go                # OpenAI-совместимый клиент с retry
│   ├── validator/validator.go       # Валидатор (build + test + race)
│   ├── coverage/
│   │   ├── coverage.go              # Diff coverage
│   │   └── filter.go                # Фильтрация неисполняемых строк
│   ├── pruner/pruner.go             # Удаление падающих тестов
│   ├── merger/merger.go             # AST-слияние тестов
│   ├── mockgen/mockgen.go           # Генерация моков
│   ├── mutation/mutation.go         # Мутационное тестирование
│   ├── dedup/dedup.go               # Дедупликация тест-кейсов
│   ├── cache/cache.go               # Кэширование результатов
│   ├── gitdiff/compare.go           # Git-based сравнение функций
│   ├── config/config.go             # Конфиг .testgen.yml
│   ├── report/html.go               # HTML-дашборд
│   └── github/commenter.go          # PR-комментарии
├── testdata/sample-project/         # Тестовый проект (generics, concurrency, HTTP, env, time)
├── .github/workflows/testgen.yml    # GitHub Actions workflow
├── .testgen.yml                     # Конфиг проекта
├── .env.example                     # Пример переменных окружения
├── action.yml                       # Reusable GitHub Action
├── Dockerfile                       # Docker-образ
└── go.mod
```

## Требования

- Go 1.22+ (для поддержки generics в тестовых проектах)
- Git
- `goimports` (`go install golang.org/x/tools/cmd/goimports@latest`)
- LLM API (OpenAI, Ollama, OpenRouter, Groq, или любой OpenAI-совместимый)
- GCC (для `go test -race` на Windows — race detector требует CGO)

## Лицензия

MIT
