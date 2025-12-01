package integration_tests

import (
	"bytes"
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

func TestAPIGatewayIntegration(t *testing.T) {
	cfg := testhelpers.GetTestConfig()
	client := testhelpers.HTTPTestClient()

	testhelpers.WaitForService(t, cfg.APIGatewayURL, 30*time.Second)
	testhelpers.WaitForService(t, cfg.AuthServiceURL, 30*time.Second)

	t.Run("HealthCheck", func(t *testing.T) {
		testAPIGatewayHealthCheck(t, client, cfg)
	})

	t.Run("AuthServiceProxy", func(t *testing.T) {
		testAuthServiceProxy(t, client, cfg)
	})

	t.Run("JWTMiddleware", func(t *testing.T) {
		testJWTMiddleware(t, client, cfg)
	})

	t.Run("RateLimiting", func(t *testing.T) {
		testRateLimiting(t, client, cfg)
	})

	t.Run("ProtectedRouteAccess", func(t *testing.T) {
		testProtectedRouteAccess(t, client, cfg)
	})
}

func testAPIGatewayHealthCheck(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	resp, err := client.Get(cfg.APIGatewayURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.Equal(t, "ok", health["status"])
	assert.Equal(t, "api-gateway", health["service"])
	assert.Equal(t, "connected", health["redis"])
}

func testAuthServiceProxy(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	registerData := map[string]interface{}{
		"email":    "test.proxy@example.com",
		"password": "testpassword123",
		"name":     "Test User",
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

	assert.True(t, resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusConflict)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	if resp.StatusCode == http.StatusCreated {
		assert.Equal(t, "success", response["status"])
		assert.Contains(t, response, "data")
	} else {
		assert.Equal(t, "error", response["status"])
	}
}

func testJWTMiddleware(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	resp, err := client.Get(cfg.APIGatewayURL + "/api/v1/social/test")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	var errorResp map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&errorResp)
	require.NoError(t, err)
	assert.Contains(t, errorResp["error"], "Authorization header missing")

	req, err := http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/test", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer invalid-token")

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func testRateLimiting(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	rateLimitClient := &http.Client{Timeout: 10 * time.Second}

	time.Sleep(11 * time.Second)
	var lastStatusCode int
	for i := 0; i < 60; i++ {
		resp, err := rateLimitClient.Get(cfg.APIGatewayURL + "/api/v1/auth/me")
		require.NoError(t, err)
		lastStatusCode = resp.StatusCode
		resp.Body.Close()

		if lastStatusCode == http.StatusTooManyRequests {
			break
		}
	}
	assert.Equal(t, http.StatusTooManyRequests, lastStatusCode)

	time.Sleep(11 * time.Second)
}

func testProtectedRouteAccess(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	jwtService := jwtlib.NewService("your-super-secret-key-change-this-in-production", time.Hour, 24*time.Hour)

	userID := uuid.New()
	token, err := jwtService.GenerateAccessToken(userID, "test@example.com", "premium")
	require.NoError(t, err)

	req, err := http.NewRequest("GET", cfg.APIGatewayURL+"/api/v1/social/", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var mockResp map[string]interface{}
	err = json.Unmarshal(body, &mockResp)
	require.NoError(t, err)
	assert.Equal(t, "social", mockResp["service"])
	assert.Equal(t, "ok", mockResp["status"])
}
