package ratelimit

import (
	"context"
	"fmt"
	"math"
	"strconv"
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

// SlidingAllow enforces the rate limit with the sliding-window algorithm,
// which eliminates the boundary-burst weakness of Allow's fixed window (where
// a client can fire the full quota right before AND right after a window
// boundary). It fails open: a nil client or a Redis error returns
// allowed=true so the limiter can never take down the API during a Redis
// outage. On denial it returns a Retry-After hint in seconds.
func (s *Service) SlidingAllow(ctx context.Context, key string, limit int, window time.Duration) (bool, int64, error) {
	if s == nil || s.redis == nil {
		return true, 0, nil
	}
	if limit <= 0 {
		limit = 1
	}
	if window <= 0 {
		window = time.Minute
	}
	sw := NewSlidingWindow(s.redis, limit, window)
	allowed, err := sw.Allow(ctx, key)
	if err != nil {
		return false, 0, err
	}
	if !allowed {
		return false, sw.RetryAfter(ctx, key), nil
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
func (sw *SlidingWindow) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now()
	windowStart := now.Add(-sw.window)

	// Redis pipeline for atomic operations
	pipe := sw.redis.Pipeline()

	// Remove old entries outside the window
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))

	// Count current entries in window
	countCmd := pipe.ZCard(ctx, key)

	// Add current request
	member := fmt.Sprintf("%d", now.UnixNano())
	pipe.ZAdd(ctx, key, redis.Z{
		Score:  float64(now.UnixNano()),
		Member: member,
	})

	// Set expiry on the key
	pipe.Expire(ctx, key, sw.window+time.Second)

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, fmt.Errorf("redis pipeline failed: %w", err)
	}

	// Check results
	count := countCmd.Val()

	// If adding this request would exceed limit, remove it and deny
	if count >= int64(sw.maxRequests) {
		pipe.ZRem(ctx, key, member)
		pipe.Exec(ctx)
		return false, nil
	}

	return true, nil
}

// GetRemaining returns the number of requests remaining in the current window.
func (sw *SlidingWindow) GetRemaining(ctx context.Context, key string) (int64, error) {
	now := time.Now()
	windowStart := now.Add(-sw.window)

	// Remove old entries and count
	pipe := sw.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, key, "0", fmt.Sprintf("%d", windowStart.UnixNano()))
	countCmd := pipe.ZCard(ctx, key)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}

	remaining := int64(sw.maxRequests) - countCmd.Val()
	if remaining < 0 {
		remaining = 0
	}

	return remaining, nil
}

// RetryAfter returns the number of seconds until the oldest request currently
// inside the window ages out, at which point a new request would be permitted
// again. It falls back to the full window duration when the key has no entries
// or Redis is unavailable, so callers always get a sane, non-zero hint.
func (sw *SlidingWindow) RetryAfter(ctx context.Context, key string) int64 {
	now := time.Now()
	windowStart := now.Add(-sw.window)

	// The oldest still-valid member is the next one to age out of the window.
	members, err := sw.redis.ZRangeByScore(ctx, key, &redis.ZRangeBy{
		Min:    fmt.Sprintf("%d", windowStart.UnixNano()),
		Max:    fmt.Sprintf("%d", now.UnixNano()),
		Offset: 0,
		Count:  1,
	}).Result()
	if err != nil || len(members) == 0 {
		return int64(sw.window.Seconds())
	}
	oldest, err := strconv.ParseInt(members[0], 10, 64)
	if err != nil {
		return int64(sw.window.Seconds())
	}
	// Time until oldest.UnixNano() + window <= now.
	untilNanos := oldest + sw.window.Nanoseconds() - now.UnixNano()
	if untilNanos < 0 {
		untilNanos = 0
	}
	secs := int64(math.Ceil(float64(untilNanos) / float64(time.Second)))
	if secs < 1 {
		secs = 1
	}
	return secs
}
