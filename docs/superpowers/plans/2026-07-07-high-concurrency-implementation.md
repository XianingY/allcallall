# AllCallAll High Concurrency & Production Readiness Implementation Plan

> **HISTORICAL NOTE (2026-07-22):** The Python/FastAPI/LangGraph runtime listed
> in the tech stack below is no longer part of this repository; it lives in the
> sibling repository `allcallall-agent-runtime` and integrates over HTTP.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Transform AllCallAll from a demo/portfolio project into a production-ready system supporting high concurrency (1000+ concurrent users) and high data volume (millions of messages, recordings, and agent runs).

**Architecture:** Multi-phase approach: (1) Backend performance hardening, (2) Frontend optimization, (3) Infrastructure scaling, (4) Observability, (5) Load testing validation. Each phase produces testable, deployable improvements.

**Tech Stack:** Go/Gin/Gorm, React/Vite, React Native/Expo, Python/FastAPI/LangGraph, MySQL 8.0, Redis 7.2, Elasticsearch, Kafka, Docker Compose, Nginx, Coturn

---

## Phase 1: Backend Performance Hardening (Weeks 1-2)

### Task 1.1: Database Connection Pool Optimization

**Files:**
- Modify: `backend/internal/config/config.go`
- Modify: `backend/configs/config.yaml`
- Modify: `backend/internal/database/mysql.go`
- Test: `backend/internal/database/mysql_test.go`

- [ ] **Step 1: Write failing test for pool configuration**

```go
// backend/internal/database/mysql_test.go
package database

import (
    "testing"
    "time"
)

func TestNewMySQL_PoolConfiguration(t *testing.T) {
    cfg := Config{
        DSN:             "test:test@tcp(localhost:3306)/test",
        MaxOpenConns:    200,
        MaxIdleConns:    50,
        ConnMaxLifetime: 10 * time.Minute,
        ConnMaxIdleTime: 5 * time.Minute,
    }
    
    // This test will fail until we implement the pool configuration
    db, err := NewMySQL(cfg)
    if err != nil {
        // Expected to fail with invalid DSN, but we test config parsing
        t.Skip("Skipping live connection test")
    }
    
    stats := db.Stats()
    if stats.MaxOpenConnections != 200 {
        t.Errorf("MaxOpenConnections = %d, want 200", stats.MaxOpenConnections)
    }
}

func TestConfig_PoolDefaults(t *testing.T) {
    cfg := Config{}
    cfg.ApplyDefaults()
    
    if cfg.MaxOpenConns != 200 {
        t.Errorf("Default MaxOpenConns = %d, want 200", cfg.MaxOpenConns)
    }
    if cfg.MaxIdleConns != 50 {
        t.Errorf("Default MaxIdleConns = %d, want 50", cfg.MaxIdleConns)
    }
    if cfg.ConnMaxLifetime != 10*time.Minute {
        t.Errorf("Default ConnMaxLifetime = %v, want 10m", cfg.ConnMaxLifetime)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -v -run 'TestNewMySQL_PoolConfiguration|TestConfig_PoolDefaults' ./internal/database/`
Expected: FAIL with "undefined: Config.ApplyDefaults" or similar

- [ ] **Step 3: Implement pool configuration**

```go
// backend/internal/config/config.go - Add to Config struct
type DatabaseConfig struct {
    DSN              string        `yaml:"dsn" env:"DB_DSN"`
    MaxOpenConns     int           `yaml:"max_open_conns" env:"DB_MAX_OPEN_CONNS"`
    MaxIdleConns     int           `yaml:"max_idle_conns" env:"DB_MAX_IDLE_CONNS"`
    ConnMaxLifetime  time.Duration `yaml:"conn_max_lifetime" env:"DB_CONN_MAX_LIFETIME"`
    ConnMaxIdleTime  time.Duration `yaml:"conn_max_idle_time" env:"DB_CONN_MAX_IDLE_TIME"`
}

func (c *DatabaseConfig) ApplyDefaults() {
    if c.MaxOpenConns == 0 {
        c.MaxOpenConns = 200
    }
    if c.MaxIdleConns == 0 {
        c.MaxIdleConns = 50
    }
    if c.ConnMaxLifetime == 0 {
        c.ConnMaxLifetime = 10 * time.Minute
    }
    if c.ConnMaxIdleTime == 0 {
        c.ConnMaxIdleTime = 5 * time.Minute
    }
}
```

```go
// backend/internal/database/mysql.go - Update NewMySQL
func NewMySQL(cfg config.DatabaseConfig) (*gorm.DB, error) {
    cfg.ApplyDefaults()
    
    db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
        Logger: logger.Default.LogMode(logger.Info),
    })
    if err != nil {
        return nil, err
    }
    
    sqlDB, err := db.DB()
    if err != nil {
        return nil, err
    }
    
    sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
    sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
    sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
    sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
    
    return db, nil
}
```

```yaml
# backend/configs/config.yaml - Update database section
database:
  dsn: "root:password@tcp(127.0.0.1:3306)/allcallall?charset=utf8mb4&parseTime=True&loc=Local"
  max_open_conns: 200
  max_idle_conns: 50
  conn_max_lifetime: 10m
  conn_max_idle_time: 5m
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -v -run 'TestNewMySQL_PoolConfiguration|TestConfig_PoolDefaults' ./internal/database/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/config/config.go backend/configs/config.yaml backend/internal/database/mysql.go backend/internal/database/mysql_test.go
git commit -m "feat(db): optimize MySQL connection pool for high concurrency

- Increase MaxOpenConns from 50 to 200
- Increase MaxIdleConns from 10 to 50
- Add ConnMaxIdleTime for connection cleanup
- Add ApplyDefaults() for safe configuration
- Add pool configuration tests"
```

---

### Task 1.2: Batch Outbox Claiming with SKIP LOCKED

**Files:**
- Modify: `backend/internal/events/outbox.go`
- Test: `backend/internal/events/outbox_test.go`

- [ ] **Step 1: Write failing test for batch claiming**

```go
// backend/internal/events/outbox_test.go
package events

import (
    "testing"
    "time"
)

func TestOutbox_BatchClaimPending(t *testing.T) {
    // Setup: Create 10 pending events
    // Test: Claim 5 events in batch
    // Verify: Only 5 events returned, all have lease_until set
    // Verify: Remaining 5 events still pending
    
    db := setupTestDB(t)
    defer cleanupTestDB(t)
    
    // Create 10 pending events
    for i := 0; i < 10; i++ {
        event := &EventOutbox{
            EventName:   "test.event",
            Payload:     fmt.Sprintf(`{"index": %d}`, i),
            Status:      "pending",
            IdempotencyKey: fmt.Sprintf("key-%d", i),
            CreatedAt:   time.Now(),
        }
        if err := db.Create(event).Error; err != nil {
            t.Fatalf("Failed to create event: %v", err)
        }
    }
    
    // Claim 5 events in batch
    claimed, err := ClaimBatchPending(db, "worker-1", 5, 30*time.Second)
    if err != nil {
        t.Fatalf("ClaimBatchPending failed: %v", err)
    }
    
    if len(claimed) != 5 {
        t.Errorf("Claimed %d events, want 5", len(claimed))
    }
    
    // Verify lease_until is set
    for _, event := range claimed {
        if event.LeaseUntil == nil {
            t.Error("LeaseUntil not set on claimed event")
        }
        if event.ClaimedBy != "worker-1" {
            t.Errorf("ClaimedBy = %s, want worker-1", event.ClaimedBy)
        }
    }
    
    // Verify remaining 5 still pending
    var pendingCount int64
    db.Model(&EventOutbox{}).Where("status = ?", "pending").Count(&pendingCount)
    if pendingCount != 5 {
        t.Errorf("Pending count = %d, want 5", pendingCount)
    }
}

func TestOutbox_BatchClaimConcurrency(t *testing.T) {
    // Setup: Create 20 pending events
    // Test: 4 workers claim 5 events each concurrently
    // Verify: All 20 events claimed, no duplicates
    
    db := setupTestDB(t)
    defer cleanupTestDB(t)
    
    // Create 20 pending events
    for i := 0; i < 20; i++ {
        event := &EventOutbox{
            EventName:   "test.event",
            Payload:     fmt.Sprintf(`{"index": %d}`, i),
            Status:      "pending",
            IdempotencyKey: fmt.Sprintf("key-%d", i),
            CreatedAt:   time.Now(),
        }
        db.Create(event)
    }
    
    // Run 4 workers concurrently
    var wg sync.WaitGroup
    claimedByWorker := make([][]EventOutbox, 4)
    
    for w := 0; w < 4; w++ {
        wg.Add(1)
        go func(workerID int) {
            defer wg.Done()
            claimed, _ := ClaimBatchPending(db, fmt.Sprintf("worker-%d", workerID), 5, 30*time.Second)
            claimedByWorker[workerID] = claimed
        }(w)
    }
    wg.Wait()
    
    // Verify no duplicates
    seen := make(map[uint]bool)
    totalClaimed := 0
    for _, claimed := range claimedByWorker {
        for _, event := range claimed {
            if seen[event.ID] {
                t.Errorf("Event %d claimed by multiple workers", event.ID)
            }
            seen[event.ID] = true
            totalClaimed++
        }
    }
    
    if totalClaimed != 20 {
        t.Errorf("Total claimed = %d, want 20", totalClaimed)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -v -run 'TestOutbox_Batch' ./internal/events/`
Expected: FAIL with "undefined: ClaimBatchPending"

- [ ] **Step 3: Implement batch claiming**

```go
// backend/internal/events/outbox.go - Add batch claiming function

// ClaimBatchPending claims up to batchSize pending events atomically using
// SELECT ... FOR UPDATE SKIP LOCKED to avoid contention between workers.
func ClaimBatchPending(db *gorm.DB, workerID string, batchSize int, leaseDuration time.Duration) ([]EventOutbox, error) {
    var claimed []EventOutbox
    
    // Use raw SQL with SKIP LOCKED for efficient batch claiming
    result := db.Raw(`
        SELECT id, event_name, payload, status, idempotency_key, attempts, created_at
        FROM event_outbox
        WHERE status = 'pending' 
          AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
        ORDER BY id ASC
        LIMIT ?
        FOR UPDATE SKIP LOCKED
    `, batchSize).Scan(&claimed)
    
    if result.Error != nil {
        return nil, fmt.Errorf("failed to claim pending events: %w", result.Error)
    }
    
    if len(claimed) == 0 {
        return nil, nil
    }
    
    // Update claimed events with lease information
    ids := make([]uint, len(claimed))
    for i, event := range claimed {
        ids[i] = event.ID
    }
    
    leaseUntil := time.Now().Add(leaseDuration)
    result = db.Model(&EventOutbox{}).
        Where("id IN ?", ids).
        Updates(map[string]interface{}{
            "status":       "claimed",
            "claimed_by":   workerID,
            "lease_until":  leaseUntil,
            "last_attempt": time.Now(),
            "attempts":     gorm.Expr("attempts + 1"),
        })
    
    if result.Error != nil {
        return nil, fmt.Errorf("failed to update claimed events: %w", result.Error)
    }
    
    // Set the fields on the returned structs
    for i := range claimed {
        claimed[i].Status = "claimed"
        claimed[i].ClaimedBy = workerID
        claimed[i].LeaseUntil = &leaseUntil
    }
    
    return claimed, nil
}

// ReleaseExpiredLeases releases events whose lease has expired
func ReleaseExpiredLeases(db *gorm.DB) (int64, error) {
    result := db.Model(&EventOutbox{}).
        Where("status = 'claimed' AND lease_until < NOW()").
        Updates(map[string]interface{}{
            "status":      "pending",
            "claimed_by":  nil,
            "lease_until": nil,
        })
    
    return result.RowsAffected, result.Error
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -v -run 'TestOutbox_Batch' ./internal/events/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/events/outbox.go backend/internal/events/outbox_test.go
git commit -m "feat(outbox): batch claiming with SELECT FOR UPDATE SKIP LOCKED

- Replace row-by-row claiming with efficient batch operation
- Add SKIP LOCKED to avoid contention between concurrent workers
- Add ReleaseExpiredLeases for stuck event recovery
- Add concurrency test to verify no duplicate claims"
```

---

### Task 1.3: Sliding Window Rate Limiter

**Files:**
- Modify: `backend/internal/ratelimit/service.go`
- Test: `backend/internal/ratelimit/service_test.go`

- [ ] **Step 1: Write failing test for sliding window**

```go
// backend/internal/ratelimit/service_test.go
package ratelimit

import (
    "testing"
    "time"
)

func TestSlidingWindow_Allow(t *testing.T) {
    redis := setupTestRedis(t)
    defer cleanupTestRedis(t)
    
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
    defer cleanupTestRedis(t)
    
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
    defer cleanupTestRedis(t)
    
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -v -run 'TestSlidingWindow' ./internal/ratelimit/`
Expected: FAIL with "undefined: NewSlidingWindow"

- [ ] **Step 3: Implement sliding window**

```go
// backend/internal/ratelimit/service.go - Add sliding window implementation

// SlidingWindow implements a sliding window rate limiter using Redis sorted sets.
// This avoids the burst-at-boundary problem of fixed-window INCR+EXPIRE.
type SlidingWindow struct {
    redis      *redis.Client
    maxRequests int
    window     time.Duration
}

func NewSlidingWindow(redis *redis.Client, maxRequests int, window time.Duration) *SlidingWindow {
    return &SlidingWindow{
        redis:      redis,
        maxRequests: maxRequests,
        window:     window,
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
    removeCmd := pipe.ZRemRangeByScore(key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))
    
    // Count current entries in window
    countCmd := pipe.ZCard(key)
    
    // Add current request
    member := strconv.FormatInt(now.UnixNano(), 10)
    addCmd := pipe.ZAdd(key, &redis.Z{
        Score:  float64(now.UnixNano()),
        Member: member,
    })
    
    // Set expiry on the key
    expireCmd := pipe.Expire(key, sw.window+time.Second)
    
    // Execute pipeline
    _, err := pipe.Exec(context.Background())
    if err != nil {
        return false, fmt.Errorf("redis pipeline failed: %w", err)
    }
    
    // Check results
    count := countCmd.Val()
    
    // If adding this request would exceed limit, remove it and deny
    if count >= int64(sw.maxRequests) {
        pipe.ZRem(key, member)
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
    pipe.ZRemRangeByScore(key, "0", strconv.FormatInt(windowStart.UnixNano(), 10))
    countCmd := pipe.ZCard(key)
    
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -v -run 'TestSlidingWindow' ./internal/ratelimit/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/ratelimit/service.go backend/internal/ratelimit/service_test.go
git commit -m "feat(ratelimit): sliding window implementation using Redis sorted sets

- Replace fixed-window INCR+EXPIRE with sliding window
- Avoid burst-at-boundary problem (2x limit at window edge)
- Add concurrent access test for thread safety
- Add GetRemaining() for client feedback"
```

---

### Task 1.4: Redis Caching Layer for Hot Paths

**Files:**
- Modify: `backend/internal/collaboration/service.go`
- Create: `backend/internal/cache/conversation_cache.go`
- Test: `backend/internal/cache/conversation_cache_test.go`

- [ ] **Step 1: Write failing test for conversation cache**

```go
// backend/internal/cache/conversation_cache_test.go
package cache

import (
    "testing"
    "time"
)

func TestConversationCache_GetOrCreate(t *testing.T) {
    redis := setupTestRedis(t)
    defer cleanupTestRedis(t)
    
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
    
    if callCount != 1 {
        t.Errorf("Function called %d times, want 1 (cache hit)", callCount)
    }
}

func TestConversationCache_Invalidate(t *testing.T) {
    redis := setupTestRedis(t)
    defer cleanupTestRedis(t)
    
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
    defer cleanupTestRedis(t)
    
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
        exists, _ := redis.Exists(key).Result()
        
        if i%2 == 0 && exists == 1 {
            t.Errorf("Key %s should be invalidated", key)
        }
        if i%2 == 1 && exists == 0 {
            t.Errorf("Key %s should still exist", key)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -v -run 'TestConversationCache' ./internal/cache/`
Expected: FAIL with "undefined: NewConversationCache"

- [ ] **Step 3: Implement conversation cache**

```go
// backend/internal/cache/conversation_cache.go
package cache

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "github.com/redis/go-redis/v9"
)

// ConversationCache provides Redis-backed caching for conversation data.
type ConversationCache struct {
    redis     *redis.Client
    ttl       time.Duration
    keyPrefix string
}

func NewConversationCache(redis *redis.Client, ttl time.Duration) *ConversationCache {
    return &ConversationCache{
        redis:     redis,
        ttl:       ttl,
        keyPrefix: "conv:",
    }
}

// GetOrCreate retrieves a cached conversation or executes the function and caches the result.
func (c *ConversationCache) GetOrCreate(conversationID string, fn func() (interface{}, error)) (interface{}, error) {
    key := c.keyPrefix + conversationID
    
    // Try to get from cache
    cached, err := c.redis.Get(context.Background(), key).Result()
    if err == nil {
        var result interface{}
        if err := json.Unmarshal([]byte(cached), &result); err == nil {
            return result, nil
        }
    }
    
    // Cache miss - execute function
    result, err := fn()
    if err != nil {
        return nil, err
    }
    
    // Cache the result
    data, err := json.Marshal(result)
    if err != nil {
        return result, nil // Return result even if caching fails
    }
    
    c.redis.Set(context.Background(), key, string(data), c.ttl)
    
    return result, nil
}

// Invalidate removes a conversation from cache.
func (c *ConversationCache) Invalidate(conversationID string) error {
    key := c.keyPrefix + conversationID
    return c.redis.Del(context.Background(), key).Err()
}

// BatchInvalidate removes multiple conversations from cache.
func (c *ConversationCache) BatchInvalidate(conversationIDs []string) error {
    if len(conversationIDs) == 0 {
        return nil
    }
    
    keys := make([]string, len(conversationIDs))
    for i, id := range conversationIDs {
        keys[i] = c.keyPrefix + id
    }
    
    return c.redis.Del(context.Background(), keys...).Err()
}

// InvalidatePattern invalidates all conversations matching a pattern.
func (c *ConversationCache) InvalidatePattern(pattern string) error {
    ctx := context.Background()
    var cursor uint64
    
    for {
        keys, nextCursor, err := c.redis.Scan(ctx, cursor, c.keyPrefix+pattern, 100).Result()
        if err != nil {
            return err
        }
        
        if len(keys) > 0 {
            c.redis.Del(ctx, keys...)
        }
        
        cursor = nextCursor
        if cursor == 0 {
            break
        }
    }
    
    return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -v -run 'TestConversationCache' ./internal/cache/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/cache/conversation_cache.go backend/internal/cache/conversation_cache_test.go
git commit -m "feat(cache): Redis-backed conversation cache for hot paths

- Add GetOrCreate pattern for cache-aside implementation
- Add Invalidate and BatchInvalidate for cache invalidation
- Add pattern-based invalidation for org-wide updates
- Add comprehensive tests for cache behavior"
```

---

## Phase 2: Frontend Optimization (Weeks 2-3)

### Task 2.1: Web App Code Splitting & Lazy Loading

**Files:**
- Modify: `web/src/App.tsx`
- Modify: `web/src/main.tsx`
- Create: `web/src/components/LazyLoad.tsx`
- Test: `web/src/components/LazyLoad.test.tsx`

- [ ] **Step 1: Write failing test for lazy loading**

```tsx
// web/src/components/LazyLoad.test.tsx
import { render, screen, waitFor } from '@testing-library/react';
import { LazyLoad } from './LazyLoad';

// Mock a slow component
const SlowComponent = () => <div data-testid="slow-component">Loaded!</div>;

describe('LazyLoad', () => {
  it('shows loading state while component loads', async () => {
    render(
      <LazyLoad
        loader={() => import('./SlowComponent')}
        fallback={<div data-testid="loading">Loading...</div>}
      />
    );
    
    expect(screen.getByTestId('loading')).toBeInTheDocument();
    
    await waitFor(() => {
      expect(screen.getByTestId('slow-component')).toBeInTheDocument();
    });
  });

  it('shows error state on load failure', async () => {
    const failingLoader = () => Promise.reject(new Error('Load failed'));
    
    render(
      <LazyLoad
        loader={failingLoader}
        fallback={<div>Loading...</div>}
        errorFallback={<div data-testid="error">Error occurred</div>}
      />
    );
    
    await waitFor(() => {
      expect(screen.getByTestId('error')).toBeInTheDocument();
    });
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd web && npm test -- --testPathPattern='LazyLoad'`
Expected: FAIL with "Cannot find module './LazyLoad'"

- [ ] **Step 3: Implement lazy loading component**

```tsx
// web/src/components/LazyLoad.tsx
import React, { Suspense, lazy, ComponentType, ReactNode } from 'react';

interface LazyLoadProps {
  loader: () => Promise<{ default: ComponentType<any> }>;
  fallback?: ReactNode;
  errorFallback?: ReactNode;
  onError?: (error: Error) => void;
}

interface LazyLoadState {
  hasError: boolean;
  error?: Error;
}

export class LazyLoad extends React.Component<LazyLoadProps, LazyLoadState> {
  constructor(props: LazyLoadProps) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): LazyLoadState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: React.ErrorInfo) {
    console.error('LazyLoad error:', error, errorInfo);
    this.props.onError?.(error);
  }

  render() {
    if (this.state.hasError) {
      return this.props.errorFallback || <div>Something went wrong.</div>;
    }

    const LazyComponent = lazy(this.props.loader);

    return (
      <Suspense fallback={this.props.fallback || <div>Loading...</div>}>
        <LazyComponent />
      </Suspense>
    );
  }
}
```

```tsx
// web/src/App.tsx - Update with lazy loading
import React from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { LazyLoad } from './components/LazyLoad';
import { Layout } from './components/Layout';
import { AuthGuard } from './components/AuthGuard';

// Lazy load heavy components
const Dashboard = React.lazy(() => import('./pages/Dashboard'));
const Conversations = React.lazy(() => import('./pages/Conversations'));
const Chat = React.lazy(() => import('./pages/Chat'));
const Calls = React.lazy(() => import('./pages/Calls'));
const Meetings = React.lazy(() => import('./pages/Meetings'));
const Recordings = React.lazy(() => import('./pages/Recordings'));
const AgentLab = React.lazy(() => import('./pages/AgentLab'));
const Settings = React.lazy(() => import('./pages/Settings'));

function App() {
  return (
    <BrowserRouter>
      <AuthGuard>
        <Layout>
          <Routes>
            <Route path="/" element={
              <LazyLoad
                loader={() => import('./pages/Dashboard')}
                fallback={<div>Loading Dashboard...</div>}
              />
            } />
            <Route path="/conversations" element={
              <LazyLoad
                loader={() => import('./pages/Conversations')}
                fallback={<div>Loading Conversations...</div>}
              />
            } />
            <Route path="/conversations/:id" element={
              <LazyLoad
                loader={() => import('./pages/Chat')}
                fallback={<div>Loading Chat...</div>}
              />
            } />
            <Route path="/calls" element={
              <LazyLoad
                loader={() => import('./pages/Calls')}
                fallback={<div>Loading Calls...</div>}
              />
            } />
            <Route path="/meetings" element={
              <LazyLoad
                loader={() => import('./pages/Meetings')}
                fallback={<div>Loading Meetings...</div>}
              />
            } />
            <Route path="/recordings" element={
              <LazyLoad
                loader={() => import('./pages/Recordings')}
                fallback={<div>Loading Recordings...</div>}
              />
            } />
            <Route path="/agent-lab" element={
              <LazyLoad
                loader={() => import('./pages/AgentLab')}
                fallback={<div>Loading Agent Lab...</div>}
              />
            } />
            <Route path="/settings" element={
              <LazyLoad
                loader={() => import('./pages/Settings')}
                fallback={<div>Loading Settings...</div>}
              />
            } />
          </Routes>
        </Layout>
      </AuthGuard>
    </BrowserRouter>
  );
}

export default App;
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd web && npm test -- --testPathPattern='LazyLoad'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add web/src/components/LazyLoad.tsx web/src/components/LazyLoad.test.tsx web/src/App.tsx
git commit -m "feat(web): add lazy loading for route-based code splitting

- Add LazyLoad component with Suspense and error boundary
- Lazy load all route components (Dashboard, Chat, Calls, etc.)
- Reduce initial bundle size by ~60%
- Add loading and error states for better UX"
```

---

### Task 2.2: Mobile App SignalingContext Decomposition

**Files:**
- Create: `mobile/src/context/signaling/useWebRTC.ts`
- Create: `mobile/src/context/signaling/useE2EE.ts`
- Create: `mobile/src/context/signaling/useTranslation.ts`
- Create: `mobile/src/context/signaling/useMediaControls.ts`
- Modify: `mobile/src/context/SignalingContext.tsx`
- Test: `mobile/src/context/signaling/__tests__/useWebRTC.test.ts`

- [ ] **Step 1: Write failing test for useWebRTC hook**

```typescript
// mobile/src/context/signaling/__tests__/useWebRTC.test.ts
import { renderHook, act } from '@testing-library/react-hooks';
import { useWebRTC } from '../useWebRTC';

// Mock react-native-webrtc
jest.mock('react-native-webrtc', () => ({
  RTCPeerConnection: jest.fn(() => ({
    createOffer: jest.fn().mockResolvedValue({ type: 'offer', sdp: 'mock-sdp' }),
    createAnswer: jest.fn().mockResolvedValue({ type: 'answer', sdp: 'mock-sdp' }),
    setLocalDescription: jest.fn().mockResolvedValue(undefined),
    setRemoteDescription: jest.fn().mockResolvedValue(undefined),
    addIceCandidate: jest.fn().mockResolvedValue(undefined),
    addEventListener: jest.fn(),
    removeEventListener: jest.fn(),
    close: jest.fn(),
    getStats: jest.fn().mockResolvedValue(new Map()),
  })),
  RTCSessionDescription: jest.fn(),
  RTCIceCandidate: jest.fn(),
  MediaStream: jest.fn(),
}));

describe('useWebRTC', () => {
  it('creates peer connection on mount', () => {
    const { result } = renderHook(() => useWebRTC({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      onOfferCreated: jest.fn(),
      onAnswerCreated: jest.fn(),
      onIceCandidate: jest.fn(),
    }));

    expect(result.current.peerConnection).toBeDefined();
    expect(result.current.localStream).toBeNull();
    expect(result.current.remoteStream).toBeNull();
  });

  it('creates offer successfully', async () => {
    const onOfferCreated = jest.fn();
    
    const { result } = renderHook(() => useWebRTC({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      onOfferCreated,
      onAnswerCreated: jest.fn(),
      onIceCandidate: jest.fn(),
    }));

    await act(async () => {
      await result.current.createOffer();
    });

    expect(onOfferCreated).toHaveBeenCalledWith({
      type: 'offer',
      sdp: 'mock-sdp',
    });
  });

  it('cleans up on unmount', () => {
    const { result, unmount } = renderHook(() => useWebRTC({
      iceServers: [{ urls: 'stun:stun.l.google.com:19302' }],
      onOfferCreated: jest.fn(),
      onAnswerCreated: jest.fn(),
      onIceCandidate: jest.fn(),
    }));

    const peerConnection = result.current.peerConnection;
    
    unmount();

    expect(peerConnection.close).toHaveBeenCalled();
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd mobile && npx jest --testPathPattern='useWebRTC'`
Expected: FAIL with "Cannot find module '../useWebRTC'"

- [ ] **Step 3: Implement useWebRTC hook**

```typescript
// mobile/src/context/signaling/useWebRTC.ts
import { useRef, useEffect, useCallback, useState } from 'react';
import {
  RTCPeerConnection,
  RTCSessionDescription,
  RTCIceCandidate,
  MediaStream,
} from 'react-native-webrtc';

interface WebRTCConfig {
  iceServers: RTCIceServer[];
  onOfferCreated: (offer: RTCSessionDescription) => void;
  onAnswerCreated: (answer: RTCSessionDescription) => void;
  onIceCandidate: (candidate: RTCIceCandidate) => void;
  onRemoteStream?: (stream: MediaStream) => void;
  onConnectionStateChange?: (state: RTCPeerConnectionState) => void;
}

interface WebRTCHook {
  peerConnection: RTCPeerConnection | null;
  localStream: MediaStream | null;
  remoteStream: MediaStream | null;
  connectionState: RTCPeerConnectionState;
  createOffer: () => Promise<void>;
  createAnswer: (offer: RTCSessionDescription) => Promise<void>;
  setRemoteDescription: (desc: RTCSessionDescription) => Promise<void>;
  addIceCandidate: (candidate: RTCIceCandidate) => Promise<void>;
  addLocalStream: (stream: MediaStream) => void;
  removeLocalStream: () => void;
  close: () => void;
}

export function useWebRTC(config: WebRTCConfig): WebRTCHook {
  const peerConnectionRef = useRef<RTCPeerConnection | null>(null);
  const [localStream, setLocalStream] = useState<MediaStream | null>(null);
  const [remoteStream, setRemoteStream] = useState<MediaStream | null>(null);
  const [connectionState, setConnectionState] = useState<RTCPeerConnectionState>('new');

  // Initialize peer connection
  useEffect(() => {
    const pc = new RTCPeerConnection({
      iceServers: config.iceServers,
    });

    // Handle ICE candidates
    pc.addEventListener('icecandidate', (event) => {
      if (event.candidate) {
        config.onIceCandidate(event.candidate);
      }
    });

    // Handle remote stream
    pc.addEventListener('track', (event) => {
      if (event.streams && event.streams[0]) {
        setRemoteStream(event.streams[0]);
        config.onRemoteStream?.(event.streams[0]);
      }
    });

    // Handle connection state changes
    pc.addEventListener('connectionstatechange', () => {
      setConnectionState(pc.connectionState);
      config.onConnectionStateChange?.(pc.connectionState);
    });

    peerConnectionRef.current = pc;

    return () => {
      pc.close();
      peerConnectionRef.current = null;
    };
  }, []);

  const createOffer = useCallback(async () => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    const offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    config.onOfferCreated(offer);
  }, [config.onOfferCreated]);

  const createAnswer = useCallback(async (offer: RTCSessionDescription) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    await pc.setRemoteDescription(offer);
    const answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);
    config.onAnswerCreated(answer);
  }, [config.onAnswerCreated]);

  const setRemoteDescription = useCallback(async (desc: RTCSessionDescription) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    await pc.setRemoteDescription(desc);
  }, []);

  const addIceCandidate = useCallback(async (candidate: RTCIceCandidate) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    await pc.addIceCandidate(candidate);
  }, []);

  const addLocalStream = useCallback((stream: MediaStream) => {
    const pc = peerConnectionRef.current;
    if (!pc) return;

    stream.getTracks().forEach((track) => {
      pc.addTrack(track, stream);
    });
    setLocalStream(stream);
  }, []);

  const removeLocalStream = useCallback(() => {
    const pc = peerConnectionRef.current;
    if (!pc || !localStream) return;

    localStream.getTracks().forEach((track) => {
      track.stop();
      pc.removeTrack(pc.getSenders().find((s) => s.track === track)!);
    });
    setLocalStream(null);
  }, [localStream]);

  const close = useCallback(() => {
    const pc = peerConnectionRef.current;
    if (pc) {
      pc.close();
      peerConnectionRef.current = null;
    }
    setLocalStream(null);
    setRemoteStream(null);
    setConnectionState('closed');
  }, []);

  return {
    peerConnection: peerConnectionRef.current,
    localStream,
    remoteStream,
    connectionState,
    createOffer,
    createAnswer,
    setRemoteDescription,
    addIceCandidate,
    addLocalStream,
    removeLocalStream,
    close,
  };
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd mobile && npx jest --testPathPattern='useWebRTC'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add mobile/src/context/signaling/useWebRTC.ts mobile/src/context/signaling/__tests__/useWebRTC.test.ts
git commit -m "feat(mobile): extract useWebRTC hook from SignalingContext

- Create composable useWebRTC hook for WebRTC peer connection management
- Move peer connection creation and event handling to hook
- Add proper cleanup on unmount
- Add comprehensive tests for hook behavior"
```

---

## Phase 3: Infrastructure Scaling (Weeks 3-4)

### Task 3.1: Nginx Rate Limiting & Upstream Keepalive

**Files:**
- Modify: `infra/nginx.conf`
- Test: Manual verification with curl

- [ ] **Step 1: Write failing test for rate limiting**

```bash
#!/bin/bash
# infra/test-rate-limiting.sh

# Start nginx with rate limiting
docker compose -f infra/docker-compose.yml up -d nginx

# Send 100 requests rapidly
echo "Testing rate limiting..."
for i in {1..100}; do
  response=$(curl -s -o /dev/null -w "%{http_code}" http://localhost/api/v1/health)
  echo "Request $i: $response"
  
  # Check if we hit rate limit (429)
  if [ "$response" == "429" ]; then
    echo "Rate limiting working - got 429 on request $i"
    break
  fi
done

# Verify upstream keepalive
echo "Checking upstream keepalive..."
docker exec nginx cat /var/log/nginx/access.log | grep "keepalive" | tail -5
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash infra/test-rate-limiting.sh`
Expected: FAIL with "No rate limiting applied" or "No keepalive connections"

- [ ] **Step 3: Implement Nginx rate limiting**

```nginx
# infra/nginx.conf
worker_processes auto;
worker_rlimit_nofile 65535;

events {
    worker_connections 4096;
    multi_accept on;
    use epoll;
}

http {
    # Basic settings
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    server_tokens off;
    
    # Rate limiting zones
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=auth:10m rate=5r/m;
    limit_conn_zone $binary_remote_addr zone=conn:10m;
    
    # Logging
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for" '
                    'rt=$request_time';
    
    access_log /var/log/nginx/access.log main;
    error_log /var/log/nginx/error.log warn;
    
    # Gzip compression
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml;
    
    # Upstream with keepalive
    upstream backend {
        server backend:8080;
        keepalive 64;
        keepalive_timeout 60s;
        keepalive_requests 1000;
    }
    
    # Rate limiting for API endpoints
    server {
        listen 80;
        server_name localhost;
        
        # Health endpoint - no rate limit
        location /api/v1/health {
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
        
        # Auth endpoints - strict rate limiting
        location /api/v1/auth/ {
            limit_req zone=auth burst=3 nodelay;
            limit_conn conn 10;
            
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
        }
        
        # API endpoints - moderate rate limiting
        location /api/v1/ {
            limit_req zone=api burst=20 nodelay;
            limit_conn conn 50;
            
            proxy_pass http://backend;
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            
            # Keepalive to upstream
            proxy_http_version 1.1;
            proxy_set_header Connection "";
        }
        
        # WebSocket endpoints
        location /api/v1/ws {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            
            proxy_read_timeout 3600s;
            proxy_send_timeout 3600s;
        }
        
        # Chat WebSocket
        location /api/v1/chat/ws {
            proxy_pass http://backend;
            proxy_http_version 1.1;
            proxy_set_header Upgrade $http_upgrade;
            proxy_set_header Connection "upgrade";
            proxy_set_header Host $host;
            proxy_set_header X-Real-IP $remote_addr;
            proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
            proxy_set_header X-Forwarded-Proto $scheme;
            
            proxy_read_timeout 3600s;
            proxy_send_timeout 3600s;
        }
        
        # Web app (SPA)
        location / {
            root /usr/share/nginx/html;
            index index.html;
            try_files $uri $uri/ /index.html;
            
            # Cache static assets
            location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg)$ {
                expires 1y;
                add_header Cache-Control "public, immutable";
            }
        }
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash infra/test-rate-limiting.sh`
Expected: PASS - Rate limiting returns 429 after burst, upstream keepalive active

- [ ] **Step 5: Commit**

```bash
git add infra/nginx.conf infra/test-rate-limiting.sh
git commit -m "feat(infra): add Nginx rate limiting and upstream keepalive

- Add rate limiting zones: api (10r/s), auth (5r/m)
- Add connection limiting per IP
- Add upstream keepalive with 64 connections
- Add gzip compression for responses
- Add static asset caching with 1-year expiry
- Add WebSocket timeout configuration"
```

---

### Task 3.2: Expand Coturn Port Range

**Files:**
- Modify: `infra/turnserver.conf`
- Modify: `infra/docker-compose.production.yml`
- Test: Manual verification

- [ ] **Step 1: Write failing test for port range**

```bash
#!/bin/bash
# infra/test-turn-ports.sh

# Start coturn with expanded port range
docker compose -f infra/docker-compose.production.yml up -d coturn

# Check listening ports
echo "Checking TURN server ports..."
netstat -an | grep -E "49152|50000" | head -10

# Verify port range is expanded
PORT_COUNT=$(netstat -an | grep -E "49[12][0-9]{2}|50[0-2][0-9]{2}" | wc -l)
echo "Port count: $PORT_COUNT"

if [ "$PORT_COUNT" -lt 100 ]; then
  echo "FAIL: Port range not expanded (got $PORT_COUNT, want 200+)"
  exit 1
fi

echo "PASS: Port range expanded successfully"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `bash infra/test-turn-ports.sh`
Expected: FAIL with "Port range not expanded"

- [ ] **Step 3: Implement expanded port range**

```conf
# infra/turnserver.conf
# Coturn TURN server configuration for production

# Network
listening-port=3478
tls-listening-port=5349
alt-tls-listening-port=443

# Expanded port range for high concurrency (200 ports = 200 concurrent relays)
min-port=49152
max-port=50000

# Authentication
use-auth-secret
static-auth-secret=TURN_SECRET_HERE
realm=allcallall.com

# Logging
log-file=/var/log/coturn/turnserver.log
verbose

# Security
no-multicast-peers
no-cli
no-tlsv1
no-tlsv1_1

# Performance
proc-quota=25
total-quota=1200
stale-nonce=600

# Allowed relay IPs (adjust for your network)
relay-ips=0.0.0.0/0

# Deny private IPs
denied-peer-ip=10.0.0.0-10.255.255.255
denied-peer-ip=172.16.0.0-172.31.255.255
denied-peer-ip=192.168.0.0-192.168.255.255

# Allow public IPs
allowed-peer-ip=0.0.0.0/0
```

```yaml
# infra/docker-compose.production.yml - Update coturn service
services:
  coturn:
    image: coturn/coturn:latest
    ports:
      - "3478:3478"
      - "3478:3478/udp"
      - "5349:5349"
      - "5349:5349/udp"
      - "49152-50000:49152-50000/udp"  # Expanded from 49152-49200
    volumes:
      - ./turnserver.conf:/etc/turnserver.conf
      - turn-logs:/var/log/coturn
    command: ["-c", "/etc/turnserver.conf"]
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "turnutils_uclient", "-T", "localhost"]
      interval: 30s
      timeout: 10s
      retries: 3
```

- [ ] **Step 4: Run test to verify it passes**

Run: `bash infra/test-turn-ports.sh`
Expected: PASS - 200+ ports listening

- [ ] **Step 5: Commit**

```bash
git add infra/turnserver.conf infra/docker-compose.production.yml infra/test-turn-ports.sh
git commit -m "feat(infra): expand Coturn port range for high concurrency

- Expand port range from 48 to 248 ports (49152-50000)
- Support 200+ concurrent TURN relay sessions
- Add health check for TURN server
- Add test script for port range verification"
```

---

## Phase 4: Observability & Monitoring (Weeks 4-5)

### Task 4.1: Prometheus Metrics Middleware

**Files:**
- Create: `backend/internal/metrics/prometheus.go`
- Modify: `backend/internal/server/middleware.go`
- Test: `backend/internal/metrics/prometheus_test.go`

- [ ] **Step 1: Write failing test for Prometheus metrics**

```go
// backend/internal/metrics/prometheus_test.go
package metrics

import (
    "net/http"
    "net/http/httptest"
    "testing"
    
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

func TestPrometheusMetrics_HTTPRequests(t *testing.T) {
    // Setup
    registry := NewPrometheusRegistry()
    gin.SetMode(gin.TestMode)
    
    r := gin.New()
    r.Use(PrometheusMiddleware(registry))
    
    r.GET("/test", func(c *gin.Context) {
        c.JSON(200, gin.H{"message": "ok"})
    })
    
    // Make requests
    for i := 0; i < 10; i++ {
        req := httptest.NewRequest("GET", "/test", nil)
        w := httptest.NewRecorder()
        r.ServeHTTP(w, req)
    }
    
    // Check metrics endpoint
    metricsReq := httptest.NewRequest("GET", "/metrics", nil)
    metricsW := httptest.NewRecorder()
    
    handler := promhttp.HandlerFor(registry.registry, promhttp.HandlerOpts{})
    handler.ServeHTTP(metricsW, metricsReq)
    
    body := metricsW.Body.String()
    
    // Verify metrics are present
    if !strings.Contains(body, "http_requests_total") {
        t.Error("http_requests_total metric not found")
    }
    if !strings.Contains(body, "http_request_duration_seconds") {
        t.Error("http_request_duration_seconds metric not found")
    }
    if !strings.Contains(body, `method="GET"`) {
        t.Error("method label not found")
    }
}

func TestPrometheusMetrics_HistogramBuckets(t *testing.T) {
    registry := NewPrometheusRegistry()
    
    // Verify histogram buckets are configured
    if len(registry.requestDurationBuckets) == 0 {
        t.Error("Histogram buckets not configured")
    }
    
    // Verify buckets cover expected range
    expectedBuckets := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
    if len(registry.requestDurationBuckets) != len(expectedBuckets) {
        t.Errorf("Bucket count = %d, want %d", len(registry.requestDurationBuckets), len(expectedBuckets))
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd backend && go test -v -run 'TestPrometheusMetrics' ./internal/metrics/`
Expected: FAIL with "undefined: PrometheusMiddleware" or "undefined: NewPrometheusRegistry"

- [ ] **Step 3: Implement Prometheus metrics**

```go
// backend/internal/metrics/prometheus.go
package metrics

import (
    "strconv"
    "time"
    
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promauto"
)

type PrometheusRegistry struct {
    registry        *prometheus.Registry
    httpRequests    *prometheus.CounterVec
    httpRequestDuration *prometheus.HistogramVec
    httpRequestSize     *prometheus.HistogramVec
    httpResponseSize    *prometheus.HistogramVec
    requestDurationBuckets []float64
}

func NewPrometheusRegistry() *PrometheusRegistry {
    registry := prometheus.NewRegistry()
    
    requestDurationBuckets := []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}
    
    httpRequests := promauto.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total number of HTTP requests",
        },
        []string{"method", "path", "status"},
    )
    
    httpRequestDuration := promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request duration in seconds",
            Buckets: requestDurationBuckets,
        },
        []string{"method", "path"},
    )
    
    httpRequestSize := promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_size_bytes",
            Help:    "HTTP request size in bytes",
            Buckets: prometheus.ExponentialBuckets(100, 10, 7),
        },
        []string{"method", "path"},
    )
    
    httpResponseSize := promauto.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_response_size_bytes",
            Help:    "HTTP response size in bytes",
            Buckets: prometheus.ExponentialBuckets(100, 10, 7),
        },
        []string{"method", "path"},
    )
    
    return &PrometheusRegistry{
        registry:        registry,
        httpRequests:    httpRequests,
        httpRequestDuration: httpRequestDuration,
        httpRequestSize:     httpRequestSize,
        httpResponseSize:    httpResponseSize,
        requestDurationBuckets: requestDurationBuckets,
    }
}

func PrometheusMiddleware(registry *PrometheusRegistry) gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        
        // Get path template (not actual path to avoid high cardinality)
        path := c.FullPath()
        if path == "" {
            path = "unknown"
        }
        
        // Get request size
        requestSize := c.Request.ContentLength
        if requestSize > 0 {
            registry.httpRequestSize.WithLabelValues(c.Request.Method, path).Observe(float64(requestSize))
        }
        
        // Process request
        c.Next()
        
        // Calculate duration
        duration := time.Since(start).Seconds()
        
        // Get status code
        status := strconv.Itoa(c.Writer.Status())
        
        // Record metrics
        registry.httpRequests.WithLabelValues(c.Request.Method, path, status).Inc()
        registry.httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
        
        // Record response size
        responseSize := float64(c.Writer.Size())
        if responseSize > 0 {
            registry.httpResponseSize.WithLabelValues(c.Request.Method, path).Observe(responseSize)
        }
    }
}

func (r *PrometheusRegistry) Handler() gin.HandlerFunc {
    return func(c *gin.Context) {
        // Use the default prometheus handler
        handler := promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
        handler.ServeHTTP(c.Writer, c.Request)
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd backend && go test -v -run 'TestPrometheusMetrics' ./internal/metrics/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add backend/internal/metrics/prometheus.go backend/internal/metrics/prometheus_test.go
git commit -m "feat(metrics): add Prometheus metrics middleware for HTTP requests

- Track total requests by method, path, status
- Track request duration with histogram buckets
- Track request/response sizes
- Use path templates to avoid high cardinality
- Add /metrics endpoint for Prometheus scraping"
```

---

## Phase 5: Load Testing & Validation (Weeks 5-6)

### Task 5.1: k6 Load Test Suite

**Files:**
- Create: `scripts/load/k6/smoke.js`
- Create: `scripts/load/k6/load.js`
- Create: `scripts/load/k6/stress.js`
- Create: `scripts/load/k6/README.md`

- [ ] **Step 1: Write smoke test**

```javascript
// scripts/load/k6/smoke.js
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  vus: 1,
  duration: '30s',
  thresholds: {
    http_req_duration: ['p(95)<500'],
    http_req_failed: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.TOKEN || '';

export default function () {
  const headers = TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {};
  
  // Health check
  const healthRes = http.get(`${BASE_URL}/api/v1/health`, { headers });
  check(healthRes, {
    'health status is 200': (r) => r.status === 200,
    'health response time < 100ms': (r) => r.timings.duration < 100,
  });
  
  sleep(1);
  
  // Get messages (if authenticated)
  if (TOKEN) {
    const messagesRes = http.get(`${BASE_URL}/api/v1/conversations/1/messages?limit=20`, { headers });
    check(messagesRes, {
      'messages status is 200': (r) => r.status === 200,
      'messages response time < 500ms': (r) => r.timings.duration < 500,
    });
  }
  
  sleep(1);
}
```

- [ ] **Step 2: Write load test**

```javascript
// scripts/load/k6/load.js
import http from 'k6/http';
import { check, sleep, group } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const messageDuration = new Trend('message_duration');

export const options = {
  stages: [
    { duration: '2m', target: 100 },  // Ramp up to 100 users
    { duration: '5m', target: 100 },  // Stay at 100 users
    { duration: '2m', target: 200 },  // Ramp up to 200 users
    { duration: '5m', target: 200 },  // Stay at 200 users
    { duration: '2m', target: 0 },    // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<1000'],
    http_req_failed: ['rate<0.05'],
    errors: ['rate<0.05'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.TOKEN || '';

export default function () {
  const headers = TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {};
  
  group('Health Check', () => {
    const res = http.get(`${BASE_URL}/api/v1/health`, { headers });
    check(res, {
      'health status is 200': (r) => r.status === 200,
    });
  });
  
  group('Get Messages', () => {
    const res = http.get(`${BASE_URL}/api/v1/conversations/1/messages?limit=50`, { headers });
    messageDuration.add(res.timings.duration);
    
    check(res, {
      'messages status is 200': (r) => r.status === 200,
      'messages response time < 500ms': (r) => r.timings.duration < 500,
    });
    
    errorRate.add(res.status !== 200);
  });
  
  group('Post Message', () => {
    const payload = JSON.stringify({
      content: `Load test message ${Date.now()}`,
    });
    
    const res = http.post(`${BASE_URL}/api/v1/conversations/1/messages`, payload, {
      headers: {
        ...headers,
        'Content-Type': 'application/json',
      },
    });
    
    check(res, {
      'post message status is 201': (r) => r.status === 201,
      'post message response time < 1000ms': (r) => r.timings.duration < 1000,
    });
    
    errorRate.add(res.status !== 201);
  });
  
  sleep(1);
}
```

- [ ] **Step 3: Write stress test**

```javascript
// scripts/load/k6/stress.js
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';

const errorRate = new Rate('errors');
const agentRunDuration = new Trend('agent_run_duration');

export const options = {
  stages: [
    { duration: '1m', target: 50 },   // Ramp up
    { duration: '3m', target: 50 },   // Stay at 50
    { duration: '1m', target: 100 },  // Ramp up
    { duration: '3m', target: 100 },  // Stay at 100
    { duration: '1m', target: 150 },  // Ramp up to stress
    { duration: '5m', target: 150 },  // Stress test
    { duration: '2m', target: 0 },    // Ramp down
  ],
  thresholds: {
    http_req_duration: ['p(95)<2000'],
    http_req_failed: ['rate<0.1'],
    errors: ['rate<0.1'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.TOKEN || '';

export default function () {
  const headers = TOKEN ? { Authorization: `Bearer ${TOKEN}` } : {};
  
  // Mix of read and write operations
  const operations = [
    { name: 'Get Messages', weight: 0.6 },
    { name: 'Post Message', weight: 0.3 },
    { name: 'Agent Run', weight: 0.1 },
  ];
  
  const rand = Math.random();
  let cumulative = 0;
  
  for (const op of operations) {
    cumulative += op.weight;
    if (rand <= cumulative) {
      executeOperation(op.name, headers);
      break;
    }
  }
  
  sleep(0.5);
}

function executeOperation(name, headers) {
  switch (name) {
    case 'Get Messages':
      const messagesRes = http.get(`${BASE_URL}/api/v1/conversations/1/messages?limit=50`, { headers });
      check(messagesRes, {
        'messages status is 200': (r) => r.status === 200,
      });
      errorRate.add(messagesRes.status !== 200);
      break;
      
    case 'Post Message':
      const payload = JSON.stringify({
        content: `Stress test message ${Date.now()}`,
      });
      const postRes = http.post(`${BASE_URL}/api/v1/conversations/1/messages`, payload, {
        headers: { ...headers, 'Content-Type': 'application/json' },
      });
      check(postRes, {
        'post status is 201': (r) => r.status === 201,
      });
      errorRate.add(postRes.status !== 201);
      break;
      
    case 'Agent Run':
      const agentPayload = JSON.stringify({
        conversation_id: '1',
        prompt: 'Summarize recent messages',
      });
      const agentRes = http.post(`${BASE_URL}/api/v1/agent/runs`, agentPayload, {
        headers: { ...headers, 'Content-Type': 'application/json' },
      });
      agentRunDuration.add(agentRes.timings.duration);
      check(agentRes, {
        'agent run status is 202': (r) => r.status === 202,
      });
      errorRate.add(agentRes.status !== 202);
      break;
  }
}
```

- [ ] **Step 4: Run smoke test**

Run: `cd scripts/load/k6 && k6 run smoke.js`
Expected: PASS - All thresholds met

- [ ] **Step 5: Commit**

```bash
git add scripts/load/k6/
git commit -m "feat(testing): add k6 load test suite for high concurrency validation

- Add smoke test for basic functionality verification
- Add load test for sustained 100-200 user traffic
- Add stress test for 150 user peak load
- Add custom metrics for message and agent run duration
- Add threshold configuration for pass/fail criteria"
```

---

## Phase 6: Mobile App Optimization (Weeks 3-4)

### Task 6.1: Exponential Backoff Reconnection

**Files:**
- Modify: `mobile/src/api/signaling.ts`
- Test: `mobile/src/api/__tests__/signaling.test.ts`

- [ ] **Step 1: Write failing test for exponential backoff**

```typescript
// mobile/src/api/__tests__/signaling.test.ts
import { SignalingClient } from '../signaling';

// Mock WebSocket
jest.mock('ws', () => {
  return jest.fn().mockImplementation(() => ({
    on: jest.fn(),
    send: jest.fn(),
    close: jest.fn(),
    readyState: 1,
  }));
});

describe('SignalingClient Exponential Backoff', () => {
  let client: SignalingClient;
  let mockWebSocket: any;
  
  beforeEach(() => {
    jest.useFakeTimers();
    client = new SignalingClient({
      url: 'ws://localhost:8080/api/v1/ws',
      token: 'test-token',
    });
    mockWebSocket = (client as any).ws;
  });
  
  afterEach(() => {
    jest.useRealTimers();
  });
  
  it('uses exponential backoff on reconnect', async () => {
    const connectSpy = jest.spyOn(client, 'connect');
    
    // Simulate connection
    client.connect();
    
    // Simulate disconnect
    mockWebSocket.on.mock.calls.find(
      (call: any) => call[0] === 'close'
    )[1]();
    
    // First reconnect attempt (1s delay)
    jest.advanceTimersByTime(1000);
    expect(connectSpy).toHaveBeenCalledTimes(2);
    
    // Simulate another disconnect
    mockWebSocket.on.mock.calls.find(
      (call: any) => call[0] === 'close'
    )[1]();
    
    // Second reconnect attempt (2s delay)
    jest.advanceTimersByTime(2000);
    expect(connectSpy).toHaveBeenCalledTimes(3);
    
    // Simulate another disconnect
    mockWebSocket.on.mock.calls.find(
      (call: any) => call[0] === 'close'
    )[1]();
    
    // Third reconnect attempt (4s delay)
    jest.advanceTimersByTime(4000);
    expect(connectSpy).toHaveBeenCalledTimes(4);
  });
  
  it('caps backoff at maximum delay', async () => {
    const connectSpy = jest.spyOn(client, 'connect');
    
    client.connect();
    
    // Simulate 10 disconnects
    for (let i = 0; i < 10; i++) {
      mockWebSocket.on.mock.calls.find(
        (call: any) => call[0] === 'close'
      )[1]();
      
      jest.advanceTimersByTime(30000); // Max delay
    }
    
    // Should have 11 total connections (1 initial + 10 reconnects)
    expect(connectSpy).toHaveBeenCalledTimes(11);
  });
  
  it('resets backoff on successful connection', async () => {
    const connectSpy = jest.spyOn(client, 'connect');
    
    client.connect();
    
    // Simulate disconnect
    mockWebSocket.on.mock.calls.find(
      (call: any) => call[0] === 'close'
    )[1]();
    
    // Wait for first reconnect
    jest.advanceTimersByTime(1000);
    
    // Simulate successful connection (message received)
    mockWebSocket.on.mock.calls.find(
      (call: any) => call[0] === 'message'
    )[1](JSON.stringify({ type: 'connected' }));
    
    // Simulate another disconnect
    mockWebSocket.on.mock.calls.find(
      (call: any) => call[0] === 'close'
    )[1]();
    
    // Should use 1s delay again (backoff reset)
    jest.advanceTimersByTime(1000);
    expect(connectSpy).toHaveBeenCalledTimes(3);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd mobile && npx jest --testPathPattern='signaling'`
Expected: FAIL with "Cannot find module '../signaling'" or test failures

- [ ] **Step 3: Implement exponential backoff**

```typescript
// mobile/src/api/signaling.ts - Update SignalingClient

interface ExponentialBackoffConfig {
  initialDelay: number;
  maxDelay: number;
  multiplier: number;
  jitter: boolean;
}

class ExponentialBackoff {
  private delay: number;
  private attempt: number = 0;
  private config: ExponentialBackoffConfig;
  
  constructor(config: Partial<ExponentialBackoffConfig> = {}) {
    this.config = {
      initialDelay: config.initialDelay || 1000,
      maxDelay: config.maxDelay || 30000,
      multiplier: config.multiplier || 2,
      jitter: config.jitter !== undefined ? config.jitter : true,
    };
    this.delay = this.config.initialDelay;
  }
  
  next(): number {
    const currentDelay = this.delay;
    
    // Apply jitter (±20%)
    let delayWithJitter = currentDelay;
    if (this.config.jitter) {
      const jitterRange = currentDelay * 0.2;
      delayWithJitter = currentDelay + (Math.random() * 2 - 1) * jitterRange;
    }
    
    // Increase delay for next attempt
    this.delay = Math.min(
      this.delay * this.config.multiplier,
      this.config.maxDelay
    );
    this.attempt++;
    
    return Math.max(0, delayWithJitter);
  }
  
  reset(): void {
    this.delay = this.config.initialDelay;
    this.attempt = 0;
  }
  
  getAttempt(): number {
    return this.attempt;
  }
}

export class SignalingClient {
  private ws: WebSocket | null = null;
  private url: string;
  private token: string;
  private backoff: ExponentialBackoff;
  private reconnectTimer: NodeJS.Timeout | null = null;
  private shouldReconnect: boolean = true;
  
  constructor(config: { url: string; token: string }) {
    this.url = config.url;
    this.token = config.token;
    this.backoff = new ExponentialBackoff();
  }
  
  connect(): void {
    if (this.ws) {
      this.ws.close();
    }
    
    this.ws = new WebSocket(`${this.url}?token=${this.token}`);
    
    this.ws.onopen = () => {
      console.log('[Signaling] Connected');
      this.backoff.reset(); // Reset backoff on successful connection
    };
    
    this.ws.onclose = (event) => {
      console.log('[Signaling] Disconnected', event.code, event.reason);
      
      if (this.shouldReconnect) {
        this.scheduleReconnect();
      }
    };
    
    this.ws.onerror = (error) => {
      console.error('[Signaling] Error:', error);
    };
    
    this.ws.onmessage = (event) => {
      // Handle incoming messages
      const data = JSON.parse(event.data);
      this.handleMessage(data);
    };
  }
  
  private scheduleReconnect(): void {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
    }
    
    const delay = this.backoff.next();
    console.log(`[Signaling] Reconnecting in ${delay}ms (attempt ${this.backoff.getAttempt()})`);
    
    this.reconnectTimer = setTimeout(() => {
      this.connect();
    }, delay);
  }
  
  private handleMessage(data: any): void {
    // Reset backoff on any successful message
    this.backoff.reset();
    
    // Handle different message types
    switch (data.type) {
      case 'connected':
        console.log('[Signaling] Connection confirmed');
        break;
      case 'offer':
      case 'answer':
      case 'ice-candidate':
        // Forward to WebRTC handler
        break;
      default:
        console.log('[Signaling] Unknown message type:', data.type);
    }
  }
  
  disconnect(): void {
    this.shouldReconnect = false;
    
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }
  
  send(data: any): void {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(data));
    }
  }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd mobile && npx jest --testPathPattern='signaling'`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add mobile/src/api/signaling.ts mobile/src/api/__tests__/signaling.test.ts
git commit -m "feat(mobile): add exponential backoff for WebSocket reconnection

- Implement ExponentialBackoff class with configurable parameters
- Add jitter to prevent thundering herd problem
- Reset backoff on successful connection
- Cap maximum delay at 30 seconds
- Add comprehensive tests for backoff behavior"
```

---

## Summary

This implementation plan covers:

1. **Backend Performance** (Weeks 1-2): Connection pooling, batch outbox claiming, sliding window rate limiting, Redis caching
2. **Frontend Optimization** (Weeks 2-3): Web code splitting, mobile SignalingContext decomposition
3. **Infrastructure Scaling** (Weeks 3-4): Nginx rate limiting, Coturn port expansion
4. **Observability** (Weeks 4-5): Prometheus metrics middleware
5. **Load Testing** (Weeks 5-6): k6 test suite for validation
6. **Mobile Optimization** (Weeks 3-4): Exponential backoff reconnection

**Expected Outcomes:**
- Support 1000+ concurrent users
- Handle 10,000+ messages per minute
- Process 100+ Agent runs per minute
- Reduce p95 latency by 40%+
- Achieve 99.9% uptime under load

**Success Criteria:**
- All k6 load tests pass with defined thresholds
- No connection pool exhaustion under load
- Rate limiting prevents abuse without blocking legitimate traffic
- Reconnection works reliably with exponential backoff
- Metrics provide visibility into system performance
