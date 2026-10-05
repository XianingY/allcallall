package events

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/alerting"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

var ErrOutboxHandlerNotFound = errors.New("outbox handler not found")

type Handler func(ctx context.Context, event models.EventOutbox) error

// ProcessBatchResult holds the outcome of a single ProcessBatch call.
// Handler failures successfully persisted as retry or dead outcomes are
// counted in Retried/Dead respectively and do not appear in Errors; only
// claim failures (returned as the batch error) and state-transition database
// failures (collected in Errors) are considered processor errors.
type ProcessBatchResult struct {
	Claimed   int       // events claimed from the store
	Succeeded int       // events published successfully
	Retried   int       // events moved to retry (handler failed, under max attempts)
	Dead      int       // events moved to dead-letter (handler failed, max attempts reached)
	Errors    []error   // state-transition database failures
}

type processOutcome int

const (
	outcomePublished processOutcome = iota
	outcomeRetried
	outcomeDead
)

type Processor struct {
	store                *Store
	handlers             map[string]Handler
	events               []string
	metrics              metrics.Recorder
	logger               zerolog.Logger
	alerter              *alerting.Service
	batchSize            int
	maxAttempts          int
	retryDelay           time.Duration
	workerID             string
	lease                time.Duration
	errorBackoff         time.Duration
	backlogSampleInterval time.Duration
	mu                   sync.RWMutex
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
		errorBackoff:          5 * time.Second,
		backlogSampleInterval: 10 * time.Second,
	}
}

// WithLogger 注入日志器。生产环境务必注入——否则 outbox 批量失败只会体现在指标上。
// WithLogger injects a logger so batch-level failures are visible in logs.
func (p *Processor) WithLogger(logger zerolog.Logger) *Processor {
	p.logger = logger
	return p
}

// WithAlerter 注入告警服务。批次级失败会按 P2 上报，避免积压静默无人知晓。
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

// ProcessOnce processes one batch of pending outbox events and returns the
// number of events whose state transition succeeded (published, retried, or
// dead-lettered). It is a compatibility wrapper around ProcessBatch for
// callers that have not yet migrated.
func (p *Processor) ProcessOnce(ctx context.Context) (int, error) {
	// Sample backlog before processing for backward compatibility when called
	// standalone (outside Run). The Run loop uses a separate ticker instead.
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
// Handler failures that are successfully persisted as retry or dead outcomes
// are counted in the result and do not cause the batch to abort; only claim
// failures (returned as the error) and state-transition database failures
// (collected in result.Errors) are considered processor errors.
func (p *Processor) ProcessBatch(ctx context.Context) (ProcessBatchResult, error) {
	if p == nil || p.store == nil {
		return ProcessBatchResult{}, errors.New("outbox processor store is nil")
	}
	events := p.eventFilter()
	rows, err := p.store.ClaimPendingForEvents(ctx, p.batchSize, p.workerID, p.lease, events)
	if err != nil {
		return ProcessBatchResult{}, err
	}
	var result ProcessBatchResult
	result.Claimed = len(rows)
	for _, row := range rows {
		outcome, eventErr := p.processEvent(ctx, row)
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

// Run continuously drains pending outbox events, immediately repeating while
// work is available and waiting only when a batch claims nothing. Backlog
// sampling is performed on a separate ticker (default 10 s) to avoid hitting
// the database on every hot-loop iteration. Cancellation exits cleanly.
func (p *Processor) Run(ctx context.Context, idleInterval time.Duration) {
	if idleInterval <= 0 {
		idleInterval = time.Minute
	}

	// Sample backlog on a separate ticker so the hot processing loop
	// does not call CountPendingForEvents on every iteration.
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

// recordRunFailure 让批次级失败可见并上报。此前错误被整体丢弃，outbox 会静默
// 停滞而循环继续空转，故障完全不可观测。
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
	// 事件积压会直接表现为业务链路静默中断，按 P2 上报（去重窗口内只报一次）。
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
func (p *Processor) processEvent(ctx context.Context, row models.EventOutbox) (processOutcome, error) {
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
		handlerErr = handler(handlerCtx, row)
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
		// 达到最大重试次数：转入死信终态并告警，避免毒事件继续占用处理批次。
		// 此处 Emit 与 recordRunFailure 一致使用 background ctx，因为事件已离开
		// 请求生命周期；死信属数据投递失败，按 P1 上报（可能丢失业务事件）。
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
