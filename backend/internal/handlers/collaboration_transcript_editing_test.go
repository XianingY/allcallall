package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/testutil"
	"github.com/allcallall/backend/internal/user"
)

type transcriptEditingHandlerEnv struct {
	router  *gin.Engine
	db      *gorm.DB
	org     *models.Organization
	owner   models.User
	session models.RecordingSession
	segment models.MeetingTranscriptSegment
}

func newTranscriptEditingHandlerEnv(t *testing.T) *transcriptEditingHandlerEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testutil.OpenSQLite(t, "transcript-editing.db")
	testutil.AutoMigrateAll(t, db)

	userSvc := user.NewService(user.NewRepository(db))
	service := collaboration.NewService(db, userSvc)
	handler := NewCollaborationHandler(zerolog.Nop(), service, userSvc, collaboration.NewChatHub(nil, zerolog.Nop()), nil)

	env := &transcriptEditingHandlerEnv{db: db}
	env.owner = models.User{Email: "owner@example.com", PasswordHash: "hash", DisplayName: "Owner", Status: "active"}
	if err := db.Create(&env.owner).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}
	member := models.User{Email: "member@example.com", PasswordHash: "hash", DisplayName: "Member", Status: "active"}
	if err := db.Create(&member).Error; err != nil {
		t.Fatalf("create member failed: %v", err)
	}
	org, err := service.CreateOrganization(context.Background(), env.owner.ID, "Transcript Handler Org")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	env.org = org
	if err := db.Create(&models.OrganizationMember{
		OrganizationID: org.ID,
		UserID:         member.ID,
		Role:           models.OrganizationRoleMember,
		JoinedAt:       time.Now(),
	}).Error; err != nil {
		t.Fatalf("create member failed: %v", err)
	}

	conversation := models.Conversation{
		OrganizationID: org.ID,
		Type:           models.ConversationTypeChannel,
		Title:          "Handler transcript conversation",
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityNormal,
		CreatedBy:      env.owner.ID,
	}
	if err := db.Create(&conversation).Error; err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}
	now := time.Now().UTC()
	room := models.CallRoom{
		OrganizationID: org.ID,
		ConversationID: &conversation.ID,
		Title:          "Handler room",
		Status:         models.RoomStatusEnded,
		CreatedBy:      env.owner.ID,
	}
	if err := db.Create(&room).Error; err != nil {
		t.Fatalf("create room failed: %v", err)
	}
	env.session = models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         room.ID,
		StartedBy:      env.owner.ID,
		Status:         models.RecordingStatusStopped,
		StartedAt:      &now,
		StoppedAt:      &now,
	}
	if err := db.Create(&env.session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	// owner 既是 admin 也是说话人，走最小可编辑路径；普通成员改他人段的
	// 403 已在服务层权限矩阵覆盖。
	env.segment = models.MeetingTranscriptSegment{
		OrganizationID:     org.ID,
		ConversationID:     conversation.ID,
		RoomID:             room.ID,
		RecordingSessionID: env.session.ID,
		RecordingFileID:    1,
		SpeakerUserID:      &env.owner.ID,
		Source:             models.MeetingTranscriptSourceRecording,
		Provider:           "test",
		Text:               "Helo worlld",
		StartMS:            100,
		EndMS:              900,
	}
	if err := db.Create(&env.segment).Error; err != nil {
		t.Fatalf("create segment failed: %v", err)
	}

	env.router = gin.New()
	env.router.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: env.owner.ID, Email: env.owner.Email})
		c.Next()
	})
	handler.RegisterProtectedRoutes(env.router.Group("/api/v1"))
	return env
}

func (e *transcriptEditingHandlerEnv) patchSegment(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch,
		fmt.Sprintf("/api/v1/recordings/%d/transcript/segments/%d", e.session.ID, e.segment.ID),
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", e.org.ID))
	rec := httptest.NewRecorder()
	e.router.ServeHTTP(rec, req)
	return rec
}

func TestHandleEditRecordingTranscriptSegment(t *testing.T) {
	env := newTranscriptEditingHandlerEnv(t)

	rec := env.patchSegment(t, `{"text":"Hello world","reason":"fix typo"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var updated struct {
		Segment        models.MeetingTranscriptSegment `json:"segment"`
		RevisionNumber int                             `json:"revision_number"`
		Edited         bool                            `json:"edited"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !updated.Edited || updated.Segment.Text != "Hello world" || updated.RevisionNumber != 1 {
		t.Fatalf("unexpected edit response: %+v", updated)
	}

	// 相同文本是 200 no-op，不新增修订。
	noOp := env.patchSegment(t, `{"text":"Hello world"}`)
	if noOp.Code != http.StatusOK {
		t.Fatalf("expected 200 no-op, got %d body=%s", noOp.Code, noOp.Body.String())
	}
	var noOpBody struct {
		Edited bool `json:"edited"`
	}
	if err := json.Unmarshal(noOp.Body.Bytes(), &noOpBody); err != nil {
		t.Fatalf("decode no-op response failed: %v", err)
	}
	if noOpBody.Edited {
		t.Fatalf("no-op must report edited=false: %s", noOp.Body.String())
	}

	missing := httptest.NewRequest(http.MethodPatch, "/api/v1/recordings/999999/transcript/segments/999999",
		strings.NewReader(`{"text":"x"}`))
	missing.Header.Set("Content-Type", "application/json")
	missing.Header.Set("X-Organization-ID", fmt.Sprintf("%d", env.org.ID))
	missingRec := httptest.NewRecorder()
	env.router.ServeHTTP(missingRec, missing)
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	invalid := env.patchSegment(t, `{"text":"   "}`)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blank text, got %d body=%s", invalid.Code, invalid.Body.String())
	}
}

func TestHandleListTranscriptSegmentRevisions(t *testing.T) {
	env := newTranscriptEditingHandlerEnv(t)
	for _, text := range []string{"one", "two"} {
		rec := env.patchSegment(t, fmt.Sprintf(`{"text":%q}`, text))
		if rec.Code != http.StatusOK {
			t.Fatalf("edit %q failed: %d body=%s", text, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/v1/recordings/%d/transcript/segments/%d/revisions?limit=1", env.session.ID, env.segment.ID), nil)
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", env.org.ID))
	rec := httptest.NewRecorder()
	env.router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var page collaboration.TranscriptSegmentRevisionPage
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode revisions failed: %v", err)
	}
	if len(page.Revisions) != 1 || page.Revisions[0].NextText != "two" || page.NextBeforeID == nil {
		t.Fatalf("unexpected revisions page: %+v", page)
	}
}
