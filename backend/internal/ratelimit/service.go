package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Service struct {
	redis *redis.Client
}

func NewService(redis *redis.Client) *Service {
	return &Service{redis: redis}
}

func (s *Service) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, int64, error) {
	if s == nil || s.redis == nil {
		return true, 0, nil
	}
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}

	fullKey := fmt.Sprintf("ratelimit:%s", key)
	count, err := s.redis.Incr(ctx, fullKey).Result()
	if err != nil {
		return false, 0, err
	}
	if count == 1 {
		if err := s.redis.Expire(ctx, fullKey, window).Err(); err != nil {
			return false, 0, err
		}
	}
	if count > limit {
		ttl, ttlErr := s.redis.TTL(ctx, fullKey).Result()
		if ttlErr != nil {
			return false, 0, ttlErr
		}
		return false, int64(ttl.Seconds()), nil
	}
	return true, 0, nil
}
