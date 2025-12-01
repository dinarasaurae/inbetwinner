#!/bin/bash

# Performance Benchmark Script для inBeTwin API Gateway
# Тестирует производительность с fasthttp vs net/http

set -e

API_URL="http://localhost:8080"
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "🚀 Performance Benchmark для inBeTwin API Gateway"
echo "================================================="
echo ""

# Проверяем что есть все необходимые инструменты
check_tool() {
    if ! command -v $1 &> /dev/null; then
        echo -e "${RED}❌ $1 не найден. Установите: $2${NC}"
        exit 1
    fi
}

echo "🔍 Проверяем инструменты..."
check_tool "curl" "curl"
check_tool "ab" "apache2-utils (apt install apache2-utils или brew install httpie)"
check_tool "wrk" "wrk (https://github.com/wg/wrk или brew install wrk)"

# Генерируем тестовый JWT токен
echo "🔑 Генерируем JWT токен..."
TOKEN=$(go run generate-test-token.go | grep "Token:" | cut -d' ' -f2)
if [ -z "$TOKEN" ]; then
    echo -e "${RED}❌ Не удалось сгенерировать токен${NC}"
    exit 1
fi
echo -e "${GREEN}✅ Токен сгенерирован${NC}"

# Функция для красивого вывода
print_header() {
    echo ""
    echo -e "${BLUE}════════════════════════════════════════════════${NC}"
    echo -e "${BLUE} $1${NC}"
    echo -e "${BLUE}════════════════════════════════════════════════${NC}"
}

print_test() {
    echo -e "\n${YELLOW}📊 $1${NC}"
}

print_result() {
    echo -e "${GREEN}✅ $1${NC}"
}

# Проверяем что сервер запущен
echo "🔍 Проверяем что API Gateway запущен..."
if ! curl -s -f "$API_URL/health" > /dev/null; then
    echo -e "${RED}❌ API Gateway не доступен на $API_URL${NC}"
    echo "Запустите: docker-compose up -d"
    exit 1
fi
echo -e "${GREEN}✅ API Gateway доступен${NC}"

print_header "BENCHMARK 1: HEALTH CHECK (Простые запросы)"

print_test "Apache Bench - 1000 requests, concurrency 10"
ab -n 1000 -c 10 "$API_URL/health" | grep -E "(Requests per second|Time per request|Connection Times)"

print_test "wrk - 10 seconds, 2 threads, 10 connections"
wrk -t2 -c10 -d10s "$API_URL/health"

print_header "BENCHMARK 2: PUBLIC AUTH PROXY (Proxy без токена)"

print_test "Apache Bench - Auth service proxy"
ab -n 500 -c 10 "$API_URL/api/v1/auth/test" | grep -E "(Requests per second|Time per request|Connection Times)"

print_test "wrk - Auth service proxy"
wrk -t2 -c10 -d10s "$API_URL/api/v1/auth/test"

print_header "BENCHMARK 3: PROTECTED PROXY (С JWT токеном)"

# Создаем временный файл с заголовком
HEADER_FILE=$(mktemp)
echo "Authorization: Bearer $TOKEN" > $HEADER_FILE

print_test "Apache Bench - Protected proxy с JWT"
ab -n 500 -c 10 -H "Authorization: Bearer $TOKEN" "$API_URL/api/v1/social/test" | grep -E "(Requests per second|Time per request|Connection Times)"

print_test "wrk - Protected proxy с JWT"
wrk -t2 -c10 -d10s -H "Authorization: Bearer $TOKEN" "$API_URL/api/v1/social/test"

# Очищаем временный файл
rm -f $HEADER_FILE

print_header "BENCHMARK 4: MIXED LOAD TEST (Реальная нагрузка)"

print_test "Mixed requests - Health + Auth + Protected"
echo "Запускаем параллельно:"
echo "- Health check (легкие запросы)"
echo "- Auth proxy (средние запросы)"
echo "- Protected proxy (тяжелые запросы с JWT)"

# Запускаем 3 теста параллельно
(
    echo "Health check (background):"
    wrk -t1 -c5 -d20s "$API_URL/health" | grep "Requests/sec"
) &

(
    echo "Auth proxy (background):"
    wrk -t1 -c5 -d20s "$API_URL/api/v1/auth/test" | grep "Requests/sec"
) &

(
    echo "Protected proxy (background):"
    wrk -t1 -c5 -d20s -H "Authorization: Bearer $TOKEN" "$API_URL/api/v1/social/test" | grep "Requests/sec"
) &

# Ждем завершения всех тестов
wait

print_header "BENCHMARK 5: STRESS TEST (Высокая нагрузка)"

print_test "High concurrency test - 100 concurrent connections"
wrk -t4 -c100 -d30s "$API_URL/health"

print_test "Burst test - 1000 requests максимально быстро"
ab -n 1000 -c 50 "$API_URL/health" | grep -E "(Requests per second|Time per request|Failed requests)"

print_header "BENCHMARK 6: MEMORY & LATENCY"

print_test "Latency distribution test"
wrk -t2 -c10 -d10s --latency "$API_URL/api/v1/social/test" -H "Authorization: Bearer $TOKEN"

print_header "РЕЗУЛЬТАТЫ"

echo ""
echo -e "${GREEN}🎯 Benchmark завершен!${NC}"
echo ""
echo "📈 Ожидаемая производительность с fasthttp:"
echo "   • Health check: >50,000 req/sec"
echo "   • Proxy requests: >20,000 req/sec"
echo "   • Protected requests: >15,000 req/sec"
echo ""
echo "🔬 Сравнение с net/http:"
echo "   • fasthttp обычно в 2-3x быстрее"
echo "   • Меньше memory allocations"
echo "   • Лучше latency при высокой нагрузке"
echo ""
echo "💡 Для production рекомендуется:"
echo "   • Включить HTTP/2 (в Fiber v3)"
echo "   • Настроить reverse proxy (Nginx/HAProxy)"
echo "   • Мониторинг (Prometheus + Grafana)"
echo ""
echo -e "${BLUE}📊 Проанализируйте результаты выше ☝️${NC}"
echo ""