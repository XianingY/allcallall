package trace

import (
	"context"
	"errors"
	"testing"
)

func TestSpanRecorderCapturesNestedSpans(t *testing.T) {
	recorder := NewMemorySpanRecorder()
	ctx := WithSpanRecorder(WithRequestID(context.Background(), "req-span-1"), recorder)

	ctx, parent := StartSpan(ctx, "parent", map[string]string{"component": "test"})
	childCtx, child := StartSpan(ctx, "child", map[string]string{"operation": "demo"})
	if TraceID(childCtx) != "req-span-1" {
		t.Fatalf("expected request id backed trace id, got %q", TraceID(childCtx))
	}
	if SpanID(childCtx) == "" {
		t.Fatalf("expected child span id")
	}
	child.End(errors.New("boom"))
	parent.End(nil)

	records := recorder.Records()
	if len(records) != 2 {
		t.Fatalf("unexpected records: %+v", records)
	}
	if records[0].Name != "child" || records[0].Status != "error" || records[0].Error != "boom" {
		t.Fatalf("unexpected child record: %+v", records[0])
	}
	if records[1].Name != "parent" || records[1].Status != "ok" {
		t.Fatalf("unexpected parent record: %+v", records[1])
	}
	if records[0].ParentSpanID != records[1].SpanID {
		t.Fatalf("expected child parent_span_id to match parent span_id: child=%+v parent=%+v", records[0], records[1])
	}
}

func TestSpanWithoutRecorderIsNoop(t *testing.T) {
	_, span := StartSpan(context.Background(), "noop", nil)
	span.End(nil)
	span.End(errors.New("ignored"))
}
