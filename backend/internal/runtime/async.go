package runtime

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/alerting"
	"github.com/allcallall/backend/internal/async"
	"github.com/allcallall/backend/internal/metrics"
)

// AsyncPoolsFromEnv builds the shared background job pools.
//
// Every pool is bounded: queue full => job dropped and counted (never an
// unbounded goroutine), terminal failures => dead-letter callback => P2 alert.
// This replaces the previous per-event `go func()` calls which could spawn
// thousands of concurrent goroutines and discard errors entirely.
func AsyncPoolsFromEnv(logger zerolog.Logger, counters metrics.Recorder, alerter *alerting.Service) []*async.Pool {
	deadLetter := func(ctx context.Context, r async.Result) {
		if alerter == nil {
			return
		}
		if err := alerter.Emit(ctx, alerting.Alert{
			Severity: alerting.SeverityP2,
			Title:    "background job dead-lettered",
			Detail:   r.Err.Error(),
			Labels: map[string]string{
				"component": "async",
				"kind":      r.Job.Kind,
			},
		}); err != nil {
			logger.Warn().Err(err).Msg("failed to emit dead-letter alert")
		}
	}

	moderation := async.NewPool(async.Options{
		Name:        "moderation",
		Workers:     envInt("ASYNC_MODERATION_WORKERS", 8),
		QueueSize:   envInt("ASYNC_MODERATION_QUEUE", 512),
		MaxAttempts: envInt("ASYNC_MODERATION_ATTEMPTS", 2),
		RetryDelay:  time.Second,
		JobTimeout:  5 * time.Second,
		Logger:      logger,
		Metrics:     counters,
		// 审核是增强项，重试耗尽只告警，绝不阻断消息投递。
		OnDeadLetter: deadLetter,
	})

	ragIndex := async.NewPool(async.Options{
		Name:        "rag-index",
		Workers:     envInt("ASYNC_RAG_INDEX_WORKERS", 8),
		QueueSize:   envInt("ASYNC_RAG_INDEX_QUEUE", 1024),
		MaxAttempts: envInt("ASYNC_RAG_INDEX_ATTEMPTS", 3),
		RetryDelay:  2 * time.Second,
		JobTimeout:  15 * time.Second,
		Logger:      logger,
		Metrics:     counters,
		// 索引失败会导致召回静默缺失，必须可被观测到。
		OnDeadLetter: deadLetter,
	})

	return []*async.Pool{moderation, ragIndex}
}

// PoolByName returns the pool with the given name, or nil when absent so the
// caller's service can fall back to its legacy behaviour.
func PoolByName(pools []*async.Pool, name string) *async.Pool {
	for _, p := range pools {
		if p != nil && p.Name() == name {
			return p
		}
	}
	return nil
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	if v, err := strconv.Atoi(raw); err == nil && v > 0 {
		return v
	}
	return fallback
}
