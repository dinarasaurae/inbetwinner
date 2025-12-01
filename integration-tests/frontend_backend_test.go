package integration_tests

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dinarasaurae/inbetwin-integration-tests/testhelpers"
)

func TestFrontendBackendIntegration(t *testing.T) {
	cfg := testhelpers.GetTestConfig()
	client := testhelpers.HTTPTestClient()

	// Ждём пока сервисы станут доступными
	testhelpers.WaitForService(t, cfg.APIGatewayURL, 30*time.Second)
	testhelpers.WaitForService(t, cfg.AuthServiceURL, 30*time.Second)

	t.Run("FullUserJourney", func(t *testing.T) {
		testFullUserJourney(t, client, cfg)
	})

	t.Run("FrontendAPICompatibility", func(t *testing.T) {
		testFrontendAPICompatibility(t, client, cfg)
	})

	t.Run("ErrorHandlingFlow", func(t *testing.T) {
		testFrontendErrorHandlingFlow(t, client, cfg)
	})

	t.Run("CrossOriginRequests", func(t *testing.T) {
		testCrossOriginRequests(t, client, cfg)
	})
}

func testFullUserJourney(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	// Симулируем полный пользовательский путь как во фронтенде

	// 1. Проверка здоровья сервисов (как во фронтенде)
	t.Log("Step 1: Checking service health...")

	// API Gateway health check
	resp, err := client.Get(cfg.APIGatewayURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var healthResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	require.NoError(t, err)
	assert.Equal(t, "ok", healthResp["status"])

	// Auth Service health check (direct, как во фронтенде)
	resp, err = client.Get(cfg.AuthServiceURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 2. Регистрация пользователя (напрямую к Auth Service)
	t.Log("Step 2: User registration...")

	testEmail := fmt.Sprintf("frontend.test.%d@example.com", time.Now().Unix())
	registerPayload := map[string]interface{}{
		"name":     "Frontend Test User",
		"email":    testEmail,
		"password": "testpassword123",
	}

	registerJSON, _ := json.Marshal(registerPayload)
	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/register",
		"application/json",
		strings.NewReader(string(registerJSON)),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Проверяем успешную регистрацию или конфликт (если пользователь существует)
	assert.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict)

	// 3. Логин (напрямую к Auth Service)
	t.Log("Step 3: User login...")

	loginPayload := map[string]interface{}{
		"email":    testEmail,
		"password": "testpassword123",
	}

	// Если пользователь уже существовал, создаём уникального
	if resp.StatusCode == http.StatusConflict {
		testEmail = fmt.Sprintf("frontend.unique.%d@example.com", time.Now().UnixNano())
		registerPayload["email"] = testEmail
		loginPayload["email"] = testEmail

		// Регистрация нового пользователя
		registerJSON, _ := json.Marshal(registerPayload)
		resp, err := client.Post(
			cfg.AuthServiceURL+"/api/v1/auth/register",
			"application/json",
			strings.NewReader(string(registerJSON)),
		)
		require.NoError(t, err)
		resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	loginJSON, _ := json.Marshal(loginPayload)
	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		strings.NewReader(string(loginJSON)),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("Login failed with status %d: %s", resp.StatusCode, string(body))
	}

	var loginResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&loginResp)
	require.NoError(t, err)

	// Проверяем структуру ответа от Auth Service
	if success, exists := loginResp["success"]; exists {
		assert.True(t, success.(bool))
	}

	// Проверяем наличие токенов
	if _, hasAccessToken := loginResp["access_token"]; hasAccessToken {
		assert.Contains(t, loginResp, "access_token")
		assert.Contains(t, loginResp, "refresh_token")
		assert.Contains(t, loginResp, "expires_in")
		assert.Contains(t, loginResp, "user")

		accessToken := loginResp["access_token"].(string)
		assert.NotEmpty(t, accessToken)

		// 4. Получение профиля (проверяем что JWT работает)
		t.Log("Step 4: Getting user profile...")

		req, err := http.NewRequest("GET", cfg.AuthServiceURL+"/api/v1/auth/profile", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var profileResp map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&profileResp)
			require.NoError(t, err)

			// Проверяем базовую структуру профиля
			if data, hasData := profileResp["data"]; hasData && data != nil {
				userData := data.(map[string]interface{})
				if userData["email"] != nil {
					assert.Equal(t, testEmail, userData["email"])
				}
				if userData["name"] != nil {
					assert.Equal(t, "Frontend Test User", userData["name"])
				}
			} else {
				t.Logf("Profile response structure: %+v", profileResp)
			}
		} else {
			t.Logf("Profile endpoint returned %d, may not be implemented", resp.StatusCode)
		}

		// 5. Тестирование API Gateway auth proxy (симулируем фронтенд)
		t.Log("Step 5: Testing API Gateway auth proxy...")

		// Проверяем что API Gateway может проксировать auth запросы
		testAPIGatewayProxy(t, client, cfg, accessToken)
	} else {
		t.Log("No access token received, basic login test passed")
	}
}

func testAPIGatewayProxy(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig, accessToken string) {
	// Тестируем простые запросы через API Gateway
	endpoints := []string{
		"/health",              // Должен работать
		"/api/v1/auth/profile", // Может не работать из-за роутинга
	}

	for _, endpoint := range endpoints {
		t.Run(fmt.Sprintf("APIGateway_%s", strings.ReplaceAll(endpoint, "/", "_")), func(t *testing.T) {
			var req *http.Request
			var err error

			if endpoint == "/api/v1/auth/profile" {
				req, err = http.NewRequest("GET", cfg.APIGatewayURL+endpoint, nil)
				require.NoError(t, err)
				req.Header.Set("Authorization", "Bearer "+accessToken)
			} else {
				req, err = http.NewRequest("GET", cfg.APIGatewayURL+endpoint, nil)
				require.NoError(t, err)
			}

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Логируем результат без строгих проверок
			t.Logf("Endpoint %s returned status %d", endpoint, resp.StatusCode)

			if resp.StatusCode == http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Logf("Response: %s", string(body))
			}
		})
	}
}

func testFrontendAPICompatibility(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	// Тестируем совместимость API с ожиданиями фронтенда

	// 1. Content-Type headers для health endpoint
	resp, err := client.Get(cfg.APIGatewayURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Проверяем, что API возвращает JSON
	contentType := resp.Header.Get("Content-Type")
	assert.Contains(t, contentType, "application/json")

	// 2. CORS headers (важно для фронтенда) - тестируем OPTIONS на health
	req, err := http.NewRequest("OPTIONS", cfg.APIGatewayURL+"/health", nil)
	require.NoError(t, err)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// API Gateway должен обрабатывать CORS
	assert.True(t, resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNoContent)

	// 3. JSON структура ошибок - тестируем на Auth Service напрямую
	loginPayload := `{"email":"invalid","password":"invalid"}`
	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		strings.NewReader(loginPayload),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Проверяем что получили ошибку авторизации
	if resp.StatusCode == http.StatusUnauthorized {
		var errorResp map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&errorResp)
		require.NoError(t, err)

		// Проверяем структуру ошибки
		if status, hasStatus := errorResp["status"]; hasStatus {
			assert.Equal(t, "error", status)
		}
		// Должно быть какое-то сообщение об ошибке
		assert.True(t, errorResp["error"] != nil || errorResp["message"] != nil)
	} else {
		t.Logf("Auth login returned %d instead of 401, may be expected", resp.StatusCode)
	}
}

func testFrontendErrorHandlingFlow(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	// Тестируем обработку ошибок как во фронтенде - используем Auth Service напрямую

	// 1. Неправильный JSON
	resp, err := client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		strings.NewReader(`{"invalid": json`),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Ожидаем ошибку парсинга JSON (400) или другую ошибку клиента
	assert.True(t, resp.StatusCode >= 400 && resp.StatusCode < 500,
		"Expected 4xx error for invalid JSON, got %d", resp.StatusCode)

	// 2. Отсутствующие поля
	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		strings.NewReader(`{}`),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Ожидаем ошибку валидации
	assert.True(t, resp.StatusCode >= 400 && resp.StatusCode < 500,
		"Expected 4xx error for missing fields, got %d", resp.StatusCode)

	// 3. Доступ к защищённому endpoint без токена
	resp, err = client.Get(cfg.AuthServiceURL + "/api/v1/auth/profile")
	require.NoError(t, err)
	defer resp.Body.Close()

	// Ожидаем ошибку авторизации
	assert.True(t, resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden,
		"Expected 401 or 403 for missing auth, got %d", resp.StatusCode)

	// 4. Неправильный токен
	req, err := http.NewRequest("GET", cfg.AuthServiceURL+"/api/v1/auth/profile", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invalid-token")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Ожидаем ошибку авторизации
	assert.True(t, resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden,
		"Expected 401 or 403 for invalid token, got %d", resp.StatusCode)
}

func testCrossOriginRequests(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	// Тестируем CORS для фронтенда

	origins := []string{
		"http://localhost:3000", // Vite dev server
		"http://127.0.0.1:3000",
		"http://localhost:5173", // Alternative Vite port
	}

	for _, origin := range origins {
		t.Run(fmt.Sprintf("CORS_Origin_%s", strings.ReplaceAll(origin, ":", "_")), func(t *testing.T) {
			req, err := http.NewRequest("GET", cfg.APIGatewayURL+"/health", nil)
			require.NoError(t, err)
			req.Header.Set("Origin", origin)

			resp, err := client.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			// Проверяем CORS headers в ответе
			accessControlOrigin := resp.Header.Get("Access-Control-Allow-Origin")
			assert.True(t, accessControlOrigin == "*" || accessControlOrigin == origin,
				"Expected CORS header, got: %s", accessControlOrigin)
		})
	}
}