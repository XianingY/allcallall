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

type Processor struct {
	store       *Store
	handlers    map[string]Handler
	events      []string
	metrics     metrics.Recorder
	logger      zerolog.Logger
	alerter     *alerting.Service
	batchSize   int
	maxAttempts int
	retryDelay  time.Duration
	workerID    string
	lease       time.Duration
	mu          sync.RWMutex
}

func NewProcessor(store *Store, recorders ...metrics.Recorder) *Processor {
	var metrics metrics.Recorder
	if len(recorders) > 0 {
		metrics = recorders[0]
	}
	return &Processor{
		store:       store,
		handlers:    make(map[string]Handler),
		metrics:     metrics,
		logger:      zerolog.Nop(),
		batchSize:   100,
		maxAttempts: 3,
		retryDelay:  time.Minute,
		workerID:    "outbox-" + uuid.NewString(),
		lease:       2 * time.Minute,
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

func (p *Processor) ProcessOnce(ctx context.Context) (int, error) {
	if p == nil || p.store == nil {
		return 0, errors.New("outbox processor store is nil")
	}
	events := p.eventFilter()
	if p.metrics != nil {
		if backlog, countErr := p.store.CountPendingForEvents(ctx, events); countErr == nil {
			p.metrics.Set("outbox_backlog", backlog)
		}
	}
	rows, err := p.store.ClaimPendingForEvents(ctx, p.batchSize, p.workerID, p.lease, events)
	if err != nil {
		return 0, err
	}
	processed := 0
	for _, row := range rows {
		if err := p.processEvent(ctx, row); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (p *Processor) Run(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	if _, err := p.ProcessOnce(ctx); err != nil {
		p.recordRunFailure(err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := p.ProcessOnce(ctx); err != nil {
				p.recordRunFailure(err)
			}
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

func (p *Processor) processEvent(ctx context.Context, row models.EventOutbox) error {
	handler := p.lookup(row.Event)
	err := ErrOutboxHandlerNotFound
	if handler != nil {
		handlerCtx := trace.WithOutboxID(trace.WithRequestID(ctx, row.RequestID), row.ID)
		handlerCtx, span := trace.StartSpan(handlerCtx, "outbox.process_event", map[string]string{
			"event":          row.Event,
			"aggregate_type": row.AggregateType,
			"aggregate_id":   strconv.FormatUint(row.AggregateID, 10),
			"outbox_id":      strconv.FormatUint(row.ID, 10),
		})
		err = handler(handlerCtx, row)
		span.End(err)
	}
	if err == nil {
		if p.metrics != nil {
			p.metrics.Inc("outbox_publish_total")
		}
		return p.store.MarkPublished(ctx, row.ID)
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
				Detail:   err.Error(),
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
		return p.store.MarkDead(ctx, row.ID, err)
	}
	if p.metrics != nil {
		p.metrics.Inc("outbox_publish_retry_total")
	}
	return p.store.MarkRetry(ctx, row.ID, err, time.Now().UTC().Add(p.retryDelay))
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
