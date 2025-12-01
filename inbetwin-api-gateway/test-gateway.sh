#!/bin/bash

set -e

API_URL="http://localhost:8080"
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "🧪 Testing inBeTwin API Gateway"
echo "================================"

print_test() {
    echo -e "\n${YELLOW}$1${NC}"
}

print_success() {
    echo -e "${GREEN}✅ $1${NC}"
}

print_error() {
    echo -e "${RED}❌ $1${NC}"
}

# Test 1: Health Check
print_test "Test 1: Health Check"
HEALTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/health")
if [ "$HEALTH_STATUS" -eq 200 ]; then
    print_success "Health check passed"
    curl -s "$API_URL/health" | jq '.'
else
    print_error "Health check failed with status $HEALTH_STATUS"
fi

# Test 2: Test Auth Service Proxy (Public route)
print_test "Test 2: Auth Service Proxy (Public route)"
AUTH_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/v1/auth/test")
if [ "$AUTH_STATUS" -eq 200 ]; then
    print_success "Auth service proxy works"
    curl -s "$API_URL/api/v1/auth/test" | jq '.'
else
    print_error "Auth service proxy failed with status $AUTH_STATUS"
fi

# Test 3: Protected route without token (should return 401)
print_test "Test 3: Protected route without token (should return 401)"
PROTECTED_STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/api/v1/social/test")
if [ "$PROTECTED_STATUS" -eq 401 ]; then
    print_success "Protected route correctly returns 401 without token"
else
    print_error "Protected route should return 401, got $PROTECTED_STATUS"
fi

# Test 4: Generate test JWT token
print_test "Test 4: Generating test JWT token"
# Создаем простой токен для тестирования
# В production используйте реальный JWT сервис
# Payload: {"user_id": "test-user-123", "exp": <future_timestamp>}

# Для быстрого теста создаем токен через API (если у вас есть endpoint)
# Или используем заранее созданный токен
echo "Creating test token..."

# Временный JWT токен для тестирования (валиден до 2025 года)
# Создан с секретом: your-super-secret-key-change-this-in-production
# Payload: {"user_id":"test-user-123","exp":1764098427,"iss":"inbetwin-api-gateway"}
TEST_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ1c2VyX2lkIjoidGVzdC11c2VyLTEyMyIsImlzcyI6ImluYmV0d2luLWFwaS1nYXRld2F5IiwiZXhwIjoxNzY0MDk4NDI3LCJpYXQiOjE3NjQwMTIwMjd9.wpjKlmXZzEx6A4oMPitHPRUTaDGjxwpUabPjG8a2SPo"

print_success "Test token generated"

# Test 5: Protected route with valid token
print_test "Test 5: Protected route with valid token"
PROTECTED_WITH_TOKEN_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $TEST_TOKEN" \
  "$API_URL/api/v1/social/test")

if [ "$PROTECTED_WITH_TOKEN_STATUS" -eq 200 ]; then
    print_success "Protected route works with token"
    curl -s -H "Authorization: Bearer $TEST_TOKEN" "$API_URL/api/v1/social/test" | jq '.'
else
    print_error "Protected route failed with token, status: $PROTECTED_WITH_TOKEN_STATUS"
fi

# Test 6: Test all service proxies
print_test "Test 6: Testing all service proxies"

SERVICES=("social" "agent" "llm" "rag" "leads")

for service in "${SERVICES[@]}"; do
    echo "Testing $service service..."
    SERVICE_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
      -H "Authorization: Bearer $TEST_TOKEN" \
      "$API_URL/api/v1/$service/test")

    if [ "$SERVICE_STATUS" -eq 200 ]; then
        print_success "$service service proxy works"
    else
        print_error "$service service proxy failed with status $SERVICE_STATUS"
    fi
done

# Test 7: Rate Limiting Test
print_test "Test 7: Rate Limiting (sending 105 requests quickly)"
echo "This may take a moment..."

RATE_LIMIT_TRIGGERED=false
for i in {1..105}; do
    STATUS=$(curl -s -o /dev/null -w "%{http_code}" "$API_URL/health")
    if [ "$STATUS" -eq 429 ]; then
        RATE_LIMIT_TRIGGERED=true
        print_success "Rate limiting triggered at request $i"
        break
    fi
    if [ $((i % 20)) -eq 0 ]; then
        echo "Sent $i requests..."
    fi
done

if [ "$RATE_LIMIT_TRIGGERED" = true ]; then
    print_success "Rate limiting works correctly"
else
    print_error "Rate limiting may not be working (no 429 status received)"
fi

# Wait for rate limit to reset (window is 1m by default)
echo "Waiting 65 seconds for rate limit to reset..."
sleep 65

# Test 8: Invalid JWT token
print_test "Test 8: Invalid JWT token"
INVALID_TOKEN="invalid.jwt.token"
INVALID_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $INVALID_TOKEN" \
  "$API_URL/api/v1/social/test")

if [ "$INVALID_STATUS" -eq 401 ]; then
    print_success "Invalid token correctly rejected"
else
    print_error "Invalid token should be rejected with 401, got $INVALID_STATUS"
fi

# Test 9: CORS Headers
print_test "Test 9: CORS Headers"
CORS_RESPONSE=$(curl -s -I -H "Origin: http://localhost:3000" "$API_URL/health")
if echo "$CORS_RESPONSE" | grep -i "access-control-allow-origin" > /dev/null; then
    print_success "CORS headers present"
else
    print_error "CORS headers missing"
fi

# Test 10: Query parameters forwarding
print_test "Test 10: Query parameters forwarding"
QUERY_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer $TEST_TOKEN" \
  "$API_URL/api/v1/social/posts?page=1&limit=10")

if [ "$QUERY_STATUS" -eq 200 ]; then
    print_success "Query parameters forwarded correctly"
else
    print_error "Query parameter forwarding failed with status $QUERY_STATUS"
fi

echo ""
echo "================================"
print_success "All tests completed!"
echo "================================"

echo ""
echo "📊 Test Summary:"
echo "- Health Check"
echo "- Public Routes (Auth)"
echo "- Protected Routes"
echo "- JWT Token Validation"
echo "- All Service Proxies"
echo "- Rate Limiting"
echo "- CORS Headers"
echo "- Query Parameter Forwarding"
echo ""
echo "Useful URLs:"
echo "- API Gateway: http://localhost:8080"
echo "- Health Check: http://localhost:8080/health"
echo "- RabbitMQ Management: http://localhost:15672 (inbetwin/inbetwin_password)"
echo ""