package events

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

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
		batchSize:   100,
		maxAttempts: 3,
		retryDelay:  time.Minute,
		workerID:    "outbox-" + uuid.NewString(),
		lease:       2 * time.Minute,
	}
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

	_, _ = p.ProcessOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, _ = p.ProcessOnce(ctx)
		}
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
			p.metrics.Inc("outbox_publish_failed_total")
		}
		return p.store.MarkFailed(ctx, row.ID, err)
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
