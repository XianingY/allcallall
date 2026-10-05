package metrics

import (
	"database/sql"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

func TestPerformanceCollectorsExposeBoundedMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	collectors.ObserveOutboxQueueWait("agent", 250*time.Millisecond)
	collectors.ObserveOutboxEvent("agent", "success", 500*time.Millisecond)
	collectors.SetOutboxInflight("agent", 2)
	collectors.ObserveAgentContext(40*time.Millisecond, 4, 30, 4096, 900)
	collectors.ObserveDependency("agent_runtime", "run", "success", 2*time.Second)
	collectors.UpdateSQLDBStats("primary", sql.DBStats{OpenConnections: 5, InUse: 3, Idle: 2, WaitCount: 7})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{TotalConns: 5, IdleConns: 2, Misses: 7, Timeouts: 1})

	assertMetricExists(t, registry, "outbox_queue_wait_seconds")
	assertMetricExists(t, registry, "agent_context_query_count")
	assertMetricExists(t, registry, "dependency_request_duration_seconds")
	assertMetricExists(t, registry, "sql_db_connections")
	assertMetricExists(t, registry, "redis_pool_connections")
	assertMetricExists(t, registry, "redis_pool_wait_total")
}

func TestBoundedLabel(t *testing.T) {
	allowed := map[string]struct{}{
		"agent": {}, "search": {}, "other": {},
	}

	if got := BoundedLabel("agent", allowed, "other"); got != "agent" {
		t.Errorf("BoundedLabel(agent) = %s, want agent", got)
	}
	if got := BoundedLabel("search", allowed, "other"); got != "search" {
		t.Errorf("BoundedLabel(search) = %s, want search", got)
	}
	if got := BoundedLabel("unknown", allowed, "other"); got != "other" {
		t.Errorf("BoundedLabel(unknown) = %s, want other", got)
	}
	if got := BoundedLabel("", allowed, "other"); got != "other" {
		t.Errorf("BoundedLabel(empty) = %s, want other", got)
	}
}

func TestBoundedLabelWorkClasses(t *testing.T) {
	for _, wc := range []string{"agent", "collaboration", "search", "transcription", "settlement"} {
		if got := BoundedLabel(wc, allowedWorkClasses, "other"); got != wc {
			t.Errorf("BoundedLabel(%s) = %s, want %s", wc, got, wc)
		}
	}
	if got := BoundedLabel("unbounded_id_123", allowedWorkClasses, "other"); got != "other" {
		t.Errorf("BoundedLabel(unbounded) = %s, want other", got)
	}
}

func TestUpdateRedisPoolStatsNilIsNoOp(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	// Should not panic or register any labels.
	collectors.UpdateRedisPoolStats("primary", nil)

	// Verify no redis_pool_connections labels were created.
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "redis_pool_connections" {
		t.Errorf("expected no redis_pool_connections samples after nil stats, got %d", len(mf.GetMetric()))
		}
	}
}

func TestUpdateSQLDBStatsBoundedPool(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	// Unknown pool name should be mapped to "other".
	collectors.UpdateSQLDBStats("my_regional_db", sql.DBStats{OpenConnections: 3, InUse: 1, Idle: 2, WaitCount: 0})

	assertMetricExists(t, registry, "sql_db_connections")
}

// assertMetricExists checks that at least one metric family with the given name
// has been registered and has a sample after the test exercised the collectors.
func assertMetricExists(t *testing.T, registry *prometheus.Registry, name string) {
	t.Helper()
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
		if len(mf.GetMetric()) > 0 {
			return
		}
		t.Fatalf("metric %s exists but has no samples", name)
		}
	}
	t.Fatalf("metric %s not found in registry", name)
}
