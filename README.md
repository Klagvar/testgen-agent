# testgen-agent

**AI-агент для автоматической генерации юнит-тестов в Go-проектах.**

Агент анализирует `git diff`, выполняет AST-анализ изменённого кода, формирует структурированный промпт и генерирует тесты через LLM. Встраивается в CI/CD-пайплайн (GitHub Actions) — тесты генерируются автоматически при открытии Pull Request.

## Архитектура

```
git diff → Diff Parser → AST Analyzer → Prompt Builder → LLM → Validator → _test.go
```

| Модуль | Описание |
|--------|----------|
| `internal/diff` | Парсинг `git diff` — извлечение изменённых файлов, хунков, строк |
| `internal/analyzer` | AST-анализ Go-кода — сигнатуры функций, параметры, возвраты, тела, ветвления |
| `internal/prompt` | Построение промптов — системный + пользовательский с контекстом функций |
| `internal/llm` | Клиент OpenAI-совместимых API (OpenAI, Ollama, OpenRouter, Groq) |
| `internal/validator` | Валидация тестов — `goimports` + `go build` + `go test`, retry при ошибках |
| `cmd/agent` | CLI-точка входа, оркестрация всего пайплайна |

## Ключевые особенности

- **Diff-ориентированный подход** — анализируются только изменённые функции, а не весь проект
- **AST-анализ** — глубокий структурный анализ Go-кода (сигнатуры, ветвления, типы), а не просто текст
- **Модель-агностичность** — работает с любым OpenAI-совместимым API (облачным или локальным)
- **Self-healing** — автоматическое исправление ошибок компиляции/тестов через retry с фидбеком LLM
- **CI/CD интеграция** — готовый GitHub Actions workflow, автокоммит тестов в PR

## Быстрый старт

### Установка

```bash
git clone https://github.com/Klagvar/testgen-agent.git
cd testgen-agent
go build -o testgen-agent ./cmd/agent/
```

### Запуск локально

```bash
# С OpenAI API
./testgen-agent --repo /path/to/project --base main \
  --api-key sk-... --model gpt-4o-mini

# С локальной Ollama
./testgen-agent --repo /path/to/project --base main \
  --api-url http://localhost:11434/v1 --model qwen2.5-coder:14b

# Dry-run (только показать промпт, без вызова LLM)
./testgen-agent --repo /path/to/project --base main --dry-run
```

### CLI-флаги

| Флаг | Описание | По умолчанию |
|------|----------|--------------|
| `--repo` | Путь к Git-репозиторию | `.` |
| `--base` | Базовая ветка для `git diff` | `main` |
| `--api-key` | API-ключ LLM (или env `TESTGEN_API_KEY`) | — |
| `--api-url` | URL LLM API (или env `TESTGEN_API_URL`) | `https://api.openai.com/v1` |
| `--model` | Модель LLM (или env `TESTGEN_MODEL`) | `gpt-4o-mini` |
| `--out` | Директория для тестов | рядом с файлом |
| `--dry-run` | Не вызывать LLM, только показать промпт | `false` |
| `--no-validate` | Не запускать валидацию тестов | `false` |

## CI/CD — GitHub Actions

### Вариант 1: Self-hosted runner + Ollama (бесплатно)

Подходит для локальной разработки — runner запущен на машине с Ollama.

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
            --model qwen2.5-coder:14b
```

### Вариант 2: Облачный runner + OpenAI API

```yaml
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
          go-version: '1.23'

      - run: go build -o testgen-agent ./cmd/agent/
      - run: git fetch origin ${{ github.base_ref }} --depth=1

      - run: |
          ./testgen-agent --repo . --base "origin/${{ github.base_ref }}" \
            --api-key ${{ secrets.OPENAI_API_KEY }} \
            --model gpt-4o-mini
```

## Пример работы

При PR с добавлением функции `Modulo` в `calc.go`:

```
📂 Репозиторий: .
🔀 Base branch: origin/main

📝 Изменено файлов: 1

  📄 testdata/sample-project/calc.go
     Хунков: 1, Изменённых строк: 16
     🔍 Затронутые функции (2):
        • func Modulo(a int, b int) (int, error)  (строки 44–49)
        • func Abs(x int) int  (строки 52–57)
     🤖 Генерация тестов через gpt-4o-mini...
     ✅ Сгенерировано (токенов: 1887 prompt + 734 completion)
     🔬 Валидация...
     ✅ Все тесты прошли (10 passed, 1.2s)

═══════════════════════════════════
📊 Итого: сгенерировано 1, валидировано 1
```

## Структура проекта

```
testgen-agent/
├── cmd/agent/main.go              # CLI-точка входа
├── internal/
│   ├── diff/parser.go             # Парсер git diff
│   ├── analyzer/analyzer.go       # AST-анализатор Go-кода
│   ├── prompt/builder.go          # Конструктор промптов для LLM
│   ├── llm/client.go              # OpenAI-совместимый клиент
│   └── validator/validator.go     # Валидатор тестов (build + test)
├── testdata/sample-project/       # Пример проекта для тестирования
├── .github/workflows/testgen.yml  # GitHub Actions workflow
├── action.yml                     # Reusable GitHub Action
├── Dockerfile                     # Docker-образ агента
└── go.mod
```

## Требования

- Go 1.21+
- Git
- `goimports` (`go install golang.org/x/tools/cmd/goimports@latest`)
- Доступ к LLM API (OpenAI, Ollama, OpenRouter, или любой OpenAI-совместимый)

## Лицензия

MIT
