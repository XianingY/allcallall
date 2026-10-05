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
	sqlDBWaitCount   *prometheus.GaugeVec
	redisPoolConns   *prometheus.GaugeVec
	redisPoolWait    *prometheus.GaugeVec
	redisPoolHits    *prometheus.GaugeVec
	redisPoolTimeout *prometheus.GaugeVec
	redisPoolStale   *prometheus.GaugeVec
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
		sqlDBWaitCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "sql_db_wait_count_total",
			Help: "Cumulative count of connection wait events in the SQL database pool.",
		}, []string{"pool"}),
		redisPoolConns: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "redis_pool_connections",
			Help: "Current Redis connection pool statistics.",
		}, []string{"pool", "state"}),
		redisPoolWait: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "redis_pool_wait_total",
			Help: "Cumulative count of connection pool waits (misses) in the Redis pool.",
		}, []string{"pool"}),
		redisPoolHits: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "redis_pool_hits_total",
			Help: "Cumulative count of connection pool hits in the Redis pool.",
		}, []string{"pool"}),
		redisPoolTimeout: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "redis_pool_timeouts_total",
			Help: "Cumulative count of connection pool timeout events in the Redis pool.",
		}, []string{"pool"}),
		redisPoolStale: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "redis_pool_stale_total",
			Help: "Cumulative count of stale connections removed from the Redis pool.",
		}, []string{"pool"}),
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
// gauges.  pool is bounded to allowedPoolNames.
func (pc *PerformanceCollectors) UpdateSQLDBStats(pool string, stats sql.DBStats) {
	p := BoundedLabel(pool, allowedPoolNames, "other")
	pc.sqlDBConns.WithLabelValues(p, "open").Set(float64(stats.OpenConnections))
	pc.sqlDBConns.WithLabelValues(p, "in_use").Set(float64(stats.InUse))
	pc.sqlDBConns.WithLabelValues(p, "idle").Set(float64(stats.Idle))
	pc.sqlDBWaitCount.WithLabelValues(p).Set(float64(stats.WaitCount))
}

// UpdateRedisPoolStats pushes a snapshot of redis.PoolStats into the Prometheus
// gauges.  Nil stats is a no-op.  pool is bounded to allowedPoolNames.
func (pc *PerformanceCollectors) UpdateRedisPoolStats(pool string, stats *redis.PoolStats) {
	if stats == nil {
		return
	}
	p := BoundedLabel(pool, allowedPoolNames, "other")
	pc.redisPoolConns.WithLabelValues(p, "total").Set(float64(stats.TotalConns))
	pc.redisPoolConns.WithLabelValues(p, "idle").Set(float64(stats.IdleConns))
	pc.redisPoolConns.WithLabelValues(p, "active").Set(float64(stats.TotalConns - stats.IdleConns))
	pc.redisPoolWait.WithLabelValues(p).Set(float64(stats.Misses))
	pc.redisPoolHits.WithLabelValues(p).Set(float64(stats.Hits))
	pc.redisPoolTimeout.WithLabelValues(p).Set(float64(stats.Timeouts))
	pc.redisPoolStale.WithLabelValues(p).Set(float64(stats.StaleConns))
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
