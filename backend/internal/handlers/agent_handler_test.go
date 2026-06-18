package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/agent"
	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/trace"
)

func newAgentHandlerTestEnv(t *testing.T) (*AgentHandler, *gorm.DB, models.Conversation) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "agent-handler.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Conversation{},
		&models.ConversationMember{},
		&models.ConversationNote{},
		&models.Message{},
		&models.AgentRun{},
		&models.AgentStep{},
		&models.AgentToolCall{},
		&models.AgentMemory{},
		&models.AgentContextChunk{},
		&models.AgentPromptVersion{},
		&models.ToolSchemaVersion{},
		&models.FollowUpTask{},
		&models.CallRoom{},
		&models.CallFollowup{},
		&models.CallTranscriptSegment{},
		&models.RecordingTranscription{},
		&models.MeetingTranscriptSegment{},
		&models.ContactProfile{},
		&models.WorkflowRun{},
		&models.WorkflowTask{},
		&models.WorkflowHistoryEvent{},
		&models.WorkflowSignal{},
		&models.WorkflowTimer{},
		&models.AgentMessage{},
		&models.ToolPolicy{},
		&models.ToolApproval{},
		&models.EventOutbox{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	conversation := models.Conversation{
		OrganizationID: 42,
		Type:           models.ConversationTypeChannel,
		Title:          "Support handoff",
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityUrgent,
		CreatedBy:      7,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	if err := db.Create(&models.ConversationMember{
		ConversationID: conversation.ID,
		UserID:         7,
		Role:           models.OrganizationRoleMember,
	}).Error; err != nil {
		t.Fatalf("create member failed: %v", err)
	}
	if err := db.Create(&models.Message{
		OrganizationID: conversation.OrganizationID,
		ConversationID: conversation.ID,
		SenderID:       7,
		Type:           models.MessageTypeText,
		Body:           "Need next call scheduling and owner confirmation.",
	}).Error; err != nil {
		t.Fatalf("create message failed: %v", err)
	}

	return NewAgentHandler(zerolog.Nop(), agent.NewService(db)), db, conversation
}

func TestAgentHandlerCreateAndGetRun(t *testing.T) {
	handler, _, conversation := newAgentHandlerTestEnv(t)
	router := newRouterWithClaims(&auth.Claims{UserID: 7, Email: "owner@example.com"}, handler.RegisterProtectedRoutes)

	body, _ := json.Marshal(map[string]any{
		"conversation_id": conversation.ID,
		"goal":            "summarize current support handoff",
	})
	rec := performRequest(t, router, http.MethodPost, "/api/v1/agent/runs", body)
	expectHandlerStatus(t, rec, http.StatusBadRequest)

	reqBody, _ := json.Marshal(map[string]any{
		"conversation_id": conversation.ID,
		"goal":            "summarize current support handoff",
	})
	rec = performRequestWithOrganizationAndRequestID(t, router, http.MethodPost, "/api/v1/agent/runs", reqBody, conversation.OrganizationID, "req-agent-handler-1")
	expectHandlerStatus(t, rec, http.StatusAccepted)

	var createResponse struct {
		Run struct {
			ID             uint64   `json:"id"`
			Status         string   `json:"status"`
			ConversationID uint64   `json:"conversation_id"`
			RequestID      string   `json:"request_id"`
			Goal           string   `json:"goal"`
			ActionItems    []string `json:"action_items"`
		} `json:"run"`
		Steps     []agentStepResponse     `json:"steps"`
		ToolCalls []agentToolCallResponse `json:"tool_calls"`
	}
	decodeBody(t, rec.Body.Bytes(), &createResponse)
	if createResponse.Run.Status != models.AgentRunStatusPending {
		t.Fatalf("unexpected status: %s", createResponse.Run.Status)
	}
	if createResponse.Run.ConversationID != conversation.ID || createResponse.Run.Goal != "summarize current support handoff" || len(createResponse.Run.ActionItems) != 0 {
		t.Fatalf("unexpected run payload: %+v", createResponse.Run)
	}
	if createResponse.Run.RequestID != "req-agent-handler-1" {
		t.Fatalf("unexpected request id payload: %+v", createResponse.Run)
	}
	if len(createResponse.Steps) != 0 || len(createResponse.ToolCalls) != 0 {
		t.Fatalf("unexpected explainability payload: steps=%d tool_calls=%d", len(createResponse.Steps), len(createResponse.ToolCalls))
	}

	rec = performRequestWithOrganization(t, router, http.MethodGet, fmt.Sprintf("/api/v1/agent/runs/%d", createResponse.Run.ID), nil, conversation.OrganizationID)
	expectHandlerStatus(t, rec, http.StatusOK)
	var getResponse struct {
		Run struct {
			ID        uint64 `json:"id"`
			RequestID string `json:"request_id"`
		} `json:"run"`
	}
	decodeBody(t, rec.Body.Bytes(), &getResponse)
	if getResponse.Run.RequestID != "req-agent-handler-1" {
		t.Fatalf("unexpected get request id payload: %+v", getResponse.Run)
	}
}

func TestAgentHandlerGetRunEvents(t *testing.T) {
	handler, _, conversation := newAgentHandlerTestEnv(t)
	router := newRouterWithClaims(&auth.Claims{UserID: 7, Email: "owner@example.com"}, handler.RegisterProtectedRoutes)

	reqBody, _ := json.Marshal(map[string]any{
		"conversation_id": conversation.ID,
		"goal":            "summarize current support handoff",
	})
	rec := performRequestWithOrganizationAndRequestID(t, router, http.MethodPost, "/api/v1/agent/runs", reqBody, conversation.OrganizationID, "req-agent-events-1")
	expectHandlerStatus(t, rec, http.StatusAccepted)
	var createResponse struct {
		Run struct {
			ID uint64 `json:"id"`
		} `json:"run"`
	}
	decodeBody(t, rec.Body.Bytes(), &createResponse)
	if _, err := handler.service.ExecuteRun(trace.WithRequestID(t.Context(), "req-agent-events-1"), createResponse.Run.ID); err != nil {
		t.Fatalf("execute run failed: %v", err)
	}

	rec = performRequestWithOrganization(t, router, http.MethodGet, fmt.Sprintf("/api/v1/agent/runs/%d/events", createResponse.Run.ID), nil, conversation.OrganizationID)
	expectHandlerStatus(t, rec, http.StatusOK)
	var eventsResponse struct {
		RunID  uint64                  `json:"run_id"`
		Events []agentRunEventResponse `json:"events"`
	}
	decodeBody(t, rec.Body.Bytes(), &eventsResponse)
	if eventsResponse.RunID != createResponse.Run.ID {
		t.Fatalf("unexpected run id: %d", eventsResponse.RunID)
	}
	seen := map[string]bool{}
	for i, event := range eventsResponse.Events {
		if event.Sequence != i+1 {
			t.Fatalf("unexpected event sequence at %d: %+v", i, event)
		}
		seen[event.Event] = true
	}
	for _, required := range []string{
		agent.RunEventRunStarted,
		agent.RunEventStepStarted,
		agent.RunEventToolCalled,
		agent.RunEventToolDone,
		agent.RunEventRunReady,
	} {
		if !seen[required] {
			t.Fatalf("missing required event %s in %+v", required, seen)
		}
	}
}

func TestAgentHandlerCreateWorkflowWithPreset(t *testing.T) {
	handler, _, conversation := newAgentHandlerTestEnv(t)
	router := newRouterWithClaims(&auth.Claims{UserID: 7, Email: "owner@example.com"}, handler.RegisterProtectedRoutes)

	reqBody, _ := json.Marshal(map[string]any{
		"conversation_id": conversation.ID,
		"preset":          "meeting_brief",
	})
	rec := performRequestWithOrganizationAndRequestID(t, router, http.MethodPost, "/api/v1/agent/workflows", reqBody, conversation.OrganizationID, "req-workflow-handler-1")
	expectHandlerStatus(t, rec, http.StatusAccepted)

	var response struct {
		Workflow struct {
			ID             uint64 `json:"id"`
			ConversationID uint64 `json:"conversation_id"`
			Preset         string `json:"preset"`
			Goal           string `json:"goal"`
			WorkflowType   string `json:"workflow_type"`
		} `json:"workflow"`
	}
	decodeBody(t, rec.Body.Bytes(), &response)
	if response.Workflow.ConversationID != conversation.ID {
		t.Fatalf("unexpected conversation id: %+v", response.Workflow)
	}
	if response.Workflow.Preset != "meeting_brief" {
		t.Fatalf("unexpected preset: %+v", response.Workflow)
	}
	if response.Workflow.WorkflowType != "meeting_agent" {
		t.Fatalf("unexpected workflow type: %+v", response.Workflow)
	}
	if response.Workflow.Goal == "" {
		t.Fatalf("expected default goal for preset, got %+v", response.Workflow)
	}
}

func TestAgentHandlerStreamsRunEvents(t *testing.T) {
	handler, _, conversation := newAgentHandlerTestEnv(t)
	router := newRouterWithClaims(&auth.Claims{UserID: 7, Email: "owner@example.com"}, handler.RegisterProtectedRoutes)

	reqBody, _ := json.Marshal(map[string]any{
		"conversation_id": conversation.ID,
		"goal":            "summarize current support handoff",
	})
	rec := performRequestWithOrganizationAndRequestID(t, router, http.MethodPost, "/api/v1/agent/runs", reqBody, conversation.OrganizationID, "req-agent-stream-1")
	expectHandlerStatus(t, rec, http.StatusAccepted)
	var createResponse struct {
		Run struct {
			ID uint64 `json:"id"`
		} `json:"run"`
	}
	decodeBody(t, rec.Body.Bytes(), &createResponse)
	if _, err := handler.service.ExecuteRun(trace.WithRequestID(t.Context(), "req-agent-stream-1"), createResponse.Run.ID); err != nil {
		t.Fatalf("execute run failed: %v", err)
	}

	rec = performRequestWithOrganization(t, router, http.MethodGet, fmt.Sprintf("/api/v1/agent/runs/%d/events/stream?timeout_ms=1000", createResponse.Run.ID), nil, conversation.OrganizationID)
	expectHandlerStatus(t, rec, http.StatusOK)
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("unexpected stream content type: %s", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, required := range []string{
		"event:run_started",
		"event:step_started",
		"event:tool_called",
		"event:tool_done",
		"event:run_ready",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("missing SSE marker %q in body:\n%s", required, body)
		}
	}
}

func TestAgentHandlerRejectsConversationOutsideMembership(t *testing.T) {
	handler, _, conversation := newAgentHandlerTestEnv(t)
	router := newRouterWithClaims(&auth.Claims{UserID: 99, Email: "outsider@example.com"}, handler.RegisterProtectedRoutes)

	body, _ := json.Marshal(map[string]any{"conversation_id": conversation.ID})
	rec := performRequestWithOrganization(t, router, http.MethodPost, "/api/v1/agent/runs", body, conversation.OrganizationID)
	expectHandlerStatus(t, rec, http.StatusForbidden)
}

func performRequestWithOrganization(t *testing.T, router http.Handler, method, path string, body []byte, organizationID uint64) *httptest.ResponseRecorder {
	t.Helper()

	return performRequestWithOrganizationAndRequestID(t, router, method, path, body, organizationID, "")
}

func performRequestWithOrganizationAndRequestID(t *testing.T, router http.Handler, method, path string, body []byte, organizationID uint64, requestID string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", organizationID))
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
		req = req.WithContext(trace.WithRequestID(req.Context(), requestID))
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}
