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

	// CounterVec metrics require two snapshots to produce a delta; the first
	// call only establishes the baseline.
	collectors.UpdateSQLDBStats("primary", sql.DBStats{OpenConnections: 5, InUse: 3, Idle: 2, WaitCount: 7})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{TotalConns: 5, IdleConns: 2, Misses: 7, Timeouts: 1})
	// Second snapshot produces the delta so counters appear in Gather().
	collectors.UpdateSQLDBStats("primary", sql.DBStats{OpenConnections: 5, InUse: 3, Idle: 2, WaitCount: 10})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{TotalConns: 5, IdleConns: 2, Misses: 10, Hits: 3, Timeouts: 2, StaleConns: 1})

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

func TestDeltaFirstSnapshotIsZero(t *testing.T) {
	prev := make(map[string]float64)
	if d := delta(prev, "x", 100); d != 0 {
		t.Errorf("first delta = %v, want 0", d)
	}
	if prev["x"] != 100 {
		t.Errorf("prev[x] = %v, want 100 (baseline stored)", prev["x"])
	}
}

func TestDeltaSubsequentSnapshot(t *testing.T) {
	prev := map[string]float64{"x": 100}
	if d := delta(prev, "x", 150); d != 50 {
		t.Errorf("delta = %v, want 50", d)
	}
	if prev["x"] != 150 {
		t.Errorf("prev[x] = %v, want 150", prev["x"])
	}
}

func TestDeltaResetEmitsCurrentValue(t *testing.T) {
	prev := map[string]float64{"x": 200}
	// External accumulator reset: new value is lower than previous.
	if d := delta(prev, "x", 30); d != 30 {
		t.Errorf("delta on reset = %v, want 30", d)
	}
	if prev["x"] != 30 {
		t.Errorf("prev[x] after reset = %v, want 30", prev["x"])
	}
	// Subsequent normal delta after reset.
	if d := delta(prev, "x", 60); d != 30 {
		t.Errorf("delta after reset = %v, want 30", d)
	}
}

func TestCumulativeCounterDeltaTracking(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	// First snapshot: counters should stay at zero (baseline established).
	collectors.UpdateSQLDBStats("primary", sql.DBStats{WaitCount: 100})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{Misses: 50, Hits: 200, Timeouts: 5, StaleConns: 3})

	assertCounterValue(t, registry, "sql_db_wait_count_total", "primary", 0)
	assertCounterValue(t, registry, "redis_pool_wait_total", "primary", 0)
	assertCounterValue(t, registry, "redis_pool_hits_total", "primary", 0)
	assertCounterValue(t, registry, "redis_pool_timeouts_total", "primary", 0)
	assertCounterValue(t, registry, "redis_pool_stale_total", "primary", 0)

	// Second snapshot: counters should increase by the delta.
	collectors.UpdateSQLDBStats("primary", sql.DBStats{WaitCount: 150})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{Misses: 70, Hits: 250, Timeouts: 8, StaleConns: 4})

	assertCounterValue(t, registry, "sql_db_wait_count_total", "primary", 50)
	assertCounterValue(t, registry, "redis_pool_wait_total", "primary", 20)
	assertCounterValue(t, registry, "redis_pool_hits_total", "primary", 50)
	assertCounterValue(t, registry, "redis_pool_timeouts_total", "primary", 3)
	assertCounterValue(t, registry, "redis_pool_stale_total", "primary", 1)

	// Third snapshot: counters should accumulate further.
	collectors.UpdateSQLDBStats("primary", sql.DBStats{WaitCount: 210})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{Misses: 100, Hits: 300, Timeouts: 10, StaleConns: 6})

	assertCounterValue(t, registry, "sql_db_wait_count_total", "primary", 110) // 50 + 60
	assertCounterValue(t, registry, "redis_pool_wait_total", "primary", 50)    // 20 + 30
	assertCounterValue(t, registry, "redis_pool_hits_total", "primary", 100)   // 50 + 50
	assertCounterValue(t, registry, "redis_pool_timeouts_total", "primary", 5) // 3 + 2
	assertCounterValue(t, registry, "redis_pool_stale_total", "primary", 3)    // 1 + 2
}

func TestCumulativeCounterResetHandling(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	// Establish baseline.
	collectors.UpdateSQLDBStats("primary", sql.DBStats{WaitCount: 100})
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{Misses: 50, Hits: 200, Timeouts: 5, StaleConns: 3})

	// Normal delta.
	collectors.UpdateSQLDBStats("primary", sql.DBStats{WaitCount: 150})
	assertCounterValue(t, registry, "sql_db_wait_count_total", "primary", 50)

	// Simulate external accumulator reset (e.g., driver pool recreated).
	// New value is lower than previous -> treat as reset, emit new value.
	collectors.UpdateSQLDBStats("primary", sql.DBStats{WaitCount: 30})
	assertCounterValue(t, registry, "sql_db_wait_count_total", "primary", 80) // 50 + 30

	// Subsequent normal delta after reset.
	collectors.UpdateSQLDBStats("primary", sql.DBStats{WaitCount: 60})
	assertCounterValue(t, registry, "sql_db_wait_count_total", "primary", 110) // 80 + (60 - 30)

	// Redis reset test.
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{Misses: 70, Hits: 250, Timeouts: 8, StaleConns: 4})
	assertCounterValue(t, registry, "redis_pool_wait_total", "primary", 20) // 70 - 50

	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{Misses: 10, Hits: 5, Timeouts: 0, StaleConns: 0}) // reset
	assertCounterValue(t, registry, "redis_pool_wait_total", "primary", 30)                                       // 20 + 10
	assertCounterValue(t, registry, "redis_pool_hits_total", "primary", 55)                                       // 50 + 5

	// Timeouts and stale: delta=0 on reset (0 < 8 and 0 < 4, so emit 0; but
	// we skip Add(0), so counters stay at their previous values).
	assertCounterValue(t, registry, "redis_pool_timeouts_total", "primary", 3) // unchanged: 3 + 0 (skipped)
	assertCounterValue(t, registry, "redis_pool_stale_total", "primary", 1)    // unchanged: 1 + 0 (skipped)

	// Normal delta after Redis reset.
	collectors.UpdateRedisPoolStats("primary", &redis.PoolStats{Misses: 25, Hits: 20, Timeouts: 2, StaleConns: 1})
	assertCounterValue(t, registry, "redis_pool_wait_total", "primary", 45)    // 30 + (25 - 10)
	assertCounterValue(t, registry, "redis_pool_hits_total", "primary", 70)    // 55 + (20 - 5)
	assertCounterValue(t, registry, "redis_pool_timeouts_total", "primary", 5) // 3 + 2
	assertCounterValue(t, registry, "redis_pool_stale_total", "primary", 2)    // 1 + 1
}

func TestUpdateRedisPoolStatsNilIsNoOp(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	// Should not panic or register any labels.
	collectors.UpdateRedisPoolStats("primary", nil)

	// Verify no redis_pool_connections or counter labels were created.
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		name := mf.GetName()
		if name == "redis_pool_connections" || name == "redis_pool_wait_total" {
			t.Errorf("expected no %q samples after nil stats, got %d", name, len(mf.GetMetric()))
		}
	}
}

func TestUpdateSQLDBStatsBoundedPool(t *testing.T) {
	registry := prometheus.NewRegistry()
	collectors := NewPerformanceCollectors(registry)

	// Unknown pool name should be mapped to "other".
	collectors.UpdateSQLDBStats("my_regional_db", sql.DBStats{OpenConnections: 3, InUse: 1, Idle: 2, WaitCount: 0})
	collectors.UpdateSQLDBStats("my_regional_db", sql.DBStats{OpenConnections: 3, InUse: 1, Idle: 2, WaitCount: 5})

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

// assertCounterValue checks that the Counter metric with the given name and
// pool label has the expected cumulative value.  If expected is 0 and the
// metric has no observations (not present in Gather output), the assertion
// passes -- a Counter with no Add calls produces no samples.
func assertCounterValue(t *testing.T, registry *prometheus.Registry, name, pool string, expected float64) {
	t.Helper()
	mfs, err := registry.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			for _, m := range mf.GetMetric() {
				for _, label := range m.GetLabel() {
					if label.GetName() == "pool" && label.GetValue() == pool {
						got := m.GetCounter().GetValue()
						if got != expected {
							t.Errorf("metric %s{pool=%q} = %v, want %v", name, pool, got, expected)
						}
						return
					}
				}
			}
			// Label combination not found.
			if expected == 0 {
				// Counter with no observations is absent from output; that is 0.
				return
			}
			t.Fatalf("metric %s with pool=%q not found among %d samples", name, pool, len(mf.GetMetric()))
		}
	}
	// Metric family not found.
	if expected == 0 {
		return
	}
	t.Fatalf("metric %s not found in registry", name)
}
