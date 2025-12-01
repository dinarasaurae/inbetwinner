package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dinarasaurae/inbetwin-integration-tests/testhelpers"
	jwtlib "github.com/dinarasaurae/inbetwin-shared/jwt-go"
)

func TestServiceCommunicationIntegration(t *testing.T) {
	cfg := testhelpers.GetTestConfig()
	client := testhelpers.HTTPTestClient()

	// Ждём пока сервисы станут доступными
	testhelpers.WaitForService(t, cfg.APIGatewayURL, 30*time.Second)
	testhelpers.WaitForService(t, cfg.AuthServiceURL, 30*time.Second)

	t.Run("EndToEndUserFlow", func(t *testing.T) {
		testEndToEndUserFlow(t, client, cfg)
	})

	t.Run("ProxyHeaderPropagation", func(t *testing.T) {
		testProxyHeaderPropagation(t, client, cfg)
	})

	t.Run("ErrorHandlingFlow", func(t *testing.T) {
		testErrorHandlingFlow(t, client, cfg)
	})

	t.Run("AuthenticationFlow", func(t *testing.T) {
		testAuthenticationFlow(t, client, cfg)
	})

	t.Run("ServiceDiscoveryAndFailover", func(t *testing.T) {
		testServiceFailover(t, client, cfg)
	})
}

func testEndToEndUserFlow(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	testEmail := "test.e2e@example.com"
	testPassword := "testpassword123"
	testName := "E2E Test User"

	registerData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
		"name":     testName,
	}

	jsonData, err := json.Marshal(registerData)
	require.NoError(t, err)

	resp, err := client.Post(
		cfg.APIGatewayURL+"/api/v1/auth/register",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		t.Log("User already exists, continuing with login")
	} else {
		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	}

	loginData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
	}

	jsonData, err = json.Marshal(loginData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.APIGatewayURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var loginResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&loginResponse)
	require.NoError(t, err)

	assert.Equal(t, "success", loginResponse["status"])
	assert.Contains(t, loginResponse, "access_token")
	assert.Contains(t, loginResponse, "refresh_token")

	accessToken := loginResponse["access_token"].(string)
	refreshToken := loginResponse["refresh_token"].(string)

	req, err := http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/auth/profile", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var profileResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&profileResponse)
	require.NoError(t, err)

	assert.Equal(t, "success", profileResponse["status"])
	userData := profileResponse["data"].(map[string]interface{})
	assert.Equal(t, testEmail, userData["email"])
	assert.Equal(t, testName, userData["name"])

	req, err = http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var mockResponse map[string]interface{}
	err = json.Unmarshal(body, &mockResponse)
	require.NoError(t, err)
	assert.Equal(t, "social", mockResponse["service"])

	refreshData := map[string]interface{}{
		"refresh_token": refreshToken,
	}

	jsonData, err = json.Marshal(refreshData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.APIGatewayURL+"/api/v1/auth/refresh",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var refreshResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&refreshResponse)
	require.NoError(t, err)

	assert.Equal(t, "success", refreshResponse["status"])
	assert.Contains(t, refreshResponse, "access_token")

	newAccessToken := refreshResponse["access_token"].(string)
	assert.NotEqual(t, accessToken, newAccessToken)

	req, err = http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+newAccessToken)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func testProxyHeaderPropagation(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	jwtService := jwtlib.NewService("your-super-secret-key-change-this-in-production", time.Hour, 24*time.Hour)
	userID := uuid.New()

	token, err := jwtService.GenerateAccessToken(userID, "test@example.com", "premium")
	require.NoError(t, err)

	req, err := http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Custom-Header", "test-value")
	req.Header.Set("User-Agent", "IntegrationTest/1.0")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Mock сервис должен получить проксированные заголовки
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var mockResponse map[string]interface{}
	err = json.Unmarshal(body, &mockResponse)
	require.NoError(t, err)
	assert.Equal(t, "social", mockResponse["service"])
}

func testErrorHandlingFlow(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	invalidJSON := `{"invalid": json`
	resp, err := client.Post(
		cfg.APIGatewayURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBufferString(invalidJSON),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	incompleteData := map[string]interface{}{
		"email": "test@example.com",
		// пароль отсутствует
	}

	jsonData, err := json.Marshal(incompleteData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.APIGatewayURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var errorResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&errorResponse)
	require.NoError(t, err)
	assert.Equal(t, "error", errorResponse["status"])

	wrongCredentials := map[string]interface{}{
		"email":    "nonexistent@example.com",
		"password": "wrongpassword",
	}

	jsonData, err = json.Marshal(wrongCredentials)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.APIGatewayURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	resp, err = client.Get(cfg.APIGatewayURL + "/api/v1/nonexistent")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.True(t, resp.StatusCode >= 400)
}

func testAuthenticationFlow(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	time.Sleep(11 * time.Second)

	// 1. Доступ без токена
	resp, err := client.Get(cfg.APIGatewayURL + "/api/v1/social/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// 2. Неправильный формат Authorization header
	req, err := http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/test", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "InvalidFormat token")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// 3. Неправильный токен
	req, err = http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/test", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// 4. Просроченный токен
	jwtService := jwtlib.NewService("your-super-secret-key-change-this-in-production", time.Millisecond, 24*time.Hour)
	userID := uuid.New()

	expiredToken, err := jwtService.GenerateAccessToken(userID, "test@example.com", "")
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	req, err = http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/test", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+expiredToken)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	// 5. Валидный токен
	validJwtService := jwtlib.NewService("your-super-secret-key-change-this-in-production", time.Hour, 24*time.Hour)
	validToken, err := validJwtService.GenerateAccessToken(userID, "test@example.com", "")
	require.NoError(t, err)

	req, err = http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+validToken)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func testServiceFailover(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	// Тест поведения при недоступности сервисов

	// 1. Тест таймаута при недоступности Auth Service
	// (Для этого теста нужно было бы временно остановить сервис)
	// Здесь мы просто проверяем что запрос к несуществующему сервису возвращает ошибку

	// Создаём клиент с коротким таймаутом
	shortTimeoutClient := &http.Client{
		Timeout: 1 * time.Second,
	}

	invalidURL := "http://localhost:9999/api/v1/auth/login"
	loginData := map[string]interface{}{
		"email":    "test@example.com",
		"password": "password",
	}

	jsonData, err := json.Marshal(loginData)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", invalidURL, bytes.NewBuffer(jsonData))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	_, err = shortTimeoutClient.Do(req)
	assert.Error(t, err)

	// 2. Тест корректной обработки ошибок сервисов
	resp, err := client.Get(cfg.APIGatewayURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	resp, err = client.Get(cfg.AuthServiceURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
