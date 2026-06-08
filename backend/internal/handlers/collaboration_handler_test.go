package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/storage"
	"github.com/allcallall/backend/internal/user"
)

func TestCollaborationHandlerRoomOfferReturnsAnswer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "handlers.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
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
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	service := collaboration.NewService(db, userSvc)
	engine, err := media.NewEngine(zerolog.Nop(), &media.Config{WebRTCConfig: webrtc.Configuration{}})
	if err != nil {
		t.Fatalf("create media engine failed: %v", err)
	}
	service.WithMediaEngine(engine)
	handler := NewCollaborationHandler(zerolog.Nop(), service, userSvc, collaboration.NewChatHub(zerolog.Nop()))

	owner := models.User{Email: "owner@example.com", PasswordHash: "hash", DisplayName: "Owner", Status: "active"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}
	org, err := service.CreateOrganization(context.Background(), owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	roomState, err := service.CreateRoom(context.Background(), org.ID, owner.ID, collaboration.CreateRoomInput{
		Title:          "Standup",
		ParticipantIDs: []uint64{},
	})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("create client pc failed: %v", err)
	}
	defer func() { _ = clientPC.Close() }()
	if _, err := clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatalf("add transceiver failed: %v", err)
	}
	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer failed: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(clientPC)
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description failed: %v", err)
	}
	<-gatherComplete

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: owner.ID, Email: owner.Email})
		c.Next()
	})
	handler.RegisterProtectedRoutes(router.Group("/api/v1"))

	body, _ := json.Marshal(map[string]any{"sdp": clientPC.LocalDescription().SDP})
	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/rooms/%d/offer", roomState.Room.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", org.ID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var response struct {
		Answer struct {
			Type string `json:"type"`
			SDP  string `json:"sdp"`
		} `json:"answer"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.Answer.Type != "answer" {
		t.Fatalf("expected answer type, got %s", response.Answer.Type)
	}
	if response.Answer.SDP == "" {
		t.Fatal("expected non-empty answer sdp")
	}
}

func TestCollaborationHandlerUpdatesConversation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "handlers-update.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
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
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	service := collaboration.NewService(db, userSvc)
	handler := NewCollaborationHandler(zerolog.Nop(), service, userSvc, collaboration.NewChatHub(zerolog.Nop()))

	owner := models.User{Email: "owner@example.com", PasswordHash: "hash", DisplayName: "Owner", Status: "active"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}
	org, err := service.CreateOrganization(context.Background(), owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	conv, err := service.CreateConversation(context.Background(), org.ID, owner.ID, collaboration.CreateConversationInput{
		Type:  models.ConversationTypeChannel,
		Title: "Inbox",
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: owner.ID, Email: owner.Email})
		c.Next()
	})
	handler.RegisterProtectedRoutes(router.Group("/api/v1"))

	body, _ := json.Marshal(map[string]any{
		"status":   models.ConversationStatusPending,
		"priority": models.ConversationPriorityHigh,
	})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/conversations/%d", conv.ID), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", org.ID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var response struct {
		Conversation struct {
			Status   string `json:"status"`
			Priority string `json:"priority"`
		} `json:"conversation"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if response.Conversation.Status != models.ConversationStatusPending {
		t.Fatalf("expected pending status, got %s", response.Conversation.Status)
	}
	if response.Conversation.Priority != models.ConversationPriorityHigh {
		t.Fatalf("expected high priority, got %s", response.Conversation.Priority)
	}
}

func TestCollaborationHandlerSupportRoomRequiresToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "handlers-support.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
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
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	service := collaboration.NewService(db, userSvc)
	handler := NewCollaborationHandler(zerolog.Nop(), service, userSvc, collaboration.NewChatHub(zerolog.Nop()))

	owner := models.User{Email: "owner@example.com", PasswordHash: "hash", DisplayName: "Owner", Status: "active"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}
	org, err := service.CreateOrganization(context.Background(), owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	roomState, err := service.CreateRoom(context.Background(), org.ID, owner.ID, collaboration.CreateRoomInput{
		Title: "Support Review",
	})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}

	router := gin.New()
	api := router.Group("/api/v1")
	handler.RegisterInternalRoutes(api)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/internal/support/rooms/%d", roomState.Room.ID), nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without support token config, got %d body=%s", rec.Code, rec.Body.String())
	}

	t.Setenv("SUPPORT_API_TOKEN", "support-secret")
	req = httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/internal/support/rooms/%d", roomState.Room.ID), nil)
	req.Header.Set("X-Support-Token", "support-secret")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with support token, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestCollaborationHandlerDownloadRecordingWritesExportAudit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "handlers-recording-download.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
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
		&models.CallRoom{},
		&models.CallRoomMember{},
		&models.CallRoomEvent{},
		&models.RecordingSession{},
		&models.RecordingFile{},
		&models.RecordingConsent{},
		&models.RecordingExport{},
		&models.Pipeline{},
		&models.PipelineStage{},
		&models.Deal{},
		&models.DealContact{},
		&models.DealActivity{},
	); err != nil {
		t.Fatalf("auto migrate failed: %v", err)
	}

	userSvc := user.NewService(user.NewRepository(db))
	service := collaboration.NewService(db, userSvc)
	recordingRoot := filepath.Join(t.TempDir(), "recordings")
	recordingStorage, err := storage.NewRecordingStorage(storage.Config{
		Driver:    storage.DriverLocal,
		LocalRoot: recordingRoot,
	})
	if err != nil {
		t.Fatalf("create recording storage failed: %v", err)
	}
	service.WithRecordingStorage(recordingStorage)
	handler := NewCollaborationHandler(zerolog.Nop(), service, userSvc, collaboration.NewChatHub(zerolog.Nop()))

	owner := models.User{Email: "owner@example.com", PasswordHash: "hash", DisplayName: "Owner", Status: "active"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner failed: %v", err)
	}
	org, err := service.CreateOrganization(context.Background(), owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}

	objectKey := filepath.Join("org-1", "meeting.ogg")
	recordingPath := filepath.Join(recordingRoot, objectKey)
	if err := os.MkdirAll(filepath.Dir(recordingPath), 0o755); err != nil {
		t.Fatalf("create recording fixture dir failed: %v", err)
	}
	if err := os.WriteFile(recordingPath, []byte("recording-data"), 0o644); err != nil {
		t.Fatalf("write recording fixture failed: %v", err)
	}
	session := models.RecordingSession{
		OrganizationID: org.ID,
		RoomID:         42,
		StartedBy:      owner.ID,
		Status:         models.RecordingStatusStopped,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create recording session failed: %v", err)
	}
	file := models.RecordingFile{
		RecordingSessionID: session.ID,
		StorageDriver:      "local",
		ObjectKey:          objectKey,
		ContentType:        "audio/ogg",
		FileSizeBytes:      int64(len("recording-data")),
	}
	if err := db.Create(&file).Error; err != nil {
		t.Fatalf("create recording file failed: %v", err)
	}

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: owner.ID, Email: owner.Email})
		c.Next()
	})
	handler.RegisterProtectedRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/recordings/%d/files/%d", session.ID, file.ID), nil)
	req.Header.Set("X-Organization-ID", fmt.Sprintf("%d", org.ID))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 download, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "recording-data" {
		t.Fatalf("unexpected download body: %q", rec.Body.String())
	}
	if rec.Header().Get("Content-Disposition") == "" {
		t.Fatal("expected content-disposition header")
	}

	var audit models.RecordingExport
	if err := db.Where("recording_session_id = ?", session.ID).Take(&audit).Error; err != nil {
		t.Fatalf("load recording export audit failed: %v", err)
	}
	if audit.RequestedBy != owner.ID {
		t.Fatalf("expected audit requested_by %d, got %d", owner.ID, audit.RequestedBy)
	}
	if audit.DownloadCount != 1 {
		t.Fatalf("expected audit download count 1, got %d", audit.DownloadCount)
	}
	if audit.Status != "completed" {
		t.Fatalf("expected completed audit status, got %q", audit.Status)
	}
}
