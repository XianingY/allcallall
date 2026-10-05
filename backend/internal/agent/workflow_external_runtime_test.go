package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestPythonLangGraphRuntimeResumeWorkflowContract(t *testing.T) {
	input := WorkflowRuntimeResumeRequest{
		RequestID:                 "request-1",
		ExecutionID:               "workflow:9:resume:3:abcd",
		ExpectedCheckpointVersion: 3,
		OrganizationID:            1,
		UserID:                    2,
		ConversationID:            3,
		WorkflowRunID:             9,
		Resume: WorkflowRuntimeResume{
			ApprovalRequestID: "approval-1",
			Decisions:         []WorkflowRuntimeDecision{{ToolCallID: "call-1", Decision: "approve"}},
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v1/workflows/meeting_brief/resume" {
			t.Errorf("unexpected runtime request %s %s", request.Method, request.URL.Path)
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read resume request: %v", err)
		}
		if bytes.Contains(raw, []byte(`"agent_run_id"`)) {
			t.Errorf("workflow resume must omit agent_run_id: %s", raw)
		}
		var received WorkflowRuntimeResumeRequest
		if err := json.Unmarshal(raw, &received); err != nil {
			t.Errorf("decode resume request: %v", err)
		}
		if received.ExecutionID != input.ExecutionID || received.Resume.ApprovalRequestID != input.Resume.ApprovalRequestID || len(received.Resume.Decisions) != 1 {
			t.Errorf("unexpected resume body: %+v", received)
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(WorkflowRuntimeResponse{
			Status:            models.WorkflowRunStatusReady,
			CheckpointID:      "checkpoint-4",
			CheckpointVersion: 4,
			ApprovalDecisions: input.Resume.Decisions,
		})
	}))
	defer server.Close()
	runtime := &PythonLangGraphRuntime{baseURL: server.URL, client: server.Client()}
	response, err := runtime.ResumeWorkflow(context.Background(), WorkflowPresetMeetingBrief, input)
	if err != nil {
		t.Fatalf("resume workflow request failed: %v", err)
	}
	if response.CheckpointVersion != 4 {
		t.Fatalf("unexpected resume response: %+v", response)
	}
}

func TestPythonLangGraphRuntimeClassifiesConflictCodes(t *testing.T) {
	for _, test := range []struct {
		name           string
		code           string
		wantCheckpoint bool
	}{
		{name: "checkpoint", code: "checkpoint_version_conflict", wantCheckpoint: true},
		{name: "invalid approval", code: "invalid_approval_resume", wantCheckpoint: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(http.StatusConflict)
				_ = json.NewEncoder(writer).Encode(map[string]any{"detail": map[string]any{"code": test.code}})
			}))
			defer server.Close()
			runtime := &PythonLangGraphRuntime{baseURL: server.URL, client: server.Client()}
			_, err := runtime.ResumeWorkflow(context.Background(), WorkflowPresetMeetingBrief, WorkflowRuntimeResumeRequest{})
			if err == nil {
				t.Fatal("expected runtime conflict")
			}
			if got := errors.Is(err, ErrCheckpointVersionConflict); got != test.wantCheckpoint {
				t.Fatalf("checkpoint classification=%v want=%v error=%v", got, test.wantCheckpoint, err)
			}
			if !test.wantCheckpoint && !errors.Is(err, ErrWorkflowRuntimeConflict) {
				t.Fatalf("expected generic workflow runtime conflict, got %v", err)
			}
		})
	}
}

func TestPythonLangGraphRuntimeClassifiesOperationalErrors(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		code   string
		want   error
	}{
		{name: "checkpoint busy", status: http.StatusConflict, code: "checkpoint_execution_busy", want: ErrCheckpointExecutionBusy},
		{name: "transaction too large", status: http.StatusRequestEntityTooLarge, code: "checkpoint_transaction_too_large", want: ErrCheckpointTransactionTooLarge},
		{name: "runtime unavailable", status: http.StatusServiceUnavailable, code: "runtime_unavailable", want: ErrWorkflowRuntimeUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				_ = json.NewEncoder(writer).Encode(map[string]any{"detail": map[string]any{"code": test.code}})
			}))
			defer server.Close()
			runtime := &PythonLangGraphRuntime{baseURL: server.URL, client: server.Client()}
			_, err := runtime.ResumeAgent(context.Background(), WorkflowRuntimeResumeRequest{})
			if !errors.Is(err, test.want) {
				t.Fatalf("expected %v, got %v", test.want, err)
			}
		})
	}
}

func minimalRuntimeRequest() WorkflowRuntimeRequest {
	return WorkflowRuntimeRequest{
		OrganizationID: 1,
		UserID:         2,
		ConversationID: 3,
		WorkflowRunID:  4,
		Preset:         "meeting_brief",
		Goal:           "test",
	}
}

func writeReadyRuntimeResponse(t *testing.T, w http.ResponseWriter) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	resp := WorkflowRuntimeResponse{
		Status:            models.WorkflowRunStatusReady,
		ExecutionID:       "workflow:1",
		CheckpointID:      "checkpoint-1",
		CheckpointVersion: 1,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		t.Fatalf("encode ready response: %v", err)
	}
}

func TestPythonRuntimePropagatesAbsoluteDeadline(t *testing.T) {
	deadline := time.Now().UTC().Add(3 * time.Second).Truncate(time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, err := time.Parse(time.RFC3339Nano, r.Header.Get("X-AllCallAll-Deadline"))
		if err != nil || !got.Equal(deadline) {
			t.Fatalf("deadline=%q parsed=%v err=%v", r.Header.Get("X-AllCallAll-Deadline"), got, err)
		}
		writeReadyRuntimeResponse(t, w)
	}))
	defer server.Close()

	runtime := NewPythonLangGraphRuntime(server.URL, server.Client())
	_, err := runtime.RunAgent(ctx, minimalRuntimeRequest())
	if err != nil {
		t.Fatal(err)
	}
}

func TestPythonRuntimePropagatesAttemptHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempt := r.Header.Get("X-AllCallAll-Attempt")
		if attempt == "" {
			t.Fatal("X-AllCallAll-Attempt header must be set")
		}
		writeReadyRuntimeResponse(t, w)
	}))
	defer server.Close()

	runtime := NewPythonLangGraphRuntime(server.URL, server.Client())
	_, err := runtime.RunAgent(context.Background(), minimalRuntimeRequest())
	if err != nil {
		t.Fatal(err)
	}
}

func TestRuntimeOverloadedError(t *testing.T) {
	for _, test := range []struct {
		name        string
		status      int
		retryAfter  string
		wantRetry   time.Duration
	}{
		{name: "429 with integer retry", status: http.StatusTooManyRequests, retryAfter: "2", wantRetry: 2 * time.Second},
		{name: "503 with HTTP-date retry", status: http.StatusServiceUnavailable, retryAfter: time.Now().UTC().Add(2 * time.Second).Format(http.TimeFormat), wantRetry: 2 * time.Second},
		{name: "429 without retry-after", status: http.StatusTooManyRequests, retryAfter: "", wantRetry: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.retryAfter != "" {
					w.Header().Set("Retry-After", test.retryAfter)
				}
				w.WriteHeader(test.status)
				_, _ = w.Write([]byte(`{"detail":"overloaded"}`))
			}))
			defer server.Close()
			runtime := NewPythonLangGraphRuntime(server.URL, server.Client())
			_, err := runtime.RunAgent(context.Background(), minimalRuntimeRequest())
			if err == nil {
				t.Fatal("expected overload error")
			}
			var overloaded *RuntimeOverloadedError
			if !errors.As(err, &overloaded) {
				t.Fatalf("expected RuntimeOverloadedError, got %T: %v", err, err)
			}
			if !errors.Is(err, ErrWorkflowRuntimeUnavailable) {
				t.Fatalf("RuntimeOverloadedError must wrap ErrWorkflowRuntimeUnavailable, got %v", err)
			}
			if test.wantRetry > 0 {
				if overloaded.RetryAfter < test.wantRetry-time.Second || overloaded.RetryAfter > test.wantRetry+time.Second {
					t.Fatalf("RetryAfter=%v want≈%v", overloaded.RetryAfter, test.wantRetry)
				}
			} else if overloaded.RetryAfter != 0 {
				t.Fatalf("RetryAfter should be 0 when no Retry-After header, got %v", overloaded.RetryAfter)
			}
		})
	}
}
