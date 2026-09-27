package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type serviceErrorEnvelope struct {
	Error string `json:"error"`
	Code  string `json:"code"`
}

func performJSON(t *testing.T, router *gin.Engine, method, path string, orgID uint64, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", orgID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func decodeServiceError(t *testing.T, rec *httptest.ResponseRecorder) serviceErrorEnvelope {
	t.Helper()
	var envelope serviceErrorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode error envelope failed: %v body=%s", err, rec.Body.String())
	}
	return envelope
}

func assertServiceError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantCode, wantMessage string) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d body=%s", wantStatus, rec.Code, rec.Body.String())
	}
	envelope := decodeServiceError(t, rec)
	if envelope.Code != wantCode {
		t.Fatalf("expected code %q, got %q body=%s", wantCode, envelope.Code, rec.Body.String())
	}
	if envelope.Error != wantMessage {
		t.Fatalf("expected error message %q, got %q", wantMessage, envelope.Error)
	}
}

func assertNoInternalLeak(t *testing.T, rec *httptest.ResponseRecorder, fragments ...string) {
	t.Helper()
	body := rec.Body.String()
	for _, fragment := range append([]string{"no such table", "sqlite", "SELECT"}, fragments...) {
		if strings.Contains(body, fragment) {
			t.Fatalf("response leaked internal detail %q: %s", fragment, body)
		}
	}
}

func dropTable(t *testing.T, db *gorm.DB, table string) {
	t.Helper()
	if err := db.Exec("DROP TABLE " + table).Error; err != nil {
		t.Fatalf("drop table %s failed: %v", table, err)
	}
}

func createTestConversation(t *testing.T, router *gin.Engine, orgID uint64) uint64 {
	t.Helper()
	rec := performJSON(t, router, http.MethodPost, "/api/v1/conversations", orgID, `{"type":"channel","title":"Ops"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create conversation failed: %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Conversation struct {
			ID uint64 `json:"id"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode conversation failed: %v body=%s", err, rec.Body.String())
	}
	if resp.Conversation.ID == 0 {
		t.Fatalf("expected conversation id, body=%s", rec.Body.String())
	}
	return resp.Conversation.ID
}

// RED: teams:40 — a database failure on team creation must return a generic
// 500, not a 400 echoing the raw sqlite error.
func TestHandleCreateTeamDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	dropTable(t, db, "teams")

	rec := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/organizations/%d/teams", org.ID), org.ID, `{"name":"Ops"}`)

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to create team")
	assertNoInternalLeak(t, rec, "teams")
}

// RED: teams:58 — a database failure on team update must return a generic 500.
func TestHandleUpdateTeamDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	dropTable(t, db, "teams")

	rec := performJSON(t, router, http.MethodPatch, fmt.Sprintf("/api/v1/organizations/%d/teams/1", org.ID), org.ID, `{"name":"Ops"}`)

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to update team")
	assertNoInternalLeak(t, rec, "teams")
}

// RED: teams:70 — a database failure on team deletion must return a generic 500.
func TestHandleDeleteTeamDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	dropTable(t, db, "teams")

	rec := performJSON(t, router, http.MethodDelete, fmt.Sprintf("/api/v1/organizations/%d/teams/1", org.ID), org.ID, "")

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to delete team")
	assertNoInternalLeak(t, rec, "teams")
}

// RED: teams:90 — a database failure while adding a team member must return a
// generic 500.
func TestHandleAddTeamMemberDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	dropTable(t, db, "teams")

	rec := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/organizations/%d/teams/1/members", org.ID), org.ID, fmt.Sprintf(`{"user_id":%d}`, org.CreatedBy))

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to add team member")
	assertNoInternalLeak(t, rec, "teams")
}

// RED: conversation:68 — a database failure on conversation creation must
// return a generic 500.
func TestHandleCreateConversationDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	dropTable(t, db, "conversations")

	rec := performJSON(t, router, http.MethodPost, "/api/v1/conversations", org.ID, `{"type":"channel","title":"Ops"}`)

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to create conversation")
	assertNoInternalLeak(t, rec, "conversations")
}

// RED: conversation:109 — a database failure on conversation update must
// return a generic 500.
func TestHandleUpdateConversationDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	convID := createTestConversation(t, router, org.ID)
	dropTable(t, db, "conversations")

	rec := performJSON(t, router, http.MethodPatch, fmt.Sprintf("/api/v1/conversations/%d", convID), org.ID, `{"status":"pending"}`)

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to update conversation")
	assertNoInternalLeak(t, rec, "conversations")
}

// RED: conversation:165 — a database failure on message creation must return a
// generic 500.
func TestHandleCreateMessageDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	convID := createTestConversation(t, router, org.ID)
	dropTable(t, db, "messages")

	rec := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/conversations/%d/messages", convID), org.ID, `{"type":"text","body":"hello"}`)

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to create message")
	assertNoInternalLeak(t, rec, "messages")
}

// RED: organization:165 — a database failure on member role update must return
// a generic 500.
func TestHandleUpdateOrganizationMemberDBFailureReturns500WithoutLeak(t *testing.T) {
	router, db, org := newCollaborationErrorTestEnv(t, false)
	dropTable(t, db, "organization_members")

	rec := performJSON(t, router, http.MethodPatch, fmt.Sprintf("/api/v1/organizations/%d/members/%d", org.ID, org.CreatedBy), org.ID, `{"role":"admin"}`)

	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to update organization member")
	assertNoInternalLeak(t, rec, "organization_members")
}

// Guard: typed validation errors keep 400 + their own message.
func TestHandleCreateTeamKeepsTeamNameRequiredMessage(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, false)

	rec := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/organizations/%d/teams", org.ID), org.ID, `{"name":"  "}`)

	assertServiceError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "team name required")
	assertNoInternalLeak(t, rec)
}

func TestHandleCreateConversationKeepsInvalidConversationTypeMessage(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, false)

	rec := performJSON(t, router, http.MethodPost, "/api/v1/conversations", org.ID, `{"type":"bogus"}`)

	assertServiceError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid conversation type")
	assertNoInternalLeak(t, rec)
}

func TestHandleUpdateConversationKeepsInvalidConversationStatusMessage(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, false)
	convID := createTestConversation(t, router, org.ID)

	rec := performJSON(t, router, http.MethodPatch, fmt.Sprintf("/api/v1/conversations/%d", convID), org.ID, `{"status":"bogus"}`)

	assertServiceError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid conversation status")
	assertNoInternalLeak(t, rec)
}

func TestHandleUpdateOrganizationMemberKeepsInvalidRoleMessage(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, false)

	rec := performJSON(t, router, http.MethodPatch, fmt.Sprintf("/api/v1/organizations/%d/members/%d", org.ID, org.CreatedBy), org.ID, `{"role":"bogus"}`)

	assertServiceError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid role")
	assertNoInternalLeak(t, rec)
}

func TestHandleCreateMessageKeepsMessageBodyRequiredMessage(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, false)
	convID := createTestConversation(t, router, org.ID)

	rec := performJSON(t, router, http.MethodPost, fmt.Sprintf("/api/v1/conversations/%d/messages", convID), org.ID, `{"type":"text","body":"  "}`)

	assertServiceError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "message body required")
	assertNoInternalLeak(t, rec)
}
