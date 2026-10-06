package events

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/alerting"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

var ErrOutboxHandlerNotFound = errors.New("outbox handler not found")

// ErrLeaseConflict is used as a context cancellation cause when a handler's
// lease is lost to another worker mid-execution.
var ErrLeaseConflict = errors.New("outbox lease conflict: ownership lost during processing")

type Handler func(ctx context.Context, event models.EventOutbox) error

// ProcessorConfig configures bounded parallel processing for the outbox
// processor. When Concurrency is 0 or 1, the processor falls back to
// sequential processing identical to the pre-parallel behavior.
type ProcessorConfig struct {
	BatchSize    int
	Concurrency  int
	QueueDepth   int
	IdleInterval time.Duration
	ErrorBackoff time.Duration
	Lease        time.Duration
	LeaseRefresh time.Duration
}

// ProcessBatchResult holds the outcome of a single ProcessBatch call.
// Handler failures successfully persisted as retry or dead outcomes are
// counted in Retried/Dead respectively and do not appear in Errors; only
// claim failures (returned as the batch error) and state-transition database
// failures (collected in Errors) are considered processor errors.
type ProcessBatchResult struct {
	Claimed   int     // events claimed from the store
	Succeeded int     // events published successfully
	Retried   int     // events moved to retry (handler failed, under max attempts)
	Dead      int     // events moved to dead-letter (handler failed, max attempts reached)
	Errors    []error // state-transition database failures
}

type processOutcome int

const (
	outcomePublished processOutcome = iota
	outcomeRetried
	outcomeDead
)

type outboxMetricScope struct {
	workClass string
	startedAt time.Time
}

// eventResult captures the outcome of processing a single event.
type eventResult struct {
	row        models.EventOutbox
	outcome    processOutcome
	handlerErr error // the handler error (nil for success)
}

// OrderingKey returns the aggregate ordering key for an outbox row:
// "aggregate_type:aggregate_id". Events sharing the same ordering key
// are dispatched to the same shard and processed sequentially, preserving
// per-aggregate FIFO order across replicas.
func OrderingKey(row models.EventOutbox) string {
	return fmt.Sprintf("%s:%d", row.AggregateType, row.AggregateID)
}

// concurrencyObserverFunc is a test-seam function called when handler
// concurrency changes. Production code never sets it. Tests call
// SetConcurrencyObserver to install one and receive a cleanup function.
type concurrencyObserverFunc func(delta int64, current int64)

var globalConcurrencyObserver atomic.Pointer[concurrencyObserverFunc]

// SetConcurrencyObserver installs a test observer for handler concurrency
// and returns a cleanup function that restores the previous (nil) state.
// This is the production-safe replacement for package-level test variables;
// it does not import "testing" and is safe for parallel test execution.
func SetConcurrencyObserver(fn concurrencyObserverFunc) func() {
	globalConcurrencyObserver.Store(&fn)
	return func() { globalConcurrencyObserver.Store(nil) }
}

func observeConcurrency(delta int64, current int64) {
	if fn := globalConcurrencyObserver.Load(); fn != nil {
		(*fn)(delta, current)
	}
}

// outboxWorkClass maps an event to one of the fixed worker-oriented label
// values. The Prometheus collector also bounds the final label, so unknown
// event names can never create an unbounded label series.
func outboxWorkClass(row models.EventOutbox) string {
	event := row.Event
	switch {
	case strings.HasPrefix(event, "agent."), strings.HasPrefix(event, "workflow."), strings.HasPrefix(event, "mcp."):
		return "agent"
	case strings.HasPrefix(event, "search."), strings.HasPrefix(event, "rag."):
		return "search"
	case strings.HasPrefix(event, "recording."), strings.HasPrefix(event, "meeting.transcription."):
		return "transcription"
	case strings.HasPrefix(event, "settlement."):
		return "settlement"
	case event == "message.created", strings.HasPrefix(event, "chat."), strings.HasPrefix(event, "conversation."), strings.HasPrefix(event, "room."), strings.HasPrefix(event, "weekly_task."):
		return "collaboration"
	default:
		return "other"
	}
}

func (p *Processor) beginOutboxMetricScope(row models.EventOutbox) outboxMetricScope {
	workClass := outboxWorkClass(row)
	queueWait := time.Since(row.CreatedAt)
	if queueWait < 0 {
		queueWait = 0
	}
	metrics.ObserveOutboxQueueWait(workClass, queueWait)
	return outboxMetricScope{workClass: workClass, startedAt: time.Now()}
}

func (s outboxMetricScope) finish(outcome processOutcome) {
	metrics.ObserveOutboxEvent(s.workClass, outboxMetricLabel(outcome), time.Since(s.startedAt))
}

func outboxMetricLabel(outcome processOutcome) string {
	switch outcome {
	case outcomePublished:
		return "success"
	case outcomeRetried:
		return "retry"
	case outcomeDead:
		return "dead_letter"
	default:
		return "other"
	}
}

type Processor struct {
	store                 *Store
	handlers              map[string]Handler
	events                []string
	orderedEvents         []string
	metrics               metrics.Recorder
	logger                zerolog.Logger
	alerter               *alerting.Service
	batchSize             int
	maxAttempts           int
	retryDelay            time.Duration
	workerID              string
	lease                 time.Duration
	leaseRefresh          time.Duration
	idleInterval          time.Duration
	errorBackoff          time.Duration
	backlogSampleInterval time.Duration
	concurrency           int
	queueDepth            int
	// currentConcurrency is scoped to this Processor so embedded and standalone
	// workers cannot observe each other's in-flight handler count.
	currentConcurrency atomic.Int64
	mu                 sync.RWMutex
}

func NewProcessor(store *Store, recorders ...metrics.Recorder) *Processor {
	var metrics metrics.Recorder
	if len(recorders) > 0 {
		metrics = recorders[0]
	}
	return &Processor{
		store:                 store,
		handlers:              make(map[string]Handler),
		metrics:               metrics,
		logger:                zerolog.Nop(),
		batchSize:             100,
		maxAttempts:           3,
		retryDelay:            time.Minute,
		workerID:              "outbox-" + uuid.NewString(),
		lease:                 2 * time.Minute,
		idleInterval:          30 * time.Second,
		errorBackoff:          5 * time.Second,
		backlogSampleInterval: 10 * time.Second,
		concurrency:           1,
		queueDepth:            2,
	}
}

// WithLogger injects a logger so batch-level failures are visible in logs.
func (p *Processor) WithLogger(logger zerolog.Logger) *Processor {
	p.logger = logger
	return p
}

// WithAlerter routes batch-level failures to the on-call alerting pipeline.
func (p *Processor) WithAlerter(svc *alerting.Service) *Processor {
	p.alerter = svc
	return p
}

func (p *Processor) Register(event string, handler Handler) {
	if event == "" || handler == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.handlers[event] = handler
}

func (p *Processor) WithBatchSize(size int) {
	if size > 0 {
		p.batchSize = size
	}
}

func (p *Processor) WithRetry(maxAttempts int, delay time.Duration) {
	if maxAttempts > 0 {
		p.maxAttempts = maxAttempts
	}
	if delay > 0 {
		p.retryDelay = delay
	}
}

func (p *Processor) WithWorker(workerID string, lease time.Duration) {
	if workerID != "" {
		p.workerID = workerID
	}
	if lease > 0 {
		p.lease = lease
	}
}

func (p *Processor) WithEventFilter(events ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = normalizedEvents(events)
}

// WithOrderedEvents sets the event types that require aggregate-order
// processing. Events in this list will only be claimed when no earlier
// pending row exists for the same aggregate, preventing cross-replica
// reordering. Events not in this list (e.g. idempotent indexing events)
// bypass the ordering check for higher concurrency.
func (p *Processor) WithOrderedEvents(events ...string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.orderedEvents = normalizedEvents(events)
}

// OrderedEvents returns a copy of the event types requiring aggregate-order
// processing. It is primarily useful for deployment wiring tests.
func (p *Processor) OrderedEvents() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]string(nil), p.orderedEvents...)
}

// WithConfig applies all ProcessorConfig fields to the processor. Zero or
// negative values are ignored so the caller can set only the fields it cares
// about. Concurrency is clamped to [1, 64]; QueueDepth is clamped to
// [concurrency, 1024]. LeaseRefresh defaults to lease/3 when zero.
func (p *Processor) WithConfig(config ProcessorConfig) *Processor {
	if config.BatchSize > 0 {
		p.batchSize = config.BatchSize
	}
	if config.Concurrency > 0 {
		p.concurrency = config.Concurrency
	}
	if p.concurrency < 1 {
		p.concurrency = 1
	}
	if p.concurrency > 64 {
		p.concurrency = 64
	}
	if config.QueueDepth > 0 {
		p.queueDepth = config.QueueDepth
	}
	if p.queueDepth < p.concurrency {
		p.queueDepth = p.concurrency
	}
	if p.queueDepth > 1024 {
		p.queueDepth = 1024
	}
	if config.IdleInterval > 0 {
		p.idleInterval = config.IdleInterval
	}
	if config.ErrorBackoff > 0 {
		p.errorBackoff = config.ErrorBackoff
	}
	if config.Lease > 0 {
		p.lease = config.Lease
	}
	if config.LeaseRefresh > 0 {
		p.leaseRefresh = config.LeaseRefresh
	} else if p.lease > 0 && p.leaseRefresh <= 0 {
		p.leaseRefresh = p.lease / 3
	}
	return p
}

// ProcessOnce processes one batch of pending outbox events and returns the
// number of events whose state transition succeeded (published, retried, or
// dead-lettered). It is a compatibility wrapper around ProcessBatch for
// callers that have not yet migrated.
func (p *Processor) ProcessOnce(ctx context.Context) (int, error) {
	if p == nil || p.store == nil {
		return 0, errors.New("outbox processor store is nil")
	}
	if p.metrics != nil {
		if backlog, countErr := p.store.CountPendingForEvents(ctx, p.eventFilter()); countErr == nil {
			p.metrics.Set("outbox_backlog", backlog)
		}
	}
	result, err := p.ProcessBatch(ctx)
	if err != nil {
		return 0, err
	}
	var firstErr error
	if len(result.Errors) > 0 {
		firstErr = result.Errors[0]
	}
	return result.Succeeded + result.Retried + result.Dead, firstErr
}

// ProcessBatch claims and processes a batch of pending outbox events.
// When concurrency > 1, events are dispatched to sharded worker queues
// keyed by OrderingKey so that events for the same aggregate are processed
// sequentially while different aggregates run concurrently. When concurrency
// is 1 (the default), processing is sequential and identical to the
// pre-parallel behavior.
func (p *Processor) ProcessBatch(ctx context.Context) (ProcessBatchResult, error) {
	if p == nil || p.store == nil {
		return ProcessBatchResult{}, errors.New("outbox processor store is nil")
	}
	events := p.eventFilter()
	orderedEvents := p.orderedEventFilter()

	var rows []models.EventOutbox
	var err error
	if len(orderedEvents) > 0 {
		rows, err = p.store.ClaimPendingForEventsOrdered(ctx, p.batchSize, p.workerID, p.lease, events, orderedEvents)
	} else {
		rows, err = p.store.ClaimPendingForEvents(ctx, p.batchSize, p.workerID, p.lease, events)
	}
	if err != nil {
		return ProcessBatchResult{}, err
	}

	var result ProcessBatchResult
	result.Claimed = len(rows)

	if len(rows) == 0 {
		return result, nil
	}

	// When concurrency is 1, process sequentially (preserves exact legacy
	// behavior including per-row state transitions).
	if p.concurrency <= 1 {
		for _, row := range rows {
			outcome, eventErr := p.processEventWithLease(ctx, row)
			if eventErr != nil {
				result.Errors = append(result.Errors, eventErr)
				continue
			}
			switch outcome {
			case outcomePublished:
				result.Succeeded++
			case outcomeRetried:
				result.Retried++
			case outcomeDead:
				result.Dead++
			}
		}
		return result, nil
	}

	// Parallel dispatch: shard events by OrderingKey so that events for the
	// same aggregate are processed sequentially on the same shard while
	// different aggregates run concurrently.
	results := p.dispatchParallel(ctx, rows)
	p.persistResults(ctx, results, &result)
	return result, nil
}

// dispatchParallel shards events by OrderingKey and processes them
// concurrently across Concurrency shards. Each shard processes its events
// sequentially, preserving per-aggregate FIFO order.
func (p *Processor) dispatchParallel(ctx context.Context, rows []models.EventOutbox) []eventResult {
	shardCount := p.concurrency
	shardCap := p.queueDepth / shardCount
	if shardCap < 1 {
		shardCap = 1
	}

	shards := make([]chan models.EventOutbox, shardCount)
	for i := range shards {
		shards[i] = make(chan models.EventOutbox, shardCap)
	}

	resultCh := make(chan eventResult, len(rows))
	var wg sync.WaitGroup

	for i := 0; i < shardCount; i++ {
		wg.Add(1)
		go func(shard chan models.EventOutbox) {
			defer wg.Done()
			for row := range shard {
				res := p.processEventOutcome(ctx, row)
				resultCh <- res
			}
		}(shards[i])
	}

	for _, row := range rows {
		key := OrderingKey(row)
		shardIdx := shardIndex(key, shardCount)
		shards[shardIdx] <- row
	}

	go func() {
		for _, shard := range shards {
			close(shard)
		}
	}()

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	var results []eventResult
	for r := range resultCh {
		results = append(results, r)
	}
	return results
}

// shardIndex maps an ordering key to a shard index using FNV-1a hash.
func shardIndex(key string, shardCount int) int {
	if shardCount <= 0 || shardCount > math.MaxInt32 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shardCount)) // #nosec G115 -- shardCount is bounded to int32
}

// processEventOutcome dispatches a single event to its handler and returns
// the outcome without persisting the state transition. The caller is
// responsible for persisting the outcome (either individually or in a batch).
func (p *Processor) processEventOutcome(ctx context.Context, row models.EventOutbox) eventResult {
	metricScope := p.beginOutboxMetricScope(row)
	var result eventResult
	defer func() {
		metricScope.finish(result.outcome)
	}()

	handler := p.lookup(row.Event)
	handlerErr := ErrOutboxHandlerNotFound
	if handler != nil {
		handlerCtx := trace.WithOutboxID(trace.WithRequestID(ctx, row.RequestID), row.ID)
		handlerCtx, span := trace.StartSpan(handlerCtx, "outbox.process_event", map[string]string{
			"event":          row.Event,
			"aggregate_type": row.AggregateType,
			"aggregate_id":   strconv.FormatUint(row.AggregateID, 10),
			"outbox_id":      strconv.FormatUint(row.ID, 10),
		})

		handlerErr = p.runWithLeaseRefresh(handlerCtx, row, func(runCtx context.Context) error {
			cur := p.currentConcurrency.Add(1)
			observeConcurrency(1, cur)
			metrics.SetOutboxInflight(metricScope.workClass, int(cur))
			err := handler(runCtx, row)
			cur = p.currentConcurrency.Add(-1)
			observeConcurrency(-1, cur)
			metrics.SetOutboxInflight(metricScope.workClass, int(cur))
			return err
		})

		span.End(handlerErr)
	}

	if handlerErr == nil {
		if p.metrics != nil {
			p.metrics.Inc("outbox_publish_total")
		}
		result = eventResult{row: row, outcome: outcomePublished, handlerErr: nil}
		return result
	}

	if row.Attempts+1 >= p.maxAttempts {
		if p.metrics != nil {
			p.metrics.Inc("outbox_dead_letter_total")
		}
		if p.alerter != nil {
			if alertErr := p.alerter.Emit(context.Background(), alerting.Alert{
				Severity: alerting.SeverityP1,
				Title:    "outbox event moved to dead-letter",
				Detail:   handlerErr.Error(),
				Labels: map[string]string{
					"worker":         p.workerID,
					"component":      "outbox",
					"event":          row.Event,
					"outbox_id":      strconv.FormatUint(row.ID, 10),
					"aggregate_type": row.AggregateType,
					"aggregate_id":   strconv.FormatUint(row.AggregateID, 10),
				},
			}); alertErr != nil {
				p.logger.Warn().Err(alertErr).Uint64("outbox_id", row.ID).
					Msg("failed to emit outbox dead-letter alert")
			}
		}
		result = eventResult{row: row, outcome: outcomeDead, handlerErr: handlerErr}
		return result
	}

	if p.metrics != nil {
		p.metrics.Inc("outbox_publish_retry_total")
	}
	result = eventResult{row: row, outcome: outcomeRetried, handlerErr: handlerErr}
	return result
}

// runWithLeaseRefresh executes fn with optional lease-refresh protection.
// When leaseRefresh > 0, a background goroutine periodically extends the
// lease while fn runs. If ownership is lost (ExtendLease returns false),
// the context passed to fn is cancelled with ErrLeaseConflict as the cause.
// When leaseRefresh <= 0, fn is called with the original context unchanged.
func (p *Processor) runWithLeaseRefresh(ctx context.Context, row models.EventOutbox, fn func(context.Context) error) error {
	if p.leaseRefresh <= 0 {
		return fn(ctx)
	}

	refreshCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(p.leaseRefresh)
		defer ticker.Stop()
		for {
			select {
			case <-refreshCtx.Done():
				return
			case <-ticker.C:
				until := time.Now().UTC().Add(p.lease)
				ok, refreshErr := p.store.ExtendLease(refreshCtx, row.ID, p.workerID, until)
				if refreshErr != nil {
					p.logger.Warn().Err(refreshErr).Uint64("outbox_id", row.ID).
						Msg("lease refresh failed")
					continue
				}
				if !ok {
					if p.metrics != nil {
						p.metrics.Inc("outbox_lease_conflict_total")
					}
					p.logger.Warn().Uint64("outbox_id", row.ID).
						Msg("lease conflict: ownership lost during processing, cancelling handler")
					cancel(ErrLeaseConflict)
					return
				}
			}
		}
	}()

	handlerErr := fn(refreshCtx)

	cancel(nil)
	<-done

	return handlerErr
}

// processEventWithLease processes a single event with lease refresh for the
// sequential (concurrency=1) path. It delegates to runWithLeaseRefresh for
// the lease-refresh lifecycle and processEvent for handler invocation and
// state persistence.
func (p *Processor) processEventWithLease(ctx context.Context, row models.EventOutbox) (processOutcome, error) {
	var outcome processOutcome
	var eventErr error
	handlerErr := p.runWithLeaseRefresh(ctx, row, func(runCtx context.Context) error {
		outcome, eventErr = p.processEvent(runCtx, row)
		return eventErr
	})
	_ = handlerErr // eventErr is already captured; handlerErr is the same or a lease-cancel error
	return outcome, eventErr
}

// persistResults batch-persists event outcomes. It groups results by outcome
// type and uses batch methods for groups larger than one, falling back to
// single-row methods for mismatches or groups of one.
func (p *Processor) persistResults(ctx context.Context, results []eventResult, batchResult *ProcessBatchResult) {
	var published, retried, dead []eventResult
	for _, r := range results {
		switch r.outcome {
		case outcomePublished:
			published = append(published, r)
		case outcomeRetried:
			retried = append(retried, r)
		case outcomeDead:
			dead = append(dead, r)
		}
	}
	p.persistPublished(ctx, published, batchResult)
	p.persistRetried(ctx, retried, batchResult)
	p.persistDead(ctx, dead, batchResult)
}

// persistPublished batch-persishes published events.
func (p *Processor) persistPublished(ctx context.Context, results []eventResult, batchResult *ProcessBatchResult) {
	if len(results) == 0 {
		return
	}
	if len(results) == 1 {
		if err := p.store.MarkPublished(ctx, results[0].row.ID); err != nil {
			batchResult.Errors = append(batchResult.Errors, err)
		} else {
			batchResult.Succeeded++
		}
		return
	}
	ids := make([]uint64, len(results))
	for i, r := range results {
		ids[i] = r.row.ID
	}
	mismatched, err := p.store.MarkPublishedBatch(ctx, ids, p.workerID)
	if err != nil {
		for _, r := range results {
			if perErr := p.store.MarkPublished(ctx, r.row.ID); perErr != nil {
				batchResult.Errors = append(batchResult.Errors, perErr)
			} else {
				batchResult.Succeeded++
			}
		}
		return
	}
	batchResult.Succeeded += len(ids) - len(mismatched)
	for _, id := range mismatched {
		if perErr := p.store.MarkPublished(ctx, id); perErr != nil {
			p.incFinalStateError("published")
			batchResult.Errors = append(batchResult.Errors, perErr)
		} else {
			batchResult.Succeeded++
		}
	}
}

// persistRetried batch-persishes retry events, grouping by (availableAt, error).
func (p *Processor) persistRetried(ctx context.Context, results []eventResult, batchResult *ProcessBatchResult) {
	if len(results) == 0 {
		return
	}
	type retryGroup struct {
		availableAt time.Time
		errMsg      string
		ids         []uint64
	}
	groups := make(map[string]*retryGroup)
	for _, r := range results {
		availableAt := time.Now().UTC().Add(p.retryDelay)
		errMsg := ""
		if r.handlerErr != nil {
			errMsg = r.handlerErr.Error()
		}
		key := fmt.Sprintf("%v:%s", availableAt.Round(time.Millisecond), errMsg)
		g, ok := groups[key]
		if !ok {
			g = &retryGroup{availableAt: availableAt, errMsg: errMsg}
			groups[key] = g
		}
		g.ids = append(g.ids, r.row.ID)
	}
	for _, g := range groups {
		if len(g.ids) == 1 {
			if err := p.store.MarkRetry(ctx, g.ids[0], errors.New(g.errMsg), g.availableAt); err != nil {
				batchResult.Errors = append(batchResult.Errors, err)
			} else {
				batchResult.Retried++
			}
			continue
		}
		mismatched, err := p.store.MarkRetryBatch(ctx, g.ids, p.workerID, errors.New(g.errMsg), g.availableAt)
		if err != nil {
			for _, id := range g.ids {
				if perErr := p.store.MarkRetry(ctx, id, errors.New(g.errMsg), g.availableAt); perErr != nil {
					batchResult.Errors = append(batchResult.Errors, perErr)
				} else {
					batchResult.Retried++
				}
			}
			continue
		}
		batchResult.Retried += len(g.ids) - len(mismatched)
		for _, id := range mismatched {
			if perErr := p.store.MarkRetry(ctx, id, errors.New(g.errMsg), g.availableAt); perErr != nil {
				p.incFinalStateError("retry")
				batchResult.Errors = append(batchResult.Errors, perErr)
			} else {
				batchResult.Retried++
			}
		}
	}
}

// persistDead batch-persishes dead-letter events, grouping by normalized error.
func (p *Processor) persistDead(ctx context.Context, results []eventResult, batchResult *ProcessBatchResult) {
	if len(results) == 0 {
		return
	}
	type deadGroup struct {
		errMsg string
		ids    []uint64
	}
	groups := make(map[string]*deadGroup)
	for _, r := range results {
		errMsg := ""
		if r.handlerErr != nil {
			errMsg = r.handlerErr.Error()
		}
		g, ok := groups[errMsg]
		if !ok {
			g = &deadGroup{errMsg: errMsg}
			groups[errMsg] = g
		}
		g.ids = append(g.ids, r.row.ID)
	}
	for _, g := range groups {
		if len(g.ids) == 1 {
			if err := p.store.MarkDead(ctx, g.ids[0], errors.New(g.errMsg)); err != nil {
				batchResult.Errors = append(batchResult.Errors, err)
			} else {
				batchResult.Dead++
			}
			continue
		}
		mismatched, err := p.store.MarkDeadBatch(ctx, g.ids, p.workerID, errors.New(g.errMsg))
		if err != nil {
			for _, id := range g.ids {
				if perErr := p.store.MarkDead(ctx, id, errors.New(g.errMsg)); perErr != nil {
					batchResult.Errors = append(batchResult.Errors, perErr)
				} else {
					batchResult.Dead++
				}
			}
			continue
		}
		batchResult.Dead += len(g.ids) - len(mismatched)
		for _, id := range mismatched {
			if perErr := p.store.MarkDead(ctx, id, errors.New(g.errMsg)); perErr != nil {
				p.incFinalStateError("dead")
				batchResult.Errors = append(batchResult.Errors, perErr)
			} else {
				batchResult.Dead++
			}
		}
	}
}

// incFinalStateError increments the outbox final-state update error metric
// with a bounded state label restricted to "published", "retry", or "dead".
func (p *Processor) incFinalStateError(state string) {
	if p.metrics == nil {
		return
	}
	switch state {
	case "published":
		p.metrics.Inc("outbox_final_state_update_error_published_total")
	case "retry":
		p.metrics.Inc("outbox_final_state_update_error_retry_total")
	case "dead":
		p.metrics.Inc("outbox_final_state_update_error_dead_total")
	}
}

// Run continuously drains pending outbox events, immediately repeating while
// work is available and waiting only when a batch claims nothing. Backlog
// sampling is performed on a separate ticker (default 10 s) to avoid hitting
// the database on every hot-loop iteration. Cancellation exits cleanly.
func (p *Processor) Run(ctx context.Context, idleInterval time.Duration) {
	if idleInterval <= 0 {
		idleInterval = time.Minute
	}
	if p.idleInterval > 0 {
		idleInterval = p.idleInterval
	}

	backlogDone := make(chan struct{})
	go func() {
		defer close(backlogDone)
		p.sampleBacklog(ctx)
		ticker := time.NewTicker(p.backlogSampleInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.sampleBacklog(ctx)
			}
		}
	}()

	for {
		result, err := p.ProcessBatch(ctx)
		if err != nil {
			p.recordRunFailure(err)
			if !waitForContextOrTimer(ctx, p.errorBackoff) {
				<-backlogDone
				return
			}
			continue
		}
		if result.Claimed > 0 {
			continue
		}
		if !waitForContextOrTimer(ctx, idleInterval) {
			<-backlogDone
			return
		}
	}
}

// recordRunFailure surfaces batch-level failures instead of dropping them.
func (p *Processor) recordRunFailure(err error) {
	if p.metrics != nil {
		p.metrics.Inc("outbox_run_errors_total")
	}
	p.logger.Error().Err(err).Str("worker", p.workerID).
		Msg("outbox processing cycle failed; backlog may be stalling")
	if p.alerter == nil {
		return
	}
	alertErr := p.alerter.Emit(context.Background(), alerting.Alert{
		Severity: alerting.SeverityP2,
		Title:    "outbox processing cycle failed",
		Detail:   err.Error(),
		Labels:   map[string]string{"worker": p.workerID, "component": "outbox"},
	})
	if alertErr != nil {
		p.logger.Warn().Err(alertErr).Msg("failed to emit outbox failure alert")
	}
}

// processEvent dispatches a single outbox event to its handler and persists the
// outcome. It returns the outcome (published, retried, or dead) and an error
// that is non-nil only for state-transition database failures. Handler failures
// that are successfully persisted as retry or dead outcomes return a nil error
// so the batch can continue processing remaining events.
func (p *Processor) processEvent(ctx context.Context, row models.EventOutbox) (outcome processOutcome, err error) {
	metricScope := p.beginOutboxMetricScope(row)
	defer func() {
		metricScope.finish(outcome)
	}()

	handler := p.lookup(row.Event)
	handlerErr := ErrOutboxHandlerNotFound
	if handler != nil {
		handlerCtx := trace.WithOutboxID(trace.WithRequestID(ctx, row.RequestID), row.ID)
		handlerCtx, span := trace.StartSpan(handlerCtx, "outbox.process_event", map[string]string{
			"event":          row.Event,
			"aggregate_type": row.AggregateType,
			"aggregate_id":   strconv.FormatUint(row.AggregateID, 10),
			"outbox_id":      strconv.FormatUint(row.ID, 10),
		})
		metrics.SetOutboxInflight(metricScope.workClass, 1)
		handlerErr = handler(handlerCtx, row)
		metrics.SetOutboxInflight(metricScope.workClass, 0)
		span.End(handlerErr)
	}
	if handlerErr == nil {
		if p.metrics != nil {
			p.metrics.Inc("outbox_publish_total")
		}
		if err := p.store.MarkPublished(ctx, row.ID); err != nil {
			return outcomePublished, err
		}
		return outcomePublished, nil
	}

	if row.Attempts+1 >= p.maxAttempts {
		if p.metrics != nil {
			p.metrics.Inc("outbox_dead_letter_total")
		}
		if p.alerter != nil {
			if alertErr := p.alerter.Emit(context.Background(), alerting.Alert{
				Severity: alerting.SeverityP1,
				Title:    "outbox event moved to dead-letter",
				Detail:   handlerErr.Error(),
				Labels: map[string]string{
					"worker":         p.workerID,
					"component":      "outbox",
					"event":          row.Event,
					"outbox_id":      strconv.FormatUint(row.ID, 10),
					"aggregate_type": row.AggregateType,
					"aggregate_id":   strconv.FormatUint(row.AggregateID, 10),
				},
			}); alertErr != nil {
				p.logger.Warn().Err(alertErr).Uint64("outbox_id", row.ID).
					Msg("failed to emit outbox dead-letter alert")
			}
		}
		if err := p.store.MarkDead(ctx, row.ID, handlerErr); err != nil {
			return outcomeDead, err
		}
		return outcomeDead, nil
	}
	if p.metrics != nil {
		p.metrics.Inc("outbox_publish_retry_total")
	}
	if err := p.store.MarkRetry(ctx, row.ID, handlerErr, time.Now().UTC().Add(p.retryDelay)); err != nil {
		return outcomeRetried, err
	}
	return outcomeRetried, nil
}

// waitForContextOrTimer waits for either context cancellation or the given
// duration. It returns true if the timer fired (caller should continue) or
// false if the context was cancelled (caller should return).
func waitForContextOrTimer(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// sampleBacklog records the current pending event count as the outbox_backlog
// gauge metric. Errors are silently ignored — the metric is best-effort.
func (p *Processor) sampleBacklog(ctx context.Context) {
	if p.metrics == nil {
		return
	}
	events := p.eventFilter()
	if backlog, err := p.store.CountPendingForEvents(ctx, events); err == nil {
		p.metrics.Set("outbox_backlog", backlog)
	}
}

func (p *Processor) lookup(event string) Handler {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.handlers[event]
}

func (p *Processor) eventFilter() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.events))
	copy(out, p.events)
	return out
}

func (p *Processor) orderedEventFilter() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.orderedEvents))
	copy(out, p.orderedEvents)
	return out
}

func normalizedEvents(events []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(events))
	for _, event := range events {
		if event == "" || seen[event] {
			continue
		}
		seen[event] = true
		out = append(out, event)
	}
	return out
}
