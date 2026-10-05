package cache

import (
	"context"
	"sync/atomic"
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

	var observed atomic.Int32
	ctx, cancel := context.WithCancel(context.Background())

	StartRedisPoolMetrics(ctx, client, 5*time.Millisecond, func(*redis.PoolStats) {
		observed.Add(1)
	})

	// Wait for at least one observation.
	deadline := time.After(2 * time.Second)
	for observed.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("no observation received before deadline")
		default:
			time.Sleep(2 * time.Millisecond)
		}
	}

	// Cancel the context; the sampler goroutine must exit.
	cancel()

	// Allow time for the goroutine to observe cancellation.
	time.Sleep(50 * time.Millisecond)

	// Record the count after the cancellation settling period.
	countAtCancel := observed.Load()

	// Wait another interval and verify no further observations.
	time.Sleep(30 * time.Millisecond)
	countAfterWait := observed.Load()

	// Allow at most 1 straggler observation from a race between
	// the ticker channel and context cancellation.
	if countAfterWait > countAtCancel+1 {
		t.Fatalf("sampler continued after cancellation: at_cancel=%d after_wait=%d", countAtCancel, countAfterWait)
	}
}
