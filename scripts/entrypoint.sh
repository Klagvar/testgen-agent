#!/bin/sh
# Entrypoint-скрипт для Docker-контейнера testgen-agent.
# Используется при запуске в CI/CD (GitHub Actions, GitLab CI и др.)
#
# Переменные окружения:
#   REPO_PATH       — путь к репозиторию (по умолчанию /repo)
#   BASE_BRANCH     — базовая ветка (по умолчанию main)
#   TESTGEN_API_KEY — API-ключ LLM
#   TESTGEN_API_URL — URL API LLM
#   TESTGEN_MODEL   — модель LLM

set -e

REPO_PATH="${REPO_PATH:-/repo}"
BASE_BRANCH="${BASE_BRANCH:-main}"

echo "═══════════════════════════════════"
echo "🤖 testgen-agent — AI Test Generator"
echo "═══════════════════════════════════"
echo ""

# Проверяем что репозиторий существует
if [ ! -d "$REPO_PATH/.git" ]; then
    echo "❌ Ошибка: $REPO_PATH не является git-репозиторием"
    exit 1
fi

# Запускаем агент
exec testgen-agent \
    --repo "$REPO_PATH" \
    --base "$BASE_BRANCH"
