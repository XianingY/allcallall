package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/user"
)

// newCollaborationErrorTestEnv builds an owner + organization on sqlite.
// When recordingSessions is false the recording_sessions table is absent, so
// recording queries fail the way a genuine database outage would.
// The returned db handle lets other tests drop tables to simulate outages on
// any collaboration endpoint.
func newCollaborationErrorTestEnv(t *testing.T, recordingSessions bool) (*gin.Engine, *gorm.DB, *models.Organization) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "recordings.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if sqlDB, dbErr := db.DB(); dbErr == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	tables := []any{
		&models.User{},
		&models.Organization{},
		&models.OrganizationMember{},
		&models.OrganizationPolicy{},
		&models.Team{},
		&models.TeamMember{},
		&models.Conversation{},
		&models.ConversationNote{},
		&models.ConversationMember{},
		&models.Message{},
		&models.MessageRead{},
		&models.ChatEvent{},
		&models.Attachment{},
		&models.MessageReaction{},
		&models.ConversationPin{},
		&models.OrganizationAuditEvent{},
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
		&models.EventOutbox{},
	}
	if recordingSessions {
		tables = append(tables, &models.RecordingSession{})
	}
	if err := db.AutoMigrate(tables...); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	service := collaboration.NewService(db, userSvc)
	handler := NewCollaborationHandler(zerolog.Nop(), service, userSvc, collaboration.NewChatHub(nil, zerolog.Nop()), nil)

	owner := models.User{Email: "owner@example.com", PasswordHash: "hash", DisplayName: "Owner", Status: "active"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}
	org, err := service.CreateOrganization(context.Background(), owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: owner.ID, Email: owner.Email})
		c.Next()
	})
	handler.RegisterProtectedRoutes(router.Group("/api/v1"))
	return router, db, org
}

// RED: site 4 — a genuine database failure on the recording list must be a 500
// with a generic message, not a 400 echoing the raw SQL error.
func TestHandleListRecordingsDBFailureReturns500WithoutLeak(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, false)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil)
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", org.ID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for recording list DB failure, got %d body=%s", rec.Code, rec.Body.String())
	}

	body := rec.Body.String()
	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, body)
	}
	if resp.Code != "RECORDING_LIST_FAILED" {
		t.Fatalf("expected code RECORDING_LIST_FAILED, got %q", resp.Code)
	}
	if resp.Error != "failed to list recordings" {
		t.Fatalf("expected generic message, got %q", resp.Error)
	}
	if strings.Contains(body, "no such table") || strings.Contains(body, "recording_sessions") {
		t.Fatalf("response leaked internal DB error: %s", body)
	}
}

// RED: site 4 — not-found on a single recording must be a distinct 404 with a
// clean message, not a generic 400.
func TestHandleGetRecordingNotFoundReturns404(t *testing.T) {
	router, _, org := newCollaborationErrorTestEnv(t, true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/recordings/999999", nil)
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", org.ID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing recording, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response failed: %v body=%s", err, rec.Body.String())
	}
	if resp.Code != "RECORDING_NOT_FOUND" {
		t.Fatalf("expected code RECORDING_NOT_FOUND, got %q", resp.Code)
	}
	if resp.Error != "recording not found" {
		t.Fatalf("expected endpoint-specific not-found message, got %q", resp.Error)
	}
}
