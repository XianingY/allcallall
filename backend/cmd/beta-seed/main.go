package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/config"
	"github.com/allcallall/backend/internal/logger"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/runtime"
)

const (
	defaultOwnerEmail  = "beta.owner@example.com"
	defaultMemberEmail = "beta.member@example.com"
	defaultGuestEmail  = "beta.guest@example.com"
	defaultPassword    = "Beta123456!"
)

type seedUser struct {
	ID       uint64 `json:"id"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

type seedOutput struct {
	OrganizationID         uint64     `json:"organization_id"`
	TeamID                 uint64     `json:"team_id"`
	ConversationID         uint64     `json:"conversation_id"`
	RoomID                 uint64     `json:"room_id"`
	RecordingSessionID     uint64     `json:"recording_session_id"`
	RecordingFileID        uint64     `json:"recording_file_id"`
	TranscriptSegmentCount int64      `json:"transcript_segment_count"`
	PendingInviteCode      string     `json:"pending_invite_code"`
	Users                  []seedUser `json:"users"`
	SuggestedRoutes        []string   `json:"suggested_routes"`
	Notes                  []string   `json:"notes"`
}

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("load config failed: %v", err))
	}
	log := logger.New(cfg.Logging.Level)
	db, cleanup, err := runtime.OpenDB(cfg, log)
	if err != nil {
		log.Fatal().Err(err).Msg("connect database failed")
	}
	defer cleanup()

	if strings.TrimSpace(os.Getenv("BETA_SEED_SKIP_MIGRATION")) == "" {
		if err := runtime.RunMigrations(db); err != nil {
			log.Fatal().Err(err).Msg("run migrations failed")
		}
	}

	out, err := seedBeta(ctx, db)
	if err != nil {
		log.Fatal().Err(err).Msg("seed beta data failed")
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		log.Fatal().Err(err).Msg("write beta seed output failed")
	}
}

func seedBeta(ctx context.Context, db *gorm.DB) (*seedOutput, error) {
	password := envOrDefault("BETA_SEED_PASSWORD", defaultPassword)
	owner, err := firstOrCreateUser(ctx, db, envOrDefault("BETA_SEED_OWNER_EMAIL", defaultOwnerEmail), "Beta Owner", password)
	if err != nil {
		return nil, err
	}
	member, err := firstOrCreateUser(ctx, db, envOrDefault("BETA_SEED_MEMBER_EMAIL", defaultMemberEmail), "Beta Member", password)
	if err != nil {
		return nil, err
	}
	org, err := firstOrCreateOrganization(ctx, db, owner.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureOrganizationMember(ctx, db, org.ID, owner.ID, models.OrganizationRoleOwner); err != nil {
		return nil, err
	}
	if err := ensureOrganizationMember(ctx, db, org.ID, member.ID, models.OrganizationRoleMember); err != nil {
		return nil, err
	}
	team, err := firstOrCreateTeam(ctx, db, org.ID, owner.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureTeamMember(ctx, db, team.ID, owner.ID, models.OrganizationRoleOwner); err != nil {
		return nil, err
	}
	if err := ensureTeamMember(ctx, db, team.ID, member.ID, models.OrganizationRoleMember); err != nil {
		return nil, err
	}
	if err := ensureOrganizationPolicy(ctx, db, org.ID); err != nil {
		return nil, err
	}
	invite, err := ensurePendingInvite(ctx, db, org.ID, team.ID, owner.ID, envOrDefault("BETA_SEED_GUEST_EMAIL", defaultGuestEmail))
	if err != nil {
		return nil, err
	}
	conversation, err := firstOrCreateConversation(ctx, db, org.ID, team.ID, owner.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureConversationMember(ctx, db, conversation.ID, owner.ID, models.OrganizationRoleOwner); err != nil {
		return nil, err
	}
	if err := ensureConversationMember(ctx, db, conversation.ID, member.ID, models.OrganizationRoleMember); err != nil {
		return nil, err
	}
	messages, err := ensureConversationMessages(ctx, db, org.ID, conversation.ID, owner.ID, member.ID)
	if err != nil {
		return nil, err
	}
	if len(messages) > 1 {
		if err := ensurePinnedMessage(ctx, db, org.ID, conversation.ID, messages[1].ID, owner.ID); err != nil {
			return nil, err
		}
	}
	if len(messages) > 0 {
		if err := ensureReaction(ctx, db, org.ID, conversation.ID, messages[0].ID, member.ID, "👍"); err != nil {
			return nil, err
		}
	}
	if err := ensureConversationNote(ctx, db, org.ID, conversation.ID, owner.ID); err != nil {
		return nil, err
	}
	if err := ensureContactProfile(ctx, db, org.ID, owner.ID, member.ID); err != nil {
		return nil, err
	}
	room, err := firstOrCreateRoom(ctx, db, org.ID, team.ID, conversation.ID, owner.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureRoomMember(ctx, db, room.ID, owner.ID, models.OrganizationRoleOwner); err != nil {
		return nil, err
	}
	if err := ensureRoomMember(ctx, db, room.ID, member.ID, models.OrganizationRoleMember); err != nil {
		return nil, err
	}
	recording, file, segmentCount, err := ensureReadyTranscript(ctx, db, org.ID, conversation.ID, room.ID, owner.ID, member.ID)
	if err != nil {
		return nil, err
	}
	if err := ensureTranscriptReadyMessage(ctx, db, org.ID, conversation.ID, owner.ID, recording.ID); err != nil {
		return nil, err
	}
	if err := ensureAuditEvents(ctx, db, org.ID, owner.ID, team.ID, invite.ID); err != nil {
		return nil, err
	}

	return &seedOutput{
		OrganizationID:         org.ID,
		TeamID:                 team.ID,
		ConversationID:         conversation.ID,
		RoomID:                 room.ID,
		RecordingSessionID:     recording.ID,
		RecordingFileID:        file.ID,
		TranscriptSegmentCount: segmentCount,
		PendingInviteCode:      invite.Code,
		Users: []seedUser{
			{ID: owner.ID, Email: owner.Email, Password: password, Role: models.OrganizationRoleOwner},
			{ID: member.ID, Email: member.Email, Password: password, Role: models.OrganizationRoleMember},
		},
		SuggestedRoutes: []string{
			fmt.Sprintf("/organizations?organizationId=%d", org.ID),
			fmt.Sprintf("/conversations/%d", conversation.ID),
			fmt.Sprintf("/recordings/%d", recording.ID),
			fmt.Sprintf("/agent-lab?conversationId=%d&preset=meeting_brief", conversation.ID),
		},
		Notes: []string{
			"The seeded recording has ready transcript metadata and segments for UI/Agent grounding.",
			"No real audio bytes are generated; use a real meeting recording for download/ASR smoke.",
			"Run with AGENT_PROVIDER=rules for deterministic local meeting brief demos.",
		},
	}, nil
}

func firstOrCreateUser(ctx context.Context, db *gorm.DB, email, displayName, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	user := models.User{Email: strings.ToLower(strings.TrimSpace(email))}
	err = db.WithContext(ctx).
		Where("email = ?", user.Email).
		Assign(models.User{DisplayName: displayName, PasswordHash: string(hash), Status: models.UserStatusActive}).
		FirstOrCreate(&user).Error
	return &user, err
}

func firstOrCreateOrganization(ctx context.Context, db *gorm.DB, ownerID uint64) (*models.Organization, error) {
	org := models.Organization{Slug: "beta-team-demo"}
	err := db.WithContext(ctx).
		Where("slug = ?", org.Slug).
		Assign(models.Organization{
			Name:        "AllCallAll Beta Team",
			Description: "Small-team Beta workspace for chat, meetings, transcripts, and Agent recap demos.",
			CreatedBy:   ownerID,
		}).
		FirstOrCreate(&org).Error
	return &org, err
}

func ensureOrganizationMember(ctx context.Context, db *gorm.DB, organizationID, userID uint64, role string) error {
	member := models.OrganizationMember{OrganizationID: organizationID, UserID: userID}
	return db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", organizationID, userID).
		Assign(models.OrganizationMember{Role: role, JoinedAt: time.Now().UTC()}).
		FirstOrCreate(&member).Error
}

func firstOrCreateTeam(ctx context.Context, db *gorm.DB, organizationID, ownerID uint64) (*models.Team, error) {
	team := models.Team{OrganizationID: organizationID, Slug: "hardware-beta"}
	err := db.WithContext(ctx).
		Where("organization_id = ? AND slug = ?", organizationID, team.Slug).
		Assign(models.Team{
			Name:        "Hardware Beta",
			Description: "Cross-functional test team for meeting recap and collaboration flows.",
			CreatedBy:   ownerID,
		}).
		FirstOrCreate(&team).Error
	return &team, err
}

func ensureTeamMember(ctx context.Context, db *gorm.DB, teamID, userID uint64, role string) error {
	member := models.TeamMember{TeamID: teamID, UserID: userID}
	return db.WithContext(ctx).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Assign(models.TeamMember{Role: role, JoinedAt: time.Now().UTC()}).
		FirstOrCreate(&member).Error
}

func ensureOrganizationPolicy(ctx context.Context, db *gorm.DB, organizationID uint64) error {
	policy := models.OrganizationPolicy{OrganizationID: organizationID}
	return db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Assign(models.OrganizationPolicy{
			RecordingMode:          models.RecordingModeAdminOptIn,
			RecordingStorageDays:   30,
			RecordingExportAllowed: true,
		}).
		FirstOrCreate(&policy).Error
}

func ensurePendingInvite(ctx context.Context, db *gorm.DB, organizationID, teamID, ownerID uint64, email string) (*models.OrganizationInvite, error) {
	teamIDCopy := teamID
	invite := models.OrganizationInvite{Code: "beta-team-demo-invite"}
	err := db.WithContext(ctx).
		Where("code = ?", invite.Code).
		Assign(models.OrganizationInvite{
			OrganizationID: organizationID,
			TeamID:         &teamIDCopy,
			InviterID:      ownerID,
			TargetEmail:    strings.ToLower(strings.TrimSpace(email)),
			Role:           models.OrganizationRoleMember,
			Status:         models.InvitationStatusPending,
			ExpiresAt:      time.Now().UTC().Add(7 * 24 * time.Hour),
		}).
		FirstOrCreate(&invite).Error
	return &invite, err
}

func firstOrCreateConversation(ctx context.Context, db *gorm.DB, organizationID, teamID, ownerID uint64) (*models.Conversation, error) {
	teamIDCopy := teamID
	ownerIDCopy := ownerID
	conversation := models.Conversation{OrganizationID: organizationID, Title: "Beta Launch Sync"}
	err := db.WithContext(ctx).
		Where("organization_id = ? AND title = ?", organizationID, conversation.Title).
		Assign(models.Conversation{
			TeamID:         &teamIDCopy,
			Type:           models.ConversationTypeChannel,
			Topic:          "Prepare the small-team Beta trial and validate meeting recap workflow.",
			Status:         models.ConversationStatusOpen,
			AssigneeUserID: &ownerIDCopy,
			Priority:       models.ConversationPriorityHigh,
			CreatedBy:      ownerID,
		}).
		FirstOrCreate(&conversation).Error
	return &conversation, err
}

func ensureConversationMember(ctx context.Context, db *gorm.DB, conversationID, userID uint64, role string) error {
	member := models.ConversationMember{ConversationID: conversationID, UserID: userID}
	return db.WithContext(ctx).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Assign(models.ConversationMember{Role: role}).
		FirstOrCreate(&member).Error
}

func ensureConversationMessages(ctx context.Context, db *gorm.DB, organizationID, conversationID, ownerID, memberID uint64) ([]models.Message, error) {
	seeds := []struct {
		SenderID uint64
		Body     string
	}{
		{ownerID, "本周目标：验证 3-6 人团队的聊天、会议录音转写和 Agent 会议复盘主链路。"},
		{memberID, "硬件测试侧担心摄像头模组供应延迟会影响下周联调，需要在复盘中标出风险。"},
		{ownerID, "请把行动项落到责任人：测试环境、供应商确认、下次会议时间。"},
	}
	messages := make([]models.Message, 0, len(seeds))
	var lastMessageAt *time.Time
	for _, seed := range seeds {
		message := models.Message{OrganizationID: organizationID, ConversationID: conversationID, Body: seed.Body}
		if err := db.WithContext(ctx).
			Where("organization_id = ? AND conversation_id = ? AND body = ?", organizationID, conversationID, seed.Body).
			Assign(models.Message{SenderID: seed.SenderID, Type: models.MessageTypeText}).
			FirstOrCreate(&message).Error; err != nil {
			return nil, err
		}
		messages = append(messages, message)
		messageTime := message.CreatedAt
		if lastMessageAt == nil || messageTime.After(*lastMessageAt) {
			lastMessageAt = &messageTime
		}
	}
	if lastMessageAt != nil {
		if err := db.WithContext(ctx).Model(&models.Conversation{}).
			Where("id = ?", conversationID).
			Update("last_message_at", *lastMessageAt).Error; err != nil {
			return nil, err
		}
	}
	return messages, nil
}

func ensurePinnedMessage(ctx context.Context, db *gorm.DB, organizationID, conversationID, messageID, ownerID uint64) error {
	pin := models.ConversationPin{OrganizationID: organizationID, ConversationID: conversationID, MessageID: messageID}
	return db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND message_id = ?", organizationID, conversationID, messageID).
		Assign(models.ConversationPin{PinnedBy: ownerID}).
		FirstOrCreate(&pin).Error
}

func ensureReaction(ctx context.Context, db *gorm.DB, organizationID, conversationID, messageID, userID uint64, emoji string) error {
	reaction := models.MessageReaction{OrganizationID: organizationID, ConversationID: conversationID, MessageID: messageID, UserID: userID, Emoji: emoji}
	return db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND message_id = ? AND user_id = ? AND emoji = ?", organizationID, conversationID, messageID, userID, emoji).
		FirstOrCreate(&reaction).Error
}

func ensureConversationNote(ctx context.Context, db *gorm.DB, organizationID, conversationID, ownerID uint64) error {
	note := models.ConversationNote{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		AuthorID:       ownerID,
		Body:           "Beta 验收重点：转写 ready 后才能生成基于会议录音的复盘；写回消息、follow-up 和 memory 必须进入审批。",
	}
	return db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND body = ?", organizationID, conversationID, note.Body).
		FirstOrCreate(&note).Error
}

func ensureContactProfile(ctx context.Context, db *gorm.DB, organizationID, ownerID, memberID uint64) error {
	profile := models.ContactProfile{OrganizationID: organizationID, OwnerID: ownerID, ContactUserID: memberID}
	return db.WithContext(ctx).
		Where("organization_id = ? AND owner_id = ? AND contact_user_id = ?", organizationID, ownerID, memberID).
		Assign(models.ContactProfile{
			Company:            "Beta Hardware Team",
			Role:               "QA Lead",
			Timezone:           "Asia/Shanghai",
			RelationshipStatus: "active",
			Note:               "关注测试环境稳定性、供应链风险和明确行动项。",
		}).
		FirstOrCreate(&profile).Error
}

func firstOrCreateRoom(ctx context.Context, db *gorm.DB, organizationID, teamID, conversationID, ownerID uint64) (*models.CallRoom, error) {
	teamIDCopy := teamID
	conversationIDCopy := conversationID
	now := time.Now().UTC().Add(-45 * time.Minute)
	ended := now.Add(28 * time.Minute)
	room := models.CallRoom{OrganizationID: organizationID, ConversationID: &conversationIDCopy, Title: "Beta Launch Review"}
	err := db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND title = ?", organizationID, conversationID, room.Title).
		Assign(models.CallRoom{
			TeamID:    &teamIDCopy,
			Status:    models.RoomStatusEnded,
			CreatedBy: ownerID,
			StartedAt: &now,
			EndedAt:   &ended,
		}).
		FirstOrCreate(&room).Error
	if err != nil {
		return nil, err
	}
	if err := db.WithContext(ctx).Model(&models.Conversation{}).
		Where("id = ?", conversationID).
		Update("room_id", room.ID).Error; err != nil {
		return nil, err
	}
	event := models.CallRoomEvent{RoomID: room.ID, UserID: ownerID, Type: "meeting.ended", PayloadJSON: `{"status":"ended","seed":"beta"}`}
	if err := db.WithContext(ctx).
		Where("room_id = ? AND type = ?", room.ID, event.Type).
		FirstOrCreate(&event).Error; err != nil {
		return nil, err
	}
	return &room, nil
}

func ensureRoomMember(ctx context.Context, db *gorm.DB, roomID, userID uint64, role string) error {
	joined := time.Now().UTC().Add(-45 * time.Minute)
	left := joined.Add(28 * time.Minute)
	member := models.CallRoomMember{RoomID: roomID, UserID: userID}
	return db.WithContext(ctx).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Assign(models.CallRoomMember{Role: role, JoinedAt: &joined, LeftAt: &left}).
		FirstOrCreate(&member).Error
}

func ensureReadyTranscript(ctx context.Context, db *gorm.DB, organizationID, conversationID, roomID, ownerID, memberID uint64) (*models.RecordingSession, *models.RecordingFile, int64, error) {
	started := time.Now().UTC().Add(-45 * time.Minute)
	stopped := started.Add(28 * time.Minute)
	session := models.RecordingSession{OrganizationID: organizationID, RoomID: roomID, StartedBy: ownerID, Status: models.RecordingStatusStopped}
	if err := db.WithContext(ctx).
		Where("organization_id = ? AND room_id = ? AND started_by = ? AND status = ?", organizationID, roomID, ownerID, models.RecordingStatusStopped).
		Assign(models.RecordingSession{StartedAt: &started, StoppedAt: &stopped}).
		FirstOrCreate(&session).Error; err != nil {
		return nil, nil, 0, err
	}
	file := models.RecordingFile{RecordingSessionID: session.ID, ObjectKey: fmt.Sprintf("beta-seed/recording-%d.ogg", session.ID)}
	if err := db.WithContext(ctx).
		Where("recording_session_id = ? AND object_key = ?", session.ID, file.ObjectKey).
		Assign(models.RecordingFile{
			StorageDriver:   "local",
			ContentType:     "audio/ogg",
			FileSizeBytes:   1024,
			DurationSeconds: 1680,
			MetadataJSON:    `{"seed":"beta","note":"metadata only; no real audio bytes generated"}`,
		}).
		FirstOrCreate(&file).Error; err != nil {
		return nil, nil, 0, err
	}
	now := time.Now().UTC()
	conversationIDCopy := conversationID
	transcription := models.RecordingTranscription{RecordingSessionID: session.ID}
	if err := db.WithContext(ctx).
		Where("recording_session_id = ?", session.ID).
		Assign(models.RecordingTranscription{
			OrganizationID: organizationID,
			ConversationID: &conversationIDCopy,
			RoomID:         roomID,
			Status:         models.RecordingTranscriptionStatusReady,
			Provider:       "beta_seed",
			SegmentCount:   4,
			StartedAt:      &now,
			CompletedAt:    &now,
		}).
		FirstOrCreate(&transcription).Error; err != nil {
		return nil, nil, 0, err
	}
	segments := []struct {
		SpeakerID uint64
		TrackKey  string
		StartMS   int64
		EndMS     int64
		Text      string
	}{
		{ownerID, "owner-audio", 0, 22000, "今天复盘 Beta 主链路：组织邀请、聊天协作、会议录音转写和 Agent 会议复盘。"},
		{memberID, "member-audio", 22000, 48000, "风险点是摄像头模组供应可能延期，如果周三前不确认，会影响下周联调。"},
		{ownerID, "owner-audio", 48000, 76000, "行动项一：测试负责人今晚补齐浏览器会议 smoke；行动项二：供应链明天确认模组 ETA。"},
		{memberID, "member-audio", 76000, 98000, "下次会议建议安排在周五上午，重点检查转写引用跳转和审批写回。"},
	}
	for _, seed := range segments {
		speakerID := seed.SpeakerID
		segment := models.MeetingTranscriptSegment{
			OrganizationID:     organizationID,
			ConversationID:     conversationID,
			RoomID:             roomID,
			RecordingSessionID: session.ID,
			RecordingFileID:    file.ID,
			StartMS:            seed.StartMS,
		}
		if err := db.WithContext(ctx).
			Where("recording_session_id = ? AND start_ms = ? AND track_key = ?", session.ID, seed.StartMS, seed.TrackKey).
			Assign(models.MeetingTranscriptSegment{
				OrganizationID:  organizationID,
				ConversationID:  conversationID,
				RoomID:          roomID,
				RecordingFileID: file.ID,
				SpeakerUserID:   &speakerID,
				TrackKey:        seed.TrackKey,
				Source:          models.MeetingTranscriptSourceRecording,
				Provider:        "beta_seed",
				Language:        "zh",
				Text:            seed.Text,
				EndMS:           seed.EndMS,
				Confidence:      0.98,
			}).
			FirstOrCreate(&segment).Error; err != nil {
			return nil, nil, 0, err
		}
	}
	var segmentCount int64
	if err := db.WithContext(ctx).Model(&models.MeetingTranscriptSegment{}).
		Where("recording_session_id = ?", session.ID).
		Count(&segmentCount).Error; err != nil {
		return nil, nil, 0, err
	}
	if err := db.WithContext(ctx).Model(&models.RecordingTranscription{}).
		Where("recording_session_id = ?", session.ID).
		Updates(map[string]any{"segment_count": segmentCount, "status": models.RecordingTranscriptionStatusReady, "updated_at": time.Now().UTC()}).Error; err != nil {
		return nil, nil, 0, err
	}
	return &session, &file, segmentCount, nil
}

func ensureTranscriptReadyMessage(ctx context.Context, db *gorm.DB, organizationID, conversationID, ownerID, recordingSessionID uint64) error {
	metadata := fmt.Sprintf(`{"event":"meeting.transcription.ready","recording_session_id":%d,"seed":"beta"}`, recordingSessionID)
	message := models.Message{OrganizationID: organizationID, ConversationID: conversationID, Type: models.MessageTypeSystem, MetadataJSON: metadata}
	err := db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND metadata_json = ?", organizationID, conversationID, metadata).
		Assign(models.Message{
			SenderID: ownerID,
			Body:     "会议录音转写已完成，可在转写时间轴中查看并生成会议复盘。",
		}).
		FirstOrCreate(&message).Error
	if err != nil {
		return err
	}
	return db.WithContext(ctx).Model(&models.Conversation{}).
		Where("id = ?", conversationID).
		Update("last_message_at", message.CreatedAt).Error
}

func ensureAuditEvents(ctx context.Context, db *gorm.DB, organizationID, ownerID, teamID, inviteID uint64) error {
	events := []models.OrganizationAuditEvent{
		{OrganizationID: organizationID, ActorUserID: ownerID, Action: "beta.seed.created", TargetType: "organization", TargetID: fmt.Sprint(organizationID), MetadataJSON: `{"seed":"beta"}`},
		{OrganizationID: organizationID, ActorUserID: ownerID, Action: "organization.team.created", TargetType: "team", TargetID: fmt.Sprint(teamID), MetadataJSON: `{"source":"beta_seed"}`},
		{OrganizationID: organizationID, ActorUserID: ownerID, Action: "organization.invite.created", TargetType: "invite", TargetID: fmt.Sprint(inviteID), MetadataJSON: `{"source":"beta_seed"}`},
	}
	for _, event := range events {
		if err := db.WithContext(ctx).
			Where("organization_id = ? AND action = ? AND target_type = ? AND target_id = ?", event.OrganizationID, event.Action, event.TargetType, event.TargetID).
			FirstOrCreate(&event).Error; err != nil {
			return err
		}
	}
	return nil
}

func envOrDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
