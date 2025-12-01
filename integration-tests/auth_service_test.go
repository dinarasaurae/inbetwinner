package integration_tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dinarasaurae/inbetwin-integration-tests/testhelpers"
)

func TestAuthServiceIntegration(t *testing.T) {
	cfg := testhelpers.GetTestConfig()
	client := testhelpers.HTTPTestClient()
	db := testhelpers.SetupPostgresDB(t, cfg)
	defer db.Close()

	testhelpers.CleanupDB(t, db)
	defer testhelpers.CleanupDB(t, db)

	testhelpers.WaitForService(t, cfg.AuthServiceURL, 30*time.Second)

	t.Run("HealthCheck", func(t *testing.T) {
		testAuthServiceHealthCheck(t, client, cfg)
	})

	t.Run("UserRegistration", func(t *testing.T) {
		testUserRegistration(t, client, cfg, db)
	})

	t.Run("UserLogin", func(t *testing.T) {
		testUserLogin(t, client, cfg, db)
	})

	t.Run("TokenRefresh", func(t *testing.T) {
		testTokenRefresh(t, client, cfg, db)
	})

	t.Run("UserProfile", func(t *testing.T) {
		testUserProfile(t, client, cfg, db)
	})
}

func testAuthServiceHealthCheck(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig) {
	resp, err := client.Get(cfg.AuthServiceURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.Equal(t, "ok", health["status"])
	assert.Equal(t, "auth-service", health["service"])
	assert.Equal(t, "1.0.0", health["version"])
	assert.Contains(t, health, "time")
}

func testUserRegistration(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig, db *sql.DB) {
	testEmail := "test.integration@example.com"

	_, _ = db.Exec("DELETE FROM users WHERE email = $1", testEmail)

	registerData := map[string]interface{}{
		"email":    testEmail,
		"password": "testpassword123",
		"name":     "Integration Test User",
	}

	jsonData, err := json.Marshal(registerData)
	require.NoError(t, err)

	resp, err := client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/register",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Contains(t, response, "data")

	userData := response["data"].(map[string]interface{})
	assert.Equal(t, testEmail, userData["email"])
	assert.Equal(t, "Integration Test User", userData["name"])

	var userID string
	var email string
	err = db.QueryRow("SELECT id, email FROM users WHERE email = $1", testEmail).Scan(&userID, &email)
	require.NoError(t, err)
	assert.Equal(t, testEmail, email)

	// Тест дублированной регистрации
	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/register",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusConflict, resp.StatusCode)

	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)
	assert.Equal(t, "error", response["status"])
}

func testUserLogin(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig, db *sql.DB) {
	testEmail := "test.login@example.com"
	testPassword := "testpassword123"

	registerData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
		"name":     "Login Test User",
	}

	jsonData, err := json.Marshal(registerData)
	require.NoError(t, err)

	_, _ = db.Exec("DELETE FROM users WHERE email = $1", testEmail)

	resp, err := client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/register",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	resp.Body.Close()

	loginData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
	}

	jsonData, err = json.Marshal(loginData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var response map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Contains(t, response, "access_token")
	assert.Contains(t, response, "refresh_token")
	assert.Contains(t, response, "expires_in")
	assert.Contains(t, response, "user")

	loginData["password"] = "wrongpassword"
	jsonData, err = json.Marshal(loginData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func testTokenRefresh(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig, db *sql.DB) {
	testEmail := "test.refresh@example.com"
	testPassword := "testpassword123"

	_, _ = db.Exec("DELETE FROM users WHERE email = $1", testEmail)

	registerData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
		"name":     "Refresh Test User",
	}

	jsonData, err := json.Marshal(registerData)
	require.NoError(t, err)

	resp, err := client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/register",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	resp.Body.Close()

	loginData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
	}

	jsonData, err = json.Marshal(loginData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	var loginResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&loginResponse)
	require.NoError(t, err)

	refreshToken := loginResponse["refresh_token"].(string)

	refreshData := map[string]interface{}{
		"refresh_token": refreshToken,
	}

	jsonData, err = json.Marshal(refreshData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/refresh",
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
	assert.Contains(t, refreshResponse, "expires_in")

	refreshData["refresh_token"] = "invalid-token"
	jsonData, err = json.Marshal(refreshData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/refresh",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func testUserProfile(t *testing.T, client *http.Client, cfg *testhelpers.TestConfig, db *sql.DB) {
	testEmail := "test.profile@example.com"
	testPassword := "testpassword123"

	_, _ = db.Exec("DELETE FROM users WHERE email = $1", testEmail)

	registerData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
		"name":     "Profile Test User",
	}

	jsonData, err := json.Marshal(registerData)
	require.NoError(t, err)

	resp, err := client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/register",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	resp.Body.Close()

	loginData := map[string]interface{}{
		"email":    testEmail,
		"password": testPassword,
	}

	jsonData, err = json.Marshal(loginData)
	require.NoError(t, err)

	resp, err = client.Post(
		cfg.AuthServiceURL+"/api/v1/auth/login",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	var loginResponse map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&loginResponse)
	require.NoError(t, err)

	accessToken := loginResponse["access_token"].(string)

	req, err := http.NewRequest("GET", cfg.AuthServiceURL+"/api/v1/auth/profile", nil)
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
	assert.Contains(t, profileResponse, "data")

	userData := profileResponse["data"].(map[string]interface{})
	assert.Equal(t, testEmail, userData["email"])
	assert.Equal(t, "Profile Test User", userData["name"])

	req, err = http.NewRequest("GET", cfg.AuthServiceURL+"/api/v1/auth/profile", nil)
	require.NoError(t, err)

	resp, err = client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
