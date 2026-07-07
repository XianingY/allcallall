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

// SlidingWindow implements a sliding window rate limiter using Redis sorted sets.
// This avoids the burst-at-boundary problem of fixed-window INCR+EXPIRE.
type SlidingWindow struct {
	redis       *redis.Client
	maxRequests int
	window      time.Duration
}

func NewSlidingWindow(redis *redis.Client, maxRequests int, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		redis:       redis,
		maxRequests: maxRequests,
		window:      window,
	}
}

// Allow checks if a request is allowed under the sliding window limit.
// Returns true if allowed, false if rate limited.
func (sw *SlidingWindow) Allow(key string) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-sw.window)
	
	// Redis pipeline for atomic operations
	pipe := sw.redis.Pipeline()
	
	// Remove old entries outside the window
	pipe.ZRemRangeByScore(context.Background(), key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))
	
	// Count current entries in window
	countCmd := pipe.ZCard(context.Background(), key)
	
	// Add current request
	member := fmt.Sprintf("%d", now.UnixNano())
	pipe.ZAdd(context.Background(), key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: member,
	})
	
	// Set expiry on the key
	pipe.Expire(context.Background(), key, sw.window+time.Second)
	
	// Execute pipeline
	_, err := pipe.Exec(context.Background())
	if err != nil {
		return false, fmt.Errorf("redis pipeline failed: %w", err)
	}
	
	// Check results
	count := countCmd.Val()
	
	// If adding this request would exceed limit, remove it and deny
	if count >= int64(sw.maxRequests) {
		pipe.ZRem(context.Background(), key, member)
		pipe.Exec(context.Background())
		return false, nil
	}
	
	return true, nil
}

// GetRemaining returns the number of requests remaining in the current window.
func (sw *SlidingWindow) GetRemaining(key string) (int64, error) {
	now := time.Now()
	windowStart := now.Add(-sw.window)
	
	// Remove old entries and count
	pipe := sw.redis.Pipeline()
	pipe.ZRemRangeByScore(context.Background(), key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))
	countCmd := pipe.ZCard(context.Background(), key)
	
	_, err := pipe.Exec(context.Background())
	if err != nil {
		return 0, err
	}
	
	remaining := int64(sw.maxRequests) - countCmd.Val()
	if remaining < 0 {
		remaining = 0
	}
	
	return remaining, nil
}
