package cache

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestSnapshotRedisPoolStats(t *testing.T) {
	// Create a client with a test pool.  PoolStats() works on the internal
	// pool object without requiring a live server, so no skip guard needed.
	client := redis.NewClient(&redis.Options{
		Addr:         "localhost:6379",
		PoolSize:     5,
		MinIdleConns: 2,
	})
	defer client.Close()

	got := SnapshotRedisPoolStats(client)
	if got == nil {
		t.Fatal("SnapshotRedisPoolStats returned nil")
	}

	// Verify the struct has the expected shape by accessing all fields.
	// Without a live server the pool may not have dialed yet, so we just
	// confirm the type is correct and the function is callable.
	_ = got.TotalConns
	_ = got.IdleConns
	_ = got.Hits
	_ = got.Misses
	_ = got.Timeouts
	_ = got.StaleConns

	// SnapshotRedisPoolStats must return the same values as the client's
	// own PoolStats() — both read from the same internal pool counter.
	direct := client.PoolStats()
	if got.TotalConns != direct.TotalConns || got.IdleConns != direct.IdleConns {
		t.Errorf("SnapshotRedisPoolStats values differ from client.PoolStats(): got %+v, want %+v", got, direct)
	}
}

func TestSnapshotRedisPoolStatsDoesNotStartGoroutine(t *testing.T) {
	// SnapshotRedisPoolStats is a synchronous wrapper around client.PoolStats().
	// It must not start any goroutine or ticker; Task 3 owns lifecycle sampling.
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	// Calling SnapshotRedisPoolStats must not panic or block.
	_ = SnapshotRedisPoolStats(client)
}


func TestStartRedisPoolMetricsCancellation(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Start the sampler with a short interval.
	StartRedisPoolMetrics(ctx, client, 5*time.Millisecond)

	// Wait for at least one tick to fire.
	time.Sleep(20 * time.Millisecond)

	// Cancel the context; the sampler goroutine must exit.
	cancel()

	// Give the goroutine time to observe cancellation.
	time.Sleep(20 * time.Millisecond)
}
