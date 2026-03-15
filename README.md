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
                    Mock generator                       Coverage analysis
                                                         Mutation testing
```

## Модули

| Модуль | Описание | Юнит-тесты |
|--------|----------|------------|
| `internal/diff` | Парсинг `git diff` — файлы, хунки, строки | 5 |
| `internal/analyzer` | AST-анализ: функции, типы, интерфейсы, ресиверы, кросс-файловые вызовы, concurrency-детекция | 23 |
| `internal/prompt` | Построение промптов с контекстом: типы, моки, concurrency hints, custom prompt | 9 |
| `internal/llm` | Клиент OpenAI-совместимых API (OpenAI, Ollama, OpenRouter, Groq) | 5 |
| `internal/validator` | Валидация: `goimports` + `go build` + `go test` + `go test -race` | 20 |
| `internal/coverage` | Парсинг `cover.out`, расчёт diff coverage, итеративная догенерация | 12 |
| `internal/pruner` | Парсинг `go test -v` вывода, AST-удаление падающих тестов | 8 |
| `internal/merger` | AST-слияние новых тестов с существующими без перезаписи | 6 |
| `internal/mockgen` | Детерминистическая генерация моков для Go-интерфейсов | 5 |
| `internal/mutation` | Мутационное тестирование: подмена операторов, расчёт mutation score | 6 |
| `internal/dedup` | AST-дедупликация table-driven тест-кейсов | 7 |
| `internal/cache` | Кэш: хэш (сигнатура + тело) → пропуск LLM для неизменённых функций | 10 |
| `internal/gitdiff` | Git-based сравнение: AST-нормализация тел функций vs base branch | 12 |
| `internal/config` | Конфиг `.testgen.yml`: модель, threshold, exclude, custom prompt | 11 |
| `internal/report` | HTML-дашборд: coverage bar chart, mutation score, статистика | 7 |
| `internal/github` | PR-комментарий через GitHub API с отчётом | 4 |
| `cmd/agent` | CLI-оркестратор всего пайплайна | — |

**Итого: 150 юнит-тестов, 17 модулей.**

## Ключевые фичи

- **Diff-ориентированный подход** — анализируются только изменённые функции
- **AST-анализ** — сигнатуры, ветвления, типы/интерфейсы, кросс-файловые зависимости
- **Модель-агностичность** — любой OpenAI-совместимый API (облачный или локальный)
- **Self-healing** — retry с фидбеком LLM + pruning падающих тестов
- **AST Merge** — новые тесты вливаются в существующие без перезаписи
- **Diff Coverage** — итеративная догенерация тестов для непокрытых строк
- **Concurrency-aware** — детекция goroutines/mutex/channels, генерация concurrent-тестов, `-race` flag
- **Mutation testing** — подмена операторов для оценки качества тестов
- **Git-based skip** — сравнение AST с base branch, пропуск неизменённых функций
- **Конфиг-файл** — `.testgen.yml` для настроек на уровне проекта
- **HTML-дашборд** — визуальный отчёт с coverage bar chart
- **PR-комментарий** — автоматический отчёт в Pull Request через GitHub API
- **Дедупликация** — удаление дублирующихся table-driven тест-кейсов

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
  --api-url http://localhost:11434/v1 --model qwen3-coder:30b

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
model: qwen3-coder:30b
api_url: http://localhost:11434/v1
coverage_threshold: 80
max_retries: 3

exclude:
  - "vendor/**"
  - "generated/**"

custom_prompt: |
  Always use table-driven tests.
  Do not use external libraries.

race_detection: true
mutation: false
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
            --model qwen3-coder:30b \
            --mutation --report html \
            --github-token ${{ github.token }} \
            --github-repo ${{ github.repository }} \
            --pr-number ${{ github.event.pull_request.number }}

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
     🔍 Affected functions (8):
        • func (*SafeCounter) Inc()
        • func (*SafeCounter) Add(delta int)
        • func FanOut(items []int, workers int, transform func(...)) []int
     📦 Types: SafeCounter, AtomicCounter
     🔗 Dependencies: Add
     ⚡ Inc: concurrent (struct field: sync.Mutex)
     ⚡ FanOut: concurrent (channel, WaitGroup, goroutine)
     🤖 Generating tests via qwen3-coder:30b...
     ✅ Generated (2603 prompt + 2160 completion tokens)
     🔬 Validating... ✅ 17 passed
     💾 Tests saved: counter_test.go
     📈 Diff coverage: 58.6%

  📄 helpers.go
     ✂️ Pruning: removed 3 failing sub-tests, kept 58 passing
     📈 Diff coverage: 68.8% ✅

  📄 types.go
     🧹 Dedup: removed 1 duplicate case
     ✅ 75 passed

═══════════════════════════════════
📊 Total: generated 3, validated 3
```

## Структура проекта

```
testgen-agent/
├── cmd/agent/main.go                # CLI-оркестратор
├── internal/
│   ├── diff/parser.go               # Парсер git diff
│   ├── analyzer/
│   │   ├── analyzer.go              # AST: функции, типы, кросс-файл
│   │   └── concurrency.go           # Детекция concurrency-паттернов
│   ├── prompt/builder.go            # Конструктор промптов
│   ├── llm/client.go                # OpenAI-совместимый клиент
│   ├── validator/validator.go       # Валидатор (build + test + race)
│   ├── coverage/coverage.go         # Diff coverage
│   ├── pruner/pruner.go             # Удаление падающих тестов
│   ├── merger/merger.go             # AST-слияние тестов
│   ├── mockgen/mockgen.go           # Генерация моков
│   ├── mutation/mutation.go         # Мутационное тестирование
│   ├── dedup/dedup.go               # Дедупликация тест-кейсов
│   ├── cache/cache.go               # Кэширование результатов
│   ├── gitdiff/compare.go           # Git-based сравнение функций
│   ├── config/config.go             # Конфиг .testgen.yml
│   ├── report/html.go               # HTML-дашборд
│   └── github/commenter.go          # PR-комментарий
├── testdata/sample-project/         # Пример проекта
├── .github/workflows/testgen.yml    # GitHub Actions workflow
├── action.yml                       # Reusable GitHub Action
├── Dockerfile                       # Docker-образ
└── go.mod
```

## Требования

- Go 1.21+
- Git
- `goimports` (`go install golang.org/x/tools/cmd/goimports@latest`)
- LLM API (OpenAI, Ollama, OpenRouter, Groq, или любой OpenAI-совместимый)

## Лицензия

MIT
