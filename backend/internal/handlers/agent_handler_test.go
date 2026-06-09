package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
		&models.FollowUpTask{},
		&models.CallRoom{},
		&models.ContactProfile{},
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
