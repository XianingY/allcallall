package trace

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	traceIDKey      contextKey = "trace_id"
	spanIDKey       contextKey = "span_id"
	spanRecorderKey contextKey = "span_recorder"
)

type SpanRecorder interface {
	RecordSpan(record SpanRecord)
}

type SpanRecord struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	RequestID    string            `json:"request_id,omitempty"`
	OutboxID     uint64            `json:"outbox_id,omitempty"`
	Name         string            `json:"name"`
	Status       string            `json:"status"`
	Error        string            `json:"error,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
	StartedAt    time.Time         `json:"started_at"`
	EndedAt      time.Time         `json:"ended_at"`
	DurationMS   int64             `json:"duration_ms"`
}

type Span struct {
	recorder     SpanRecorder
	traceID      string
	spanID       string
	parentSpanID string
	requestID    string
	outboxID     uint64
	name         string
	attributes   map[string]string
	startedAt    time.Time
	once         sync.Once
}

type MemorySpanRecorder struct {
	mu      sync.Mutex
	records []SpanRecord
}

func NewMemorySpanRecorder() *MemorySpanRecorder {
	return &MemorySpanRecorder{}
}

func (r *MemorySpanRecorder) RecordSpan(record SpanRecord) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, record)
}

func (r *MemorySpanRecorder) Records() []SpanRecord {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]SpanRecord, len(r.records))
	copy(out, r.records)
	return out
}

func WithSpanRecorder(ctx context.Context, recorder SpanRecorder) context.Context {
	if ctx == nil || recorder == nil {
		return ctx
	}
	return context.WithValue(ctx, spanRecorderKey, recorder)
}

func StartSpan(ctx context.Context, name string, attributes map[string]string) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID := TraceID(ctx)
	if traceID == "" {
		traceID = RequestID(ctx)
	}
	if traceID == "" {
		traceID = uuid.NewString()
	}
	spanID := uuid.NewString()
	span := &Span{
		recorder:     spanRecorder(ctx),
		traceID:      traceID,
		spanID:       spanID,
		parentSpanID: SpanID(ctx),
		requestID:    RequestID(ctx),
		outboxID:     OutboxID(ctx),
		name:         name,
		attributes:   copyAttributes(attributes),
		startedAt:    time.Now().UTC(),
	}
	ctx = context.WithValue(ctx, traceIDKey, traceID)
	ctx = context.WithValue(ctx, spanIDKey, spanID)
	return ctx, span
}

func (s *Span) End(err error) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		endedAt := time.Now().UTC()
		record := SpanRecord{
			TraceID:      s.traceID,
			SpanID:       s.spanID,
			ParentSpanID: s.parentSpanID,
			RequestID:    s.requestID,
			OutboxID:     s.outboxID,
			Name:         s.name,
			Status:       "ok",
			Attributes:   copyAttributes(s.attributes),
			StartedAt:    s.startedAt,
			EndedAt:      endedAt,
			DurationMS:   endedAt.Sub(s.startedAt).Milliseconds(),
		}
		if err != nil {
			record.Status = "error"
			record.Error = err.Error()
		}
		if s.recorder != nil {
			s.recorder.RecordSpan(record)
		}
	})
}

func TraceID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(traceIDKey).(string)
	return NormalizeRequestID(value)
}

func SpanID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(spanIDKey).(string)
	return NormalizeRequestID(value)
}

func spanRecorder(ctx context.Context) SpanRecorder {
	if ctx == nil {
		return nil
	}
	recorder, _ := ctx.Value(spanRecorderKey).(SpanRecorder)
	return recorder
}

func copyAttributes(attributes map[string]string) map[string]string {
	if len(attributes) == 0 {
		return nil
	}
	out := make(map[string]string, len(attributes))
	for key, value := range attributes {
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
