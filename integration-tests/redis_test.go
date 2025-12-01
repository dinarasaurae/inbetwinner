package integration_tests

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dinarasaurae/inbetwin-integration-tests/testhelpers"
)

func TestRedisIntegration(t *testing.T) {
	cfg := testhelpers.GetTestConfig()
	client := testhelpers.SetupRedis(t, cfg)
	defer client.Close()

	testhelpers.CleanupRedis(t, client)
	defer testhelpers.CleanupRedis(t, client)

	t.Run("ConnectionHealth", func(t *testing.T) {
		testRedisHealth(t, client)
	})

	t.Run("BasicOperations", func(t *testing.T) {
		testRedisBasicOperations(t, client)
	})

	t.Run("ExpirationHandling", func(t *testing.T) {
		testRedisExpiration(t, client)
	})

	t.Run("RateLimitingSimulation", func(t *testing.T) {
		testRedisRateLimiting(t, client)
	})

	t.Run("ConcurrentAccess", func(t *testing.T) {
		testRedisConcurrentAccess(t, client)
	})

	t.Run("DataTypes", func(t *testing.T) {
		testRedisDataTypes(t, client)
	})
}

func testRedisHealth(t *testing.T, client *redis.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Тест ping
	result, err := client.Ping(ctx).Result()
	assert.NoError(t, err)
	assert.Equal(t, "PONG", result)

	// Тест информации о сервере
	info, err := client.Info(ctx).Result()
	assert.NoError(t, err)
	assert.Contains(t, info, "redis_version")

	// Тест статистики
	stats, err := client.ClientList(ctx).Result()
	assert.NoError(t, err)
	assert.NotEmpty(t, stats)
}

func testRedisBasicOperations(t *testing.T, client *redis.Client) {
	ctx := context.Background()
	testKey := "test:basic:key"
	testValue := "test value"

	// SET операция
	err := client.Set(ctx, testKey, testValue, 0).Err()
	require.NoError(t, err)

	// GET операция
	result, err := client.Get(ctx, testKey).Result()
	require.NoError(t, err)
	assert.Equal(t, testValue, result)

	// EXISTS операция
	exists, err := client.Exists(ctx, testKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	// DEL операция
	deleted, err := client.Del(ctx, testKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)

	// Проверяем что ключ удалён
	_, err = client.Get(ctx, testKey).Result()
	assert.Equal(t, redis.Nil, err)
}

func testRedisExpiration(t *testing.T, client *redis.Client) {
	ctx := context.Background()
	testKey := "test:expiration:key"
	testValue := "expiring value"

	// SET с коротким TTL
	err := client.Set(ctx, testKey, testValue, 2*time.Second).Err()
	require.NoError(t, err)

	// Проверяем что ключ существует
	result, err := client.Get(ctx, testKey).Result()
	require.NoError(t, err)
	assert.Equal(t, testValue, result)

	// Проверяем TTL
	ttl, err := client.TTL(ctx, testKey).Result()
	require.NoError(t, err)
	assert.True(t, ttl > 0 && ttl <= 2*time.Second)

	// Ждём истечения TTL
	time.Sleep(3 * time.Second)

	// Проверяем что ключ исчез
	_, err = client.Get(ctx, testKey).Result()
	assert.Equal(t, redis.Nil, err)

	// Тест EXPIRE команды
	testKey2 := "test:expire:key"
	err = client.Set(ctx, testKey2, "expire test", 0).Err()
	require.NoError(t, err)

	// Устанавливаем TTL
	err = client.Expire(ctx, testKey2, 1*time.Second).Err()
	require.NoError(t, err)

	// Проверяем что ключ ещё существует
	exists, err := client.Exists(ctx, testKey2).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	// Ждём истечения
	time.Sleep(2 * time.Second)

	// Проверяем что ключ исчез
	exists, err = client.Exists(ctx, testKey2).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

func testRedisRateLimiting(t *testing.T, client *redis.Client) {
	ctx := context.Background()
	rateLimitKey := "test:rate_limit:127.0.0.1"
	maxRequests := 5
	window := 10 * time.Second

	for i := 0; i < maxRequests; i++ {
		// счётчик
		count, err := client.Incr(ctx, rateLimitKey).Result()
		require.NoError(t, err)

		if count == 1 {
			// TTL только для первого запроса
			err = client.Expire(ctx, rateLimitKey, window).Err()
			require.NoError(t, err)
		}

		assert.LessOrEqual(t, count, int64(maxRequests))
	}

	// Следующий запрос должен превысить лимит
	count, err := client.Incr(ctx, rateLimitKey).Result()
	require.NoError(t, err)
	assert.Greater(t, count, int64(maxRequests))

	// Тест получения TTL для rate limit
	ttl, err := client.TTL(ctx, rateLimitKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))

	// очистка ключа
	err = client.Del(ctx, rateLimitKey).Err()
	require.NoError(t, err)
}

func testRedisConcurrentAccess(t *testing.T, client *redis.Client) {
	ctx := context.Background()
	baseKey := "test:concurrent"
	numGoroutines := 10
	numOperations := 100

	var wg sync.WaitGroup
	var mu sync.Mutex
	errors := make([]error, 0)

	// Запускаем параллельные операции
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("%s:%d:%d", baseKey, goroutineID, j)
				value := fmt.Sprintf("value_%d_%d", goroutineID, j)

				// SET
				err := client.Set(ctx, key, value, time.Minute).Err()
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
					continue
				}

				// GET
				result, err := client.Get(ctx, key).Result()
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
					continue
				}

				if result != value {
					mu.Lock()
					errors = append(errors, fmt.Errorf("value mismatch: expected %s, got %s", value, result))
					mu.Unlock()
				}

				// DEL
				err = client.Del(ctx, key).Err()
				if err != nil {
					mu.Lock()
					errors = append(errors, err)
					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	if len(errors) > 0 {
		t.Errorf("Concurrent access errors: %v", errors[:min(5, len(errors))])
	}

	keys, err := client.Keys(ctx, baseKey+"*").Result()
	if err == nil && len(keys) > 0 {
		client.Del(ctx, keys...)
	}
}

func testRedisDataTypes(t *testing.T, client *redis.Client) {
	ctx := context.Background()

	t.Run("Strings", func(t *testing.T) {
		key := "test:string"

		// MSET/MGET
		err := client.MSet(ctx, key+"1", "value1", key+"2", "value2").Err()
		require.NoError(t, err)

		results, err := client.MGet(ctx, key+"1", key+"2").Result()
		require.NoError(t, err)
		assert.Equal(t, []interface{}{"value1", "value2"}, results)

		// Cleanup
		client.Del(ctx, key+"1", key+"2")
	})

	t.Run("Hashes", func(t *testing.T) {
		key := "test:hash"

		// HSET
		err := client.HSet(ctx, key, "field1", "value1", "field2", "value2").Err()
		require.NoError(t, err)

		// HGET
		value, err := client.HGet(ctx, key, "field1").Result()
		require.NoError(t, err)
		assert.Equal(t, "value1", value)

		// HGETALL
		all, err := client.HGetAll(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"field1": "value1", "field2": "value2"}, all)

		// HDEL
		deleted, err := client.HDel(ctx, key, "field1").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(1), deleted)

		// Cleanup
		client.Del(ctx, key)
	})

	t.Run("Sets", func(t *testing.T) {
		key := "test:set"

		// SADD
		err := client.SAdd(ctx, key, "member1", "member2", "member3").Err()
		require.NoError(t, err)

		// SCARD
		size, err := client.SCard(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), size)

		// SISMEMBER
		isMember, err := client.SIsMember(ctx, key, "member1").Result()
		require.NoError(t, err)
		assert.True(t, isMember)

		// SMEMBERS
		members, err := client.SMembers(ctx, key).Result()
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"member1", "member2", "member3"}, members)

		// Cleanup
		client.Del(ctx, key)
	})

	t.Run("Lists", func(t *testing.T) {
		key := "test:list"

		// LPUSH
		err := client.LPush(ctx, key, "item1", "item2", "item3").Err()
		require.NoError(t, err)

		// LLEN
		length, err := client.LLen(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(3), length)

		// LRANGE
		items, err := client.LRange(ctx, key, 0, -1).Result()
		require.NoError(t, err)
		assert.Equal(t, []string{"item3", "item2", "item1"}, items) // LPUSH добавляет в начало

		// RPOP
		item, err := client.RPop(ctx, key).Result()
		require.NoError(t, err)
		assert.Equal(t, "item1", item)

		// Cleanup
		client.Del(ctx, key)
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
