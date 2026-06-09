package trace

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGlobalSpanRecorderFallback(t *testing.T) {
	recorder := NewMemorySpanRecorder()
	SetGlobalSpanRecorder(recorder)
	defer SetGlobalSpanRecorder(nil)

	_, span := StartSpan(WithRequestID(context.Background(), "req-global-span"), "global.demo", nil)
	span.End(nil)

	records := recorder.Records()
	if len(records) != 1 {
		t.Fatalf("unexpected records: %+v", records)
	}
	if records[0].Name != "global.demo" || records[0].RequestID != "req-global-span" {
		t.Fatalf("unexpected global span record: %+v", records[0])
	}
}

func testTime(offsetMillis int64) time.Time {
	return time.Unix(1_700_000_000, offsetMillis*int64(time.Millisecond)).UTC()
}

func TestOTLPHTTPSpanRecorderPostsTracePayload(t *testing.T) {
	var gotPath string
	var gotPayload map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&gotPayload); err != nil {
			t.Fatalf("decode otlp payload failed: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	recorder := NewOTLPHTTPSpanRecorder(server.URL, "allcallall-test")
	recorder.RecordSpan(SpanRecord{
		TraceID:   "trace-1",
		SpanID:    "span-1",
		RequestID: "req-otlp-1",
		Name:      "agent.execute_run",
		Status:    "error",
		Error:     errors.New("planner timeout").Error(),
		Attributes: map[string]string{
			"component": "agent",
		},
		StartedAt: testTime(0),
		EndedAt:   testTime(10),
	})

	if gotPath != "/v1/traces" {
		t.Fatalf("unexpected otlp path: %s", gotPath)
	}
	raw, _ := json.Marshal(gotPayload)
	body := string(raw)
	for _, expected := range []string{"agent.execute_run", "allcallall-test", "req-otlp-1", "planner timeout", "component"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("otlp payload missing %q: %s", expected, body)
		}
	}
}
