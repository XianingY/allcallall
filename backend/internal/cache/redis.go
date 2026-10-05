package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	appmetrics "github.com/allcallall/backend/internal/metrics"
	"github.com/rs/zerolog"

	appcfg "github.com/allcallall/backend/internal/config"
)

// NewRedis 创建 Redis 客户端
// NewRedis builds a Redis client and verifies connectivity with a ping.
func NewRedis(ctx context.Context, cfg appcfg.RedisConfig, log zerolog.Logger) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Username:     cfg.Username,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
	})

	if err := ping(ctx, client, log); err != nil {
		return nil, err
	}

	return client, nil
}

func ping(ctx context.Context, client *redis.Client, log zerolog.Logger) error {
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		return err
	}
	log.Info().Str("component", "redis").Msg("connected to redis successfully")
	return nil
}

// SnapshotRedisPoolStats returns the current connection pool statistics for the
// given Redis client.  It delegates to client.PoolStats() and is provided as a
// named function so the metrics pipeline can call it without depending on the
// redis.Client method set directly.  The caller is responsible for periodic
// sampling; this function does not start any goroutine.
func SnapshotRedisPoolStats(client *redis.Client) *redis.PoolStats {
	return client.PoolStats()
}

// RedisPoolObserver is called on each sampling tick with the current pool stats.
// Use it in tests to observe sampling without mutating global metric state.
type RedisPoolObserver func(stats *redis.PoolStats)

// StartRedisPoolMetrics starts a periodic sampler that pushes Redis pool stats
// into the process-default Prometheus metrics.  The sampler runs until ctx is
// cancelled.  Optional observers are called after the metrics push on each tick.
// Call this from a lifecycle owner (bootstrap.RunServer or the
// agent worker), not from NewRedis.
func StartRedisPoolMetrics(ctx context.Context, client *redis.Client, interval time.Duration, observers ...RedisPoolObserver) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				stats := SnapshotRedisPoolStats(client)
				appmetrics.UpdateRedisPoolStats("primary", stats)
				for _, obs := range observers {
					obs(stats)
				}
			}
		}
	}()
}
