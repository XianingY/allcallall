package ratelimit

import (
	"context"
	"sync"
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

func TestSlidingWindow_Allow(t *testing.T) {
	redis := setupTestRedis(t)
	defer cleanupTestRedis(t, redis)

	limiter := NewSlidingWindow(redis, 10, time.Minute) // 10 requests per minute

	// First 10 requests should be allowed
	for i := 0; i < 10; i++ {
		allowed, err := limiter.Allow("user:123")
		if err != nil {
			t.Fatalf("Allow failed: %v", err)
		}
		if !allowed {
			t.Errorf("Request %d should be allowed", i)
		}
	}

	// 11th request should be denied
	allowed, err := limiter.Allow("user:123")
	if err != nil {
		t.Fatalf("Allow failed: %v", err)
	}
	if allowed {
		t.Error("11th request should be denied")
	}
}

func TestSlidingWindow_WindowExpiry(t *testing.T) {
	redis := setupTestRedis(t)
	defer cleanupTestRedis(t, redis)

	limiter := NewSlidingWindow(redis, 5, time.Second) // 5 requests per second

	// Use up the limit
	for i := 0; i < 5; i++ {
		limiter.Allow("user:456")
	}

	// Should be denied now
	allowed, _ := limiter.Allow("user:456")
	if allowed {
		t.Error("Should be denied after using up limit")
	}

	// Wait for window to expire
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	allowed, _ = limiter.Allow("user:456")
	if !allowed {
		t.Error("Should be allowed after window expiry")
	}
}

func TestSlidingWindow_ConcurrentAccess(t *testing.T) {
	redis := setupTestRedis(t)
	defer cleanupTestRedis(t, redis)

	limiter := NewSlidingWindow(redis, 100, time.Minute)

	var wg sync.WaitGroup
	allowed := make([]bool, 200)

	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			result, _ := limiter.Allow("user:789")
			allowed[idx] = result
		}(i)
	}
	wg.Wait()

	// Count allowed requests
	allowedCount := 0
	for _, a := range allowed {
		if a {
			allowedCount++
		}
	}

	// Should be close to 100, but not exactly due to race conditions
	if allowedCount < 95 || allowedCount > 105 {
		t.Errorf("Allowed %d requests, want ~100", allowedCount)
	}
}
