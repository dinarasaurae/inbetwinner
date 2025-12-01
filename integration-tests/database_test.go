package integration_tests

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dinarasaurae/inbetwin-integration-tests/testhelpers"
)

func TestDatabaseIntegration(t *testing.T) {
	cfg := testhelpers.GetTestConfig()
	db := testhelpers.SetupPostgresDB(t, cfg)
	defer db.Close()

	testhelpers.CleanupDB(t, db)
	defer testhelpers.CleanupDB(t, db)

	t.Run("ConnectionHealth", func(t *testing.T) {
		testDatabaseHealth(t, db)
	})

	t.Run("UserCRUD", func(t *testing.T) {
		testUserCRUD(t, db)
	})

	t.Run("RefreshTokenOperations", func(t *testing.T) {
		testRefreshTokenOperations(t, db)
	})

	t.Run("TransactionConsistency", func(t *testing.T) {
		testTransactionConsistency(t, db)
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		testConcurrentAccess(t, db)
	})
}

func testDatabaseHealth(t *testing.T, db *sql.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := db.PingContext(ctx)
	assert.NoError(t, err)

	var version string
	err = db.QueryRowContext(ctx, "SELECT version()").Scan(&version)
	assert.NoError(t, err)
	assert.Contains(t, version, "PostgreSQL")

	stats := db.Stats()
	assert.GreaterOrEqual(t, stats.OpenConnections, 0)
	assert.True(t, stats.OpenConnections >= 0)
}

func testUserCRUD(t *testing.T, db *sql.DB) {
	userID := uuid.New()
	testEmail := "test.crud@example.com"
	testName := "CRUD Test User"
	testPassword := "$2a$10$example.hashed.password.here"

	_, err := db.Exec(`
		INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, testEmail, testName, testPassword, time.Now(), time.Now())
	require.NoError(t, err)

	var foundID uuid.UUID
	var foundEmail, foundName, foundPasswordHash string
	var createdAt, updatedAt time.Time

	err = db.QueryRow(`
		SELECT id, email, name, password_hash, created_at, updated_at
		FROM users WHERE id = $1`, userID).Scan(
		&foundID, &foundEmail, &foundName, &foundPasswordHash, &createdAt, &updatedAt)
	require.NoError(t, err)

	assert.Equal(t, userID, foundID)
	assert.Equal(t, testEmail, foundEmail)
	assert.Equal(t, testName, foundName)
	assert.Equal(t, testPassword, foundPasswordHash)
	assert.WithinDuration(t, time.Now(), createdAt, 10*time.Second)
	assert.WithinDuration(t, time.Now(), updatedAt, 10*time.Second)

	newName := "Updated CRUD Test User"
	_, err = db.Exec(`
		UPDATE users SET name = $1, updated_at = $2 WHERE id = $3`,
		newName, time.Now(), userID)
	require.NoError(t, err)

	err = db.QueryRow("SELECT name FROM users WHERE id = $1", userID).Scan(&foundName)
	require.NoError(t, err)
	assert.Equal(t, newName, foundName)

	result, err := db.Exec("DELETE FROM users WHERE id = $1", userID)
	require.NoError(t, err)

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	err = db.QueryRow("SELECT id FROM users WHERE id = $1", userID).Scan(&foundID)
	assert.Equal(t, sql.ErrNoRows, err)
}

func testRefreshTokenOperations(t *testing.T, db *sql.DB) {
	userID := uuid.New()
	testEmail := "test.token@example.com"

	_, err := db.Exec(`
		INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, testEmail, "Token Test User", "password", time.Now(), time.Now())
	require.NoError(t, err)

	tokenID := uuid.New()
	tokenHash := "hashed.refresh.token"
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = db.Exec(`
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		tokenID, userID, tokenHash, expiresAt, time.Now())
	require.NoError(t, err)

	var foundTokenID uuid.UUID
	var foundUserID uuid.UUID
	var foundTokenHash string
	var foundExpiresAt time.Time

	err = db.QueryRow(`
		SELECT id, user_id, token_hash, expires_at
		FROM refresh_tokens WHERE id = $1`, tokenID).Scan(
		&foundTokenID, &foundUserID, &foundTokenHash, &foundExpiresAt)
	require.NoError(t, err)

	assert.Equal(t, tokenID, foundTokenID)
	assert.Equal(t, userID, foundUserID)
	assert.Equal(t, tokenHash, foundTokenHash)
	assert.WithinDuration(t, expiresAt, foundExpiresAt, time.Second)

	expiredTokenID := uuid.New()
	_, err = db.Exec(`
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		expiredTokenID, userID, "expired.token", time.Now().Add(-time.Hour), time.Now())
	require.NoError(t, err)

	result, err := db.Exec("DELETE FROM refresh_tokens WHERE expires_at < $1", time.Now())
	require.NoError(t, err)

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)

	err = db.QueryRow("SELECT id FROM refresh_tokens WHERE id = $1", tokenID).Scan(&foundTokenID)
	assert.NoError(t, err)

	err = db.QueryRow("SELECT id FROM refresh_tokens WHERE id = $1", expiredTokenID).Scan(&foundTokenID)
	assert.Equal(t, sql.ErrNoRows, err)

	_, _ = db.Exec("DELETE FROM refresh_tokens WHERE user_id = $1", userID)
	_, _ = db.Exec("DELETE FROM users WHERE id = $1", userID)
}

func testTransactionConsistency(t *testing.T, db *sql.DB) {
	userID := uuid.New()
	testEmail := "test.transaction@example.com"

	tx, err := db.Begin()
	require.NoError(t, err)

	_, err = tx.Exec(`
		INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, testEmail, "Transaction Test", "password", time.Now(), time.Now())
	require.NoError(t, err)

	tokenID := uuid.New()
	_, err = tx.Exec(`
		INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		tokenID, userID, "token.hash", time.Now().Add(time.Hour), time.Now())
	require.NoError(t, err)

	err = tx.Commit()
	require.NoError(t, err)

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE id = $1", userID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	err = db.QueryRow("SELECT COUNT(*) FROM refresh_tokens WHERE id = $1", tokenID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Тест отката транзакции
	anotherUserID := uuid.New()
	tx, err = db.Begin()
	require.NoError(t, err)

	_, err = tx.Exec(`
		INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		anotherUserID, "rollback@example.com", "Rollback Test", "password", time.Now(), time.Now())
	require.NoError(t, err)

	err = tx.Rollback()
	require.NoError(t, err)

	err = db.QueryRow("SELECT COUNT(*) FROM users WHERE id = $1", anotherUserID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	_, _ = db.Exec("DELETE FROM refresh_tokens WHERE user_id = $1", userID)
	_, _ = db.Exec("DELETE FROM users WHERE id = $1", userID)
}

func testConcurrentAccess(t *testing.T, db *sql.DB) {
	userID := uuid.New()
	testEmail := "test.concurrent@example.com"

	_, err := db.Exec(`
		INSERT INTO users (id, email, name, password_hash, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, testEmail, "Concurrent Test", "password", time.Now(), time.Now())
	require.NoError(t, err)

	defer func() {
		_, _ = db.Exec("DELETE FROM users WHERE id = $1", userID)
	}()

	done := make(chan bool, 10)
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		go func() {
			var foundID uuid.UUID
			var foundEmail string
			err := db.QueryRow("SELECT id, email FROM users WHERE id = $1", userID).Scan(&foundID, &foundEmail)
			if err != nil {
				errors <- err
			} else {
				assert.Equal(t, userID, foundID)
				assert.Equal(t, testEmail, foundEmail)
			}
			done <- true
		}()
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case err := <-errors:
			t.Errorf("Concurrent read error: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent reads")
		}
	}
}
