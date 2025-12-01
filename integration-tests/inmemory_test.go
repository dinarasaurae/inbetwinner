package integration_tests

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	_ "github.com/mattn/go-sqlite3"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type InMemoryTestSuite struct {
	redisServer    *miniredis.Miniredis
	redisClient    *redis.Client
	sqliteDB       *sql.DB
	mockAPIServer  *httptest.Server
	mockAuthServer *httptest.Server
}

func setupInMemoryEnvironment(t *testing.T) *InMemoryTestSuite {
	suite := &InMemoryTestSuite{}

	redisServer := miniredis.RunT(t)
	suite.redisServer = redisServer

	suite.redisClient = redis.NewClient(&redis.Options{
		Addr: redisServer.Addr(),
	})

	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err)
	suite.sqliteDB = db

	_, err = db.Exec(`
		CREATE TABLE users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			name TEXT,
			subscription_plan TEXT DEFAULT 'free',
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)
	`)
	require.NoError(t, err)

	// Create refresh_tokens table
	_, err = db.Exec(`
		CREATE TABLE refresh_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			token_hash TEXT NOT NULL,
			expires_at TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users (id)
		)
	`)
	require.NoError(t, err)

	suite.mockAuthServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{
				"status":   "ok",
				"database": "connected",
				"service":  "auth-service",
			})

		case r.URL.Path == "/auth/register" && r.Method == "POST":
			var req map[string]interface{}
			json.NewDecoder(r.Body).Decode(&req)

			email := req["email"].(string)
			name := req["name"].(string)

			// Insert into SQLite
			userID := fmt.Sprintf("user-%d", time.Now().Unix())
			_, err := suite.sqliteDB.Exec(
				"INSERT INTO users (id, email, password_hash, name) VALUES (?, ?, ?, ?)",
				userID, email, "hashed_password", name,
			)

			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{"error": "User already exists"})
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"id":    userID,
					"email": email,
					"name":  name,
				},
			})

		case r.URL.Path == "/auth/login" && r.Method == "POST":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"access_token":  "mock.jwt.token",
					"refresh_token": "mock.refresh.token",
				},
			})

		case r.URL.Path == "/auth/me" && r.Method == "GET":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer mock.jwt.token" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": "success",
				"data": map[string]interface{}{
					"id":    "user-123",
					"email": "test@example.com",
					"name":  "Test User",
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Mock API Gateway
	suite.mockAPIServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple rate limiting using in-memory Redis
		// Use X-Forwarded-For for rate limiting to simulate real scenario
		clientIP := r.Header.Get("X-Forwarded-For")
		if clientIP == "" {
			clientIP = r.RemoteAddr
		}
		key := fmt.Sprintf("rate_limit:%s", clientIP)

		ctx := context.Background()
		current, err := suite.redisClient.Incr(ctx, key).Result()
		if err == nil {
			if current == 1 {
				suite.redisClient.Expire(ctx, key, 10*time.Second)
			}
			if current > 10 { // Rate limit: 10 requests per 10 seconds
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]string{"error": "Too many requests"})
				return
			}
		}

		switch {
		case r.URL.Path == "/health":
			redisStatus := "connected"
			if suite.redisClient.Ping(ctx).Err() != nil {
				redisStatus = "disconnected"
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "ok",
				"redis":   redisStatus,
				"service": "api-gateway",
			})

		case strings.HasPrefix(r.URL.Path, "/api/v1/auth/"):
			// Proxy to mock auth service
			authPath := strings.TrimPrefix(r.URL.Path, "/api/v1")
			proxyURL := suite.mockAuthServer.URL + authPath

			req, _ := http.NewRequest(r.Method, proxyURL, r.Body)
			for k, v := range r.Header {
				req.Header[k] = v
			}

			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			defer resp.Body.Close()

			w.WriteHeader(resp.StatusCode)
			for k, v := range resp.Header {
				w.Header()[k] = v
			}

			var body interface{}
			json.NewDecoder(resp.Body).Decode(&body)
			json.NewEncoder(w).Encode(body)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	return suite
}

func (s *InMemoryTestSuite) Cleanup() {
	if s.redisClient != nil {
		s.redisClient.Close()
	}
	if s.redisServer != nil {
		s.redisServer.Close()
	}
	if s.sqliteDB != nil {
		s.sqliteDB.Close()
	}
	if s.mockAPIServer != nil {
		s.mockAPIServer.Close()
	}
	if s.mockAuthServer != nil {
		s.mockAuthServer.Close()
	}
}

func TestInMemoryEnvironment(t *testing.T) {
	suite := setupInMemoryEnvironment(t)
	defer suite.Cleanup()

	client := &http.Client{Timeout: 5 * time.Second}

	t.Run("HealthChecks", func(t *testing.T) {
		// Test API Gateway health
		resp, err := client.Get(suite.mockAPIServer.URL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var healthResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&healthResp)
		assert.Equal(t, "ok", healthResp["status"])
		assert.Equal(t, "connected", healthResp["redis"])

		resp, err = client.Get(suite.mockAuthServer.URL + "/health")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("UserRegistrationFlow", func(t *testing.T) {
		reqBody := `{"email":"inmemory.test@example.com","password":"testpass","name":"InMemory User"}`
		resp, err := client.Post(suite.mockAPIServer.URL+"/api/v1/auth/register",
			"application/json", strings.NewReader(reqBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var count int
		err = suite.sqliteDB.QueryRow("SELECT COUNT(*) FROM users WHERE email = ?",
			"inmemory.test@example.com").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)

		var userName string
		err = suite.sqliteDB.QueryRow("SELECT name FROM users WHERE email = ?",
			"inmemory.test@example.com").Scan(&userName)
		require.NoError(t, err)
		assert.Equal(t, "InMemory User", userName)
	})

	t.Run("LoginAndAuthentication", func(t *testing.T) {
		loginBody := `{"email":"test@example.com","password":"testpass"}`
		resp, err := client.Post(suite.mockAPIServer.URL+"/api/v1/auth/login",
			"application/json", strings.NewReader(loginBody))
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var loginResp map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&loginResp)
		data := loginResp["data"].(map[string]interface{})
		accessToken := data["access_token"].(string)
		assert.Equal(t, "mock.jwt.token", accessToken)

		req, err := http.NewRequest("GET", suite.mockAPIServer.URL+"/api/v1/auth/me", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("RateLimitingWithRedis", func(t *testing.T) {
		testClient := &http.Client{
			Transport: &http.Transport{},
			Timeout:   5 * time.Second,
		}

		var lastStatusCode int
		for i := 0; i < 15; i++ {
			req, err := http.NewRequest("GET", suite.mockAPIServer.URL+"/health", nil)
			require.NoError(t, err)
			req.Header.Set("X-Forwarded-For", "192.168.1.100")

			resp, err := testClient.Do(req)
			require.NoError(t, err)
			lastStatusCode = resp.StatusCode
			resp.Body.Close()

			if lastStatusCode == http.StatusTooManyRequests {
				break
			}
		}

		assert.Equal(t, http.StatusTooManyRequests, lastStatusCode)
		ctx := context.Background()
		key := "rate_limit:192.168.1.100"
		count, err := suite.redisClient.Get(ctx, key).Int()
		require.NoError(t, err)
		assert.True(t, count > 10, "Rate limit count should exceed 10")
	})

	t.Run("DatabaseTransactions", func(t *testing.T) {
		tx, err := suite.sqliteDB.Begin()
		require.NoError(t, err)

		userID := fmt.Sprintf("tx-user-%d", time.Now().Unix())
		_, err = tx.Exec("INSERT INTO users (id, email, password_hash, name) VALUES (?, ?, ?, ?)",
			userID, "tx.test@example.com", "hash", "Transaction User")
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)
		var count int
		err = suite.sqliteDB.QueryRow("SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("RedisDataTypes", func(t *testing.T) {
		ctx := context.Background()
		err := suite.redisClient.Set(ctx, "test:string", "value", time.Minute).Err()
		require.NoError(t, err)

		val, err := suite.redisClient.Get(ctx, "test:string").Result()
		require.NoError(t, err)
		assert.Equal(t, "value", val)

		err = suite.redisClient.HSet(ctx, "test:hash", "field1", "value1", "field2", "value2").Err()
		require.NoError(t, err)

		hashVal, err := suite.redisClient.HGet(ctx, "test:hash", "field1").Result()
		require.NoError(t, err)
		assert.Equal(t, "value1", hashVal)

		err = suite.redisClient.LPush(ctx, "test:list", "item1", "item2").Err()
		require.NoError(t, err)

		listLen, err := suite.redisClient.LLen(ctx, "test:list").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(2), listLen)

		err = suite.redisClient.SAdd(ctx, "test:set", "member1", "member2").Err()
		require.NoError(t, err)

		isMember, err := suite.redisClient.SIsMember(ctx, "test:set", "member1").Result()
		require.NoError(t, err)
		assert.True(t, isMember)
	})

	t.Run("ErrorHandling", func(t *testing.T) {
		// Test unauthorized access (most important error handling)
		req, err := http.NewRequest("GET", suite.mockAPIServer.URL+"/api/v1/auth/me", nil)
		require.NoError(t, err)
		// No Authorization header

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		// Test with invalid JWT
		req, err = http.NewRequest("GET", suite.mockAPIServer.URL+"/api/v1/auth/me", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer invalid.token")

		resp, err = client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

		// Test 404 for unknown endpoint
		resp, err = client.Get(suite.mockAPIServer.URL + "/unknown/endpoint")
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
