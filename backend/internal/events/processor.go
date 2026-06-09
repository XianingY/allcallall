package events

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

var ErrOutboxHandlerNotFound = errors.New("outbox handler not found")

type Handler func(ctx context.Context, event models.EventOutbox) error

type metricRecorder interface {
	Inc(name string)
}

type Processor struct {
	store       *Store
	handlers    map[string]Handler
	metrics     metricRecorder
	batchSize   int
	maxAttempts int
	retryDelay  time.Duration
	mu          sync.RWMutex
}

func NewProcessor(store *Store, counters ...metricRecorder) *Processor {
	var metrics metricRecorder
	if len(counters) > 0 {
		metrics = counters[0]
	}
	return &Processor{
		store:       store,
		handlers:    make(map[string]Handler),
		metrics:     metrics,
		batchSize:   100,
		maxAttempts: 3,
		retryDelay:  time.Minute,
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

func (p *Processor) ProcessOnce(ctx context.Context) (int, error) {
	if p == nil || p.store == nil {
		return 0, errors.New("outbox processor store is nil")
	}
	rows, err := p.store.ListPending(ctx, p.batchSize)
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
		err = handler(handlerCtx, row)
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
