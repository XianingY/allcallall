package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func setupTestRedis(t *testing.T) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		t.Skip("Redis not available, skipping test")
	}
	return client
}

func cleanupTestRedis(t *testing.T, client *redis.Client) {
	if client != nil {
		client.FlushAll(context.Background())
		client.Close()
	}
}

func TestConversationCache_GetOrCreate(t *testing.T) {
	redis := setupTestRedis(t)
	defer cleanupTestRedis(t, redis)

	cache := NewConversationCache(redis, 5*time.Minute)

	// First call should execute function and cache result
	callCount := 0
	result, err := cache.GetOrCreate("conv:123", func() (interface{}, error) {
		callCount++
		return map[string]interface{}{
			"id":   "123",
			"name": "Test Conversation",
		}, nil
	})

	if err != nil {
		t.Fatalf("GetOrCreate failed: %v", err)
	}

	if callCount != 1 {
		t.Errorf("Function called %d times, want 1", callCount)
	}

	conv := result.(map[string]interface{})
	if conv["name"] != "Test Conversation" {
		t.Errorf("Name = %s, want Test Conversation", conv["name"])
	}

	// Second call should return cached result
	result2, _ := cache.GetOrCreate("conv:123", func() (interface{}, error) {
		callCount++
		return nil, nil
	})
	
	_ = result2

	if callCount != 1 {
		t.Errorf("Function called %d times, want 1 (cache hit)", callCount)
	}
}

func TestConversationCache_Invalidate(t *testing.T) {
	redis := setupTestRedis(t)
	defer cleanupTestRedis(t, redis)

	cache := NewConversationCache(redis, 5*time.Minute)

	// Populate cache
	cache.GetOrCreate("conv:456", func() (interface{}, error) {
		return "cached-value", nil
	})

	// Invalidate
	err := cache.Invalidate("conv:456")
	if err != nil {
		t.Fatalf("Invalidate failed: %v", err)
	}

	// Next call should execute function again
	callCount := 0
	cache.GetOrCreate("conv:456", func() (interface{}, error) {
		callCount++
		return "new-value", nil
	})

	if callCount != 1 {
		t.Errorf("Function called %d times, want 1 (after invalidation)", callCount)
	}
}

func TestConversationCache_BatchInvalidate(t *testing.T) {
	redis := setupTestRedis(t)
	defer cleanupTestRedis(t, redis)

	cache := NewConversationCache(redis, 5*time.Minute)

	// Populate multiple entries
	for i := 0; i < 5; i++ {
		cache.GetOrCreate(fmt.Sprintf("conv:%d", i), func() (interface{}, error) {
			return "value", nil
		})
	}

	// Batch invalidate
	err := cache.BatchInvalidate([]string{"conv:0", "conv:2", "conv:4"})
	if err != nil {
		t.Fatalf("BatchInvalidate failed: %v", err)
	}

	// Verify invalidation
	for i := 0; i < 5; i++ {
		key := fmt.Sprintf("conv:%d", i)
		exists, _ := redis.Exists(context.Background(), key).Result()

		if i%2 == 0 && exists == 1 {
			t.Errorf("Key %s should be invalidated", key)
		}
		if i%2 == 1 && exists == 0 {
			t.Errorf("Key %s should still exist", key)
		}
	}
}
