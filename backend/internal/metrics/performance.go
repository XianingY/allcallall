package metrics

import (
	"database/sql"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"
)

// durationBuckets covers sub-millisecond to two-minute latencies.  Task 3's
// periodic sampling and the outbox/dependency paths both emit into these
// histograms; the upper bound (120 s) keeps the bucket list finite even if a
// dependency call blocks for a long time.
var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 120}

// allowedWorkClasses is the bounded label set for outbox work_class.  Unknown
// values fall back to "other" via BoundedLabel.
var allowedWorkClasses = map[string]struct{}{
	"agent": {}, "collaboration": {}, "search": {}, "transcription": {}, "settlement": {}, "other": {},
}

// allowedDependencyServices is the bounded label set for dependency service names.
var allowedDependencyServices = map[string]struct{}{
	"agent_runtime": {}, "rag_runtime": {}, "sandbox": {}, "mysql": {}, "redis": {}, "other": {},
}

// allowedDependencyOperations is the bounded label set for dependency operation names.
var allowedDependencyOperations = map[string]struct{}{
	"run": {}, "query": {}, "invoke": {}, "read": {}, "write": {}, "other": {},
}

// allowedDependencyStatuses is the bounded label set for dependency status values.
var allowedDependencyStatuses = map[string]struct{}{
	"success": {}, "error": {}, "timeout": {}, "other": {},
}

// allowedOutboxOutcomes is the bounded label set for outbox event outcomes.
var allowedOutboxOutcomes = map[string]struct{}{
	"success": {}, "failure": {}, "retry": {}, "dead_letter": {}, "other": {},
}

// allowedPoolNames is the bounded label set for SQL and Redis pool names.
var allowedPoolNames = map[string]struct{}{
	"primary": {}, "replica": {}, "other": {},
}

// BoundedLabel returns raw if it is present in allowed; otherwise fallback.
// This prevents unbounded cardinality from user or resource IDs leaking into
// Prometheus labels.
func BoundedLabel(raw string, allowed map[string]struct{}, fallback string) string {
	if _, ok := allowed[raw]; ok {
		return raw
	}
	return fallback
}

// delta computes the non-negative increment for an external cumulative counter.
// prev is a map from label key to the previous observed value; current is the
// new snapshot value.
//
// First-snapshot behavior: if prev[key] does not exist, delta stores current as
// the baseline and returns 0.  The Counter therefore starts at zero and only
// increments on the second and subsequent snapshots, avoiding a false
// full-history jump on process start.
//
// Reset behavior: if current < prev[key], the external accumulator has reset
// (e.g., Redis server restart).  delta treats the new value as the amount
// accumulated since the reset and returns current.  prev[key] is updated to
// current so subsequent deltas are computed against the new baseline.
func delta(prev map[string]float64, key string, current float64) float64 {
	p, ok := prev[key]
	prev[key] = current
	if !ok {
		// First observation: record baseline, emit nothing.
		return 0
	}
	if current < p {
		// External accumulator reset.  Emit the value accumulated since
		// the reset (treat as if prev was zero).
		return current
	}
	return current - p
}

// PerformanceCollectors holds all backend-pressure Prometheus metrics.  Use
// NewPerformanceCollectors to create an instance with constructor injection
// (for tests), or the package-level functions to record through the
// process-default collector.
type PerformanceCollectors struct {
	outboxQueueWait  *prometheus.HistogramVec
	outboxEventDur   *prometheus.HistogramVec
	outboxInflight   *prometheus.GaugeVec
	agentCtxDuration prometheus.Histogram
	agentCtxCount    prometheus.Counter
	agentCtxEntries  prometheus.Histogram
	agentCtxChunks   prometheus.Histogram
	agentCtxMaxTok   prometheus.Gauge
	agentCtxUsedTok  prometheus.Gauge
	dependencyDur    *prometheus.HistogramVec
	sqlDBConns       *prometheus.GaugeVec
	sqlDBWaitCount   *prometheus.CounterVec
	redisPoolConns   *prometheus.GaugeVec
	redisPoolWait    *prometheus.CounterVec
	redisPoolHits    *prometheus.CounterVec
	redisPoolTimeout *prometheus.CounterVec
	redisPoolStale   *prometheus.CounterVec

	// prevMu protects the previous-value maps used for delta tracking on
	// cumulative external counters (the CounterVec fields above).  Each map
	// stores the last observed absolute value keyed by the bounded pool name.
	prevMu              sync.Mutex
	prevSQLWaitCount    map[string]float64
	prevRedisMisses     map[string]float64
	prevRedisHits       map[string]float64
	prevRedisTimeouts   map[string]float64
	prevRedisStaleConns map[string]float64
}

// NewPerformanceCollectors creates and registers all backend-pressure metrics
// on the supplied registerer.  Pass prometheus.NewRegistry() in tests to avoid
// colliding with the process-default registry; the package-level functions
// use a default collector registered on prometheus.DefaultRegisterer.
func NewPerformanceCollectors(registerer prometheus.Registerer) *PerformanceCollectors {
	c := &PerformanceCollectors{
		outboxQueueWait: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "outbox_queue_wait_seconds",
			Help:    "Time an outbox event spent waiting in the queue before processing started.",
			Buckets: durationBuckets,
		}, []string{"work_class"}),
		outboxEventDur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "outbox_event_duration_seconds",
			Help:    "End-to-end processing duration of an outbox event.",
			Buckets: durationBuckets,
		}, []string{"work_class", "outcome"}),
		outboxInflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "outbox_inflight",
			Help: "Number of outbox events currently being processed.",
		}, []string{"work_class"}),
		agentCtxDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "agent_context_query_duration_seconds",
			Help:    "Duration of agent context window queries.",
			Buckets: durationBuckets,
		}),
		agentCtxCount: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "agent_context_query_count",
			Help: "Total number of agent context window queries.",
		}),
		agentCtxEntries: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "agent_context_entries",
			Help:    "Number of context entries retrieved per query.",
			Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128},
		}),
		agentCtxChunks: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "agent_context_chunks",
			Help:    "Number of chunks loaded per context query.",
			Buckets: []float64{1, 5, 10, 25, 50, 100, 250, 500},
		}),
		agentCtxMaxTok: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "agent_context_window_max_tokens",
			Help: "Maximum token capacity of the agent context window.",
		}),
		agentCtxUsedTok: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "agent_context_window_used_tokens",
			Help: "Tokens currently used in the agent context window.",
		}),
		dependencyDur: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "dependency_request_duration_seconds",
			Help:    "Duration of outbound dependency requests.",
			Buckets: durationBuckets,
		}, []string{"service", "operation", "status"}),
		sqlDBConns: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sql_db_connections",
			Help: "Current SQL database connection pool statistics.",
		}, []string{"pool", "state"}),
		sqlDBWaitCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "sql_db_wait_count_total",
			Help: "Cumulative count of connection wait events in the SQL database pool, exported as delta Counter.",
		}, []string{"pool"}),
		redisPoolConns: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "redis_pool_connections",
			Help: "Current Redis connection pool statistics.",
		}, []string{"pool", "state"}),
		redisPoolWait: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "redis_pool_wait_total",
			Help: "Cumulative count of connection pool waits (Misses in go-redis PoolStats), exported as delta Counter.",
		}, []string{"pool"}),
		redisPoolHits: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "redis_pool_hits_total",
			Help: "Cumulative count of connection pool hits in the Redis pool, exported as delta Counter.",
		}, []string{"pool"}),
		redisPoolTimeout: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "redis_pool_timeouts_total",
			Help: "Cumulative count of connection pool timeout events in the Redis pool, exported as delta Counter.",
		}, []string{"pool"}),
		redisPoolStale: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "redis_pool_stale_total",
			Help: "Cumulative count of stale connections removed from the Redis pool, exported as delta Counter.",
		}, []string{"pool"}),
		prevSQLWaitCount:    make(map[string]float64),
		prevRedisMisses:     make(map[string]float64),
		prevRedisHits:       make(map[string]float64),
		prevRedisTimeouts:   make(map[string]float64),
		prevRedisStaleConns: make(map[string]float64),
	}

	registerer.MustRegister(
		c.outboxQueueWait,
		c.outboxEventDur,
		c.outboxInflight,
		c.agentCtxDuration,
		c.agentCtxCount,
		c.agentCtxEntries,
		c.agentCtxChunks,
		c.agentCtxMaxTok,
		c.agentCtxUsedTok,
		c.dependencyDur,
		c.sqlDBConns,
		c.sqlDBWaitCount,
		c.redisPoolConns,
		c.redisPoolWait,
		c.redisPoolHits,
		c.redisPoolTimeout,
		c.redisPoolStale,
	)

	return c
}

// --- Collector methods -------------------------------------------------------

// ObserveOutboxQueueWait records the time an outbox event waited before
// processing started.  work_class is bounded to allowedWorkClasses.
func (pc *PerformanceCollectors) ObserveOutboxQueueWait(workClass string, wait time.Duration) {
	pc.outboxQueueWait.WithLabelValues(BoundedLabel(workClass, allowedWorkClasses, "other")).Observe(wait.Seconds())
}

// ObserveOutboxEvent records the end-to-end processing duration of an outbox
// event.  Both work_class and outcome are bounded.
func (pc *PerformanceCollectors) ObserveOutboxEvent(workClass, outcome string, duration time.Duration) {
	wc := BoundedLabel(workClass, allowedWorkClasses, "other")
	oc := BoundedLabel(outcome, allowedOutboxOutcomes, "other")
	pc.outboxEventDur.WithLabelValues(wc, oc).Observe(duration.Seconds())
}

// SetOutboxInflight sets the number of outbox events currently being processed.
// work_class is bounded.
func (pc *PerformanceCollectors) SetOutboxInflight(workClass string, n int) {
	pc.outboxInflight.WithLabelValues(BoundedLabel(workClass, allowedWorkClasses, "other")).Set(float64(n))
}

// ObserveAgentContext records an agent context window query: its duration,
// number of entries and chunks retrieved, and the window's max and used token
// counts.
func (pc *PerformanceCollectors) ObserveAgentContext(duration time.Duration, entries, chunks, maxTokens, usedTokens int) {
	pc.agentCtxDuration.Observe(duration.Seconds())
	pc.agentCtxCount.Inc()
	pc.agentCtxEntries.Observe(float64(entries))
	pc.agentCtxChunks.Observe(float64(chunks))
	pc.agentCtxMaxTok.Set(float64(maxTokens))
	pc.agentCtxUsedTok.Set(float64(usedTokens))
}

// ObserveDependency records the duration of an outbound dependency request.
// All three labels (service, operation, status) are bounded.
func (pc *PerformanceCollectors) ObserveDependency(service, operation, status string, duration time.Duration) {
	s := BoundedLabel(service, allowedDependencyServices, "other")
	o := BoundedLabel(operation, allowedDependencyOperations, "other")
	st := BoundedLabel(status, allowedDependencyStatuses, "other")
	pc.dependencyDur.WithLabelValues(s, o, st).Observe(duration.Seconds())
}

// UpdateSQLDBStats pushes a snapshot of sql.DBStats into the Prometheus
// metrics.  pool is bounded to allowedPoolNames.
//
// Connection counts (open, in_use, idle) are current-level gauges.
// WaitCount is a cumulative counter from the driver; this method computes the
// delta since the last snapshot and adds it to the CounterVec, so the
// Prometheus counter only increases and rate() works correctly across process
// restarts.
//
// First-snapshot behavior: WaitCount is recorded as the baseline but no
// counter increment is emitted, so the Counter starts at zero.
// Reset behavior: if the new WaitCount is lower than the previous value
// (driver pool was recreated), the new value is emitted as the delta.
func (pc *PerformanceCollectors) UpdateSQLDBStats(pool string, stats sql.DBStats) {
	p := BoundedLabel(pool, allowedPoolNames, "other")
	pc.sqlDBConns.WithLabelValues(p, "open").Set(float64(stats.OpenConnections))
	pc.sqlDBConns.WithLabelValues(p, "in_use").Set(float64(stats.InUse))
	pc.sqlDBConns.WithLabelValues(p, "idle").Set(float64(stats.Idle))

	pc.prevMu.Lock()
	d := delta(pc.prevSQLWaitCount, p, float64(stats.WaitCount))
	pc.prevMu.Unlock()
	if d > 0 {
		pc.sqlDBWaitCount.WithLabelValues(p).Add(d)
	}
}

// UpdateRedisPoolStats pushes a snapshot of redis.PoolStats into the Prometheus
// metrics.  Nil stats is a no-op.  pool is bounded to allowedPoolNames.
//
// Connection counts (total, idle, active) are current-level gauges.
// Misses, Hits, Timeouts, and StaleConns are cumulative counters from the
// go-redis pool; this method computes deltas since the last snapshot and adds
// them to CounterVecs, so the Prometheus counters only increase and rate()
// works correctly across process restarts.
//
// redis_pool_wait_total is sourced from PoolStats.Misses (the count of times
// a free connection was NOT found in the pool, which is when callers wait).
//
// First-snapshot behavior: values are recorded as baselines but no counter
// increments are emitted.
// Reset behavior: if a new value is lower than the previous value (Redis server
// restarted and counters reset), the new value is emitted as the delta.
func (pc *PerformanceCollectors) UpdateRedisPoolStats(pool string, stats *redis.PoolStats) {
	if stats == nil {
		return
	}
	p := BoundedLabel(pool, allowedPoolNames, "other")
	pc.redisPoolConns.WithLabelValues(p, "total").Set(float64(stats.TotalConns))
	pc.redisPoolConns.WithLabelValues(p, "idle").Set(float64(stats.IdleConns))
	pc.redisPoolConns.WithLabelValues(p, "active").Set(float64(stats.TotalConns - stats.IdleConns))

	pc.prevMu.Lock()
	dw := delta(pc.prevRedisMisses, p, float64(stats.Misses))
	dh := delta(pc.prevRedisHits, p, float64(stats.Hits))
	dt := delta(pc.prevRedisTimeouts, p, float64(stats.Timeouts))
	ds := delta(pc.prevRedisStaleConns, p, float64(stats.StaleConns))
	pc.prevMu.Unlock()

	if dw > 0 {
		pc.redisPoolWait.WithLabelValues(p).Add(dw)
	}
	if dh > 0 {
		pc.redisPoolHits.WithLabelValues(p).Add(dh)
	}
	if dt > 0 {
		pc.redisPoolTimeout.WithLabelValues(p).Add(dt)
	}
	if ds > 0 {
		pc.redisPoolStale.WithLabelValues(p).Add(ds)
	}
}

// --- Package-level functions (process-default collector) ---------------------

var (
	defaultCollectors     *PerformanceCollectors
	defaultCollectorsOnce sync.Once
)

func initDefaultCollectors() {
	defaultCollectorsOnce.Do(func() {
		defaultCollectors = NewPerformanceCollectors(prometheus.DefaultRegisterer)
	})
}

// ObserveOutboxQueueWait records the queue wait through the process-default collector.
func ObserveOutboxQueueWait(workClass string, wait time.Duration) {
	initDefaultCollectors()
	defaultCollectors.ObserveOutboxQueueWait(workClass, wait)
}

// ObserveOutboxEvent records the event duration through the process-default collector.
func ObserveOutboxEvent(workClass, outcome string, duration time.Duration) {
	initDefaultCollectors()
	defaultCollectors.ObserveOutboxEvent(workClass, outcome, duration)
}

// SetOutboxInflight sets the inflight count through the process-default collector.
func SetOutboxInflight(workClass string, n int) {
	initDefaultCollectors()
	defaultCollectors.SetOutboxInflight(workClass, n)
}

// ObserveAgentContext records an agent context query through the process-default collector.
func ObserveAgentContext(duration time.Duration, entries, chunks, maxTokens, usedTokens int) {
	initDefaultCollectors()
	defaultCollectors.ObserveAgentContext(duration, entries, chunks, maxTokens, usedTokens)
}

// ObserveDependency records a dependency request through the process-default collector.
func ObserveDependency(service, operation, status string, duration time.Duration) {
	initDefaultCollectors()
	defaultCollectors.ObserveDependency(service, operation, status, duration)
}

// UpdateSQLDBStats pushes sql.DBStats through the process-default collector.
func UpdateSQLDBStats(pool string, stats sql.DBStats) {
	initDefaultCollectors()
	defaultCollectors.UpdateSQLDBStats(pool, stats)
}

// UpdateRedisPoolStats pushes redis.PoolStats through the process-default collector.
func UpdateRedisPoolStats(pool string, stats *redis.PoolStats) {
	initDefaultCollectors()
	defaultCollectors.UpdateRedisPoolStats(pool, stats)
}
