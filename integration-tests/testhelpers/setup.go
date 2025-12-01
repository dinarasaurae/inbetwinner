package testhelpers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type TestConfig struct {
	APIGatewayURL  string
	AuthServiceURL string
	PostgresURL    string
	RedisURL       string
}

func GetTestConfig() *TestConfig {
	return &TestConfig{
		APIGatewayURL:  getEnvOrDefault("API_GATEWAY_URL", "http://localhost:8080"),
		AuthServiceURL: getEnvOrDefault("AUTH_SERVICE_URL", "http://localhost:3001"),
		PostgresURL:    getEnvOrDefault("POSTGRES_URL", "postgres://auth_user:auth_password@localhost:5432/auth_db?sslmode=disable"),
		RedisURL:       getEnvOrDefault("REDIS_URL", "localhost:6379"),
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// WaitForService ждёт пока сервис станет доступным
func WaitForService(t *testing.T, url string, timeout time.Duration) {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url + "/health")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(1 * time.Second)
	}

	require.Fail(t, fmt.Sprintf("Service %s did not become ready within %v", url, timeout))
}

// SetupPostgresDB создаёт тестовое подключение к базе данных
func SetupPostgresDB(t *testing.T, cfg *TestConfig) *sql.DB {
	db, err := sql.Open("postgres", cfg.PostgresURL)
	require.NoError(t, err)

	// Проверяем подключение
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = db.PingContext(ctx)
	require.NoError(t, err)

	return db
}

// SetupRedis создаёт тестовое подключение к Redis
func SetupRedis(t *testing.T, cfg *TestConfig) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: cfg.RedisURL,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := client.Ping(ctx).Err()
	require.NoError(t, err)

	return client
}

// CleanupDB очищает тестовые данные из базы данных
func CleanupDB(t *testing.T, db *sql.DB) {
	// Очищаем тестовые таблицы (в порядке зависимостей)
	// Для refresh_tokens удаляем по связанным пользователям
	_, err := db.Exec(`
		DELETE FROM refresh_tokens
		WHERE user_id IN (
			SELECT id FROM users WHERE email LIKE '%test%'
		)
	`)
	if err != nil {
		log.Printf("Warning: Could not clean refresh_tokens table: %v", err)
	}

	// Для users очищаем по email
	_, err = db.Exec("DELETE FROM users WHERE email LIKE '%test%'")
	if err != nil {
		log.Printf("Warning: Could not clean users table: %v", err)
	}
}

// CleanupRedis очищает тестовые данные из Redis
func CleanupRedis(t *testing.T, client *redis.Client) {
	ctx := context.Background()

	// Удаляем ключи, связанные с тестами
	keys, err := client.Keys(ctx, "*test*").Result()
	if err != nil {
		log.Printf("Warning: Could not get Redis keys: %v", err)
		return
	}

	if len(keys) > 0 {
		err = client.Del(ctx, keys...).Err()
		if err != nil {
			log.Printf("Warning: Could not delete Redis keys: %v", err)
		}
	}
}

// HTTPTestClient создаёт настроенный HTTP клиент для тестов
func HTTPTestClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
	}
}
