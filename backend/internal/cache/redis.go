package cache

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
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
