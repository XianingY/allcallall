package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) listMemberships(ctx context.Context, userID uint64) ([]currentOrgMember, error) {
	var rows []currentOrgMember
	err := s.db.WithContext(ctx).
		Table("organizations").
		Select("organizations.*, organization_members.role AS role").
		Joins("JOIN organization_members ON organization_members.organization_id = organizations.id").
		Where("organization_members.user_id = ?", userID).
		Order("organizations.id ASC").
		Find(&rows).Error
	return rows, err
}

func (s *Service) findSharedOrganizationID(ctx context.Context, firstUserID, secondUserID uint64) (uint64, error) {
	var memberships []models.OrganizationMember
	if err := s.db.WithContext(ctx).
		Where("user_id IN ?", []uint64{firstUserID, secondUserID}).
		Order("organization_id ASC").
		Find(&memberships).Error; err != nil {
		return 0, err
	}
	counts := make(map[uint64]int, len(memberships))
	for _, membership := range memberships {
		counts[membership.OrganizationID]++
	}
	for _, membership := range memberships {
		if counts[membership.OrganizationID] >= 2 {
			return membership.OrganizationID, nil
		}
	}
	return 0, ErrOrganizationAccessDenied
}

func (s *Service) findDirectConversationTx(ctx context.Context, tx *gorm.DB, organizationID uint64, memberIDs []uint64) *models.Conversation {
	if len(memberIDs) != 2 {
		return nil
	}
	var conversations []models.Conversation
	if err := tx.WithContext(ctx).
		Where("organization_id = ? AND type = ?", organizationID, models.ConversationTypeDirect).
		Find(&conversations).Error; err != nil {
		return nil
	}
	sort.Slice(memberIDs, func(i, j int) bool { return memberIDs[i] < memberIDs[j] })
	for _, conv := range conversations {
		ids, err := s.listConversationMemberIDsTx(ctx, tx, conv.ID)
		if err != nil {
			continue
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		if len(ids) == 2 && ids[0] == memberIDs[0] && ids[1] == memberIDs[1] {
			found := conv
			return &found
		}
	}
	return nil
}

func (s *Service) createMeetingConversationTx(ctx context.Context, tx *gorm.DB, organizationID, userID uint64, title string, teamID *uint64, roomID uint64, participantIDs []uint64) (*models.Conversation, error) {
	conv := &models.Conversation{
		OrganizationID: organizationID,
		TeamID:         teamID,
		RoomID:         &roomID,
		Type:           models.ConversationTypeMeeting,
		Title:          defaultString(strings.TrimSpace(title), "Meeting"),
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityNormal,
		CreatedBy:      userID,
	}
	if err := tx.Create(conv).Error; err != nil {
		return nil, err
	}
	for _, memberID := range uniqueUint64s(append(participantIDs, userID)) {
		member := models.ConversationMember{
			ConversationID: conv.ID,
			UserID:         memberID,
		}
		if err := tx.Create(&member).Error; err != nil {
			return nil, err
		}
	}
	return conv, nil
}

func (s *Service) ensureConversationMember(ctx context.Context, organizationID, userID, conversationID uint64) error {
	var count int64
	err := s.db.WithContext(ctx).
		Table("conversation_members").
		Joins("JOIN conversations ON conversations.id = conversation_members.conversation_id").
		Where("conversation_members.conversation_id = ? AND conversation_members.user_id = ? AND conversations.organization_id = ?", conversationID, userID, organizationID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrConversationAccessDenied
	}
	return nil
}

func (s *Service) loadMessageRecord(ctx context.Context, messageID uint64) (*MessageRecord, error) {
	return s.loadMessageRecordForUser(ctx, messageID, 0)
}

func (s *Service) loadMessageRecordForUser(ctx context.Context, messageID, viewerID uint64) (*MessageRecord, error) {
	var record MessageRecord
	err := s.db.WithContext(ctx).
		Table("messages").
		Select("messages.*, users.email AS sender_email, users.display_name AS sender_display_name").
		Joins("JOIN users ON users.id = messages.sender_id").
		Where("messages.id = ?", messageID).
		Take(&record).Error
	if err != nil {
		return nil, err
	}
	records := []MessageRecord{record}
	if err := s.hydrateMessageRecords(ctx, viewerID, records); err != nil {
		return nil, err
	}
	return &records[0], nil
}

func (s *Service) hydrateMessageRecords(ctx context.Context, viewerID uint64, records []MessageRecord) error {
	if len(records) == 0 {
		return nil
	}
	ids := make([]uint64, 0, len(records))
	replyIDs := make([]uint64, 0, len(records))
	indexByID := make(map[uint64]int, len(records))
	for i := range records {
		ids = append(ids, records[i].ID)
		indexByID[records[i].ID] = i
		if records[i].ReplyToMessageID != nil {
			replyIDs = append(replyIDs, *records[i].ReplyToMessageID)
		}
		if records[i].DeletedAt != nil {
			records[i].Body = ""
		}
	}

	if len(replyIDs) > 0 {
		var replies []MessageRecord
		if err := s.db.WithContext(ctx).
			Table("messages").
			Select("messages.*, users.email AS sender_email, users.display_name AS sender_display_name").
			Joins("JOIN users ON users.id = messages.sender_id").
			Where("messages.id IN ?", uniqueUint64s(replyIDs)).
			Find(&replies).Error; err != nil {
			return err
		}
		replyByID := map[uint64]MessageReplyPreview{}
		for _, reply := range replies {
			body := reply.Body
			deleted := reply.DeletedAt != nil
			if deleted {
				body = ""
			}
			replyByID[reply.ID] = MessageReplyPreview{
				ID:                reply.ID,
				SenderID:          reply.SenderID,
				SenderEmail:       reply.SenderEmail,
				SenderDisplayName: reply.SenderDisplayName,
				Body:              truncate(body, 120),
				Deleted:           deleted,
			}
		}
		for i := range records {
			if records[i].ReplyToMessageID == nil {
				continue
			}
			if preview, ok := replyByID[*records[i].ReplyToMessageID]; ok {
				item := preview
				records[i].ReplyTo = &item
			}
		}
	}

	var attachments []models.Attachment
	if err := s.db.WithContext(ctx).
		Where("message_id IN ?", ids).
		Order("id ASC").
		Find(&attachments).Error; err != nil {
		return err
	}
	for _, attachment := range attachments {
		if attachment.MessageID == nil {
			continue
		}
		if idx, ok := indexByID[*attachment.MessageID]; ok {
			records[idx].Attachments = append(records[idx].Attachments, AttachmentView{
				Attachment:  attachment,
				DownloadURL: attachmentDownloadURL(attachment.ID),
			})
		}
	}

	var reactions []models.MessageReaction
	if err := s.db.WithContext(ctx).Where("message_id IN ?", ids).Find(&reactions).Error; err != nil {
		return err
	}
	reactionsByMessage := map[uint64]map[string]*MessageReactionSummary{}
	for _, reaction := range reactions {
		if reactionsByMessage[reaction.MessageID] == nil {
			reactionsByMessage[reaction.MessageID] = map[string]*MessageReactionSummary{}
		}
		summary := reactionsByMessage[reaction.MessageID][reaction.Emoji]
		if summary == nil {
			summary = &MessageReactionSummary{Emoji: reaction.Emoji}
			reactionsByMessage[reaction.MessageID][reaction.Emoji] = summary
		}
		summary.Count++
		summary.ReactedUserIDs = append(summary.ReactedUserIDs, reaction.UserID)
		if viewerID != 0 && reaction.UserID == viewerID {
			summary.ReactedByMe = true
		}
	}
	for messageID, byEmoji := range reactionsByMessage {
		idx, ok := indexByID[messageID]
		if !ok {
			continue
		}
		for _, summary := range byEmoji {
			records[idx].Reactions = append(records[idx].Reactions, *summary)
		}
		sort.Slice(records[idx].Reactions, func(i, j int) bool {
			return records[idx].Reactions[i].Emoji < records[idx].Reactions[j].Emoji
		})
	}

	var pins []models.ConversationPin
	if err := s.db.WithContext(ctx).Where("message_id IN ?", ids).Find(&pins).Error; err != nil {
		return err
	}
	for _, pin := range pins {
		if idx, ok := indexByID[pin.MessageID]; ok {
			records[idx].Pinned = true
		}
	}
	return nil
}

func (s *Service) ensureMessageAccess(ctx context.Context, organizationID, userID, conversationID, messageID uint64) error {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.Message{}).
		Where("id = ? AND organization_id = ? AND conversation_id = ?", messageID, organizationID, conversationID).
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (s *Service) listConversationMemberIDs(ctx context.Context, conversationID uint64) ([]uint64, error) {
	return s.listConversationMemberIDsTx(ctx, s.db, conversationID)
}

func (s *Service) listConversationMemberIDsTx(ctx context.Context, tx *gorm.DB, conversationID uint64) ([]uint64, error) {
	var ids []uint64
	if err := tx.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("conversation_id = ?", conversationID).
		Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Service) getDefaultPipeline(ctx context.Context, organizationID uint64) (*models.Pipeline, *uint64, error) {
	var pipeline models.Pipeline
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND is_default = ?", organizationID, true).Take(&pipeline).Error; err != nil {
		return nil, nil, err
	}
	var stage models.PipelineStage
	var stageID *uint64
	if err := s.db.WithContext(ctx).Where("pipeline_id = ?", pipeline.ID).Order("position ASC").Take(&stage).Error; err == nil {
		stageID = &stage.ID
	}
	return &pipeline, stageID, nil
}

func (s *Service) seedDefaultPipelineTx(tx *gorm.DB, organizationID, userID uint64) error {
	pipeline := models.Pipeline{
		OrganizationID: organizationID,
		Name:           "Default Pipeline",
		IsDefault:      true,
		CreatedBy:      userID,
	}
	if err := tx.Create(&pipeline).Error; err != nil {
		return err
	}
	stages := []models.PipelineStage{
		{PipelineID: pipeline.ID, Name: "new", Position: 1},
		{PipelineID: pipeline.ID, Name: "qualified", Position: 2},
		{PipelineID: pipeline.ID, Name: "meeting_scheduled", Position: 3},
		{PipelineID: pipeline.ID, Name: "proposal_sent", Position: 4},
		{PipelineID: pipeline.ID, Name: "won", Position: 5, IsClosed: true},
		{PipelineID: pipeline.ID, Name: "lost", Position: 6, IsClosed: true},
	}
	return tx.Create(&stages).Error
}

func (s *Service) recordDealActivity(ctx context.Context, organizationID, dealID, userID uint64, activityType, referenceType, referenceID, summary string, metadata map[string]any) error {
	metadataJSON := ""
	if len(metadata) > 0 {
		raw, err := json.Marshal(metadata)
		if err != nil {
			return err
		}
		metadataJSON = string(raw)
	}
	return s.db.WithContext(ctx).Create(&models.DealActivity{
		OrganizationID: organizationID,
		DealID:         dealID,
		Type:           activityType,
		ReferenceType:  referenceType,
		ReferenceID:    referenceID,
		Summary:        summary,
		MetadataJSON:   metadataJSON,
		CreatedBy:      userID,
	}).Error
}

func (s *Service) ensureRoomParticipantJoined(ctx context.Context, organizationID, userID, roomID uint64) error {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
		return err
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&models.CallRoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Updates(map[string]any{
			"joined_at":  &now,
			"left_at":    nil,
			"updated_at": now,
		}).Error; err != nil {
		return err
	}
	if room.Status == models.RoomStatusScheduled {
		return s.db.WithContext(ctx).Model(&room).
			Updates(map[string]any{
				"status":     models.RoomStatusActive,
				"started_at": &now,
				"updated_at": now,
			}).Error
	}
	return nil
}

func uniqueSlug(name string, userID uint64) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, " ", "-")
	normalized = strings.ReplaceAll(normalized, "_", "-")
	if normalized == "" {
		normalized = "workspace"
	}
	return fmt.Sprintf("%s-%d", normalized, userID)
}

func uniqueUint64s(items []uint64) []uint64 {
	seen := make(map[uint64]struct{}, len(items))
	result := make([]uint64, 0, len(items))
	for _, item := range items {
		if item == 0 {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func mapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func findRoomMember(items []RoomMemberSummary, userID uint64) *RoomMemberSummary {
	for index := range items {
		if items[index].UserID == userID {
			return &items[index]
		}
	}
	return nil
}

func isValidOrgRole(role string) bool {
	switch role {
	case models.OrganizationRoleOwner, models.OrganizationRoleAdmin, models.OrganizationRoleMember:
		return true
	default:
		return false
	}
}

func isValidRecordingMode(mode string) bool {
	switch mode {
	case models.RecordingModeOff, models.RecordingModeAdminOptIn, models.RecordingModeForcedForTeamMeetings:
		return true
	default:
		return false
	}
}

func isValidConversationType(kind string) bool {
	switch kind {
	case models.ConversationTypeDirect, models.ConversationTypeChannel, models.ConversationTypeMeeting:
		return true
	default:
		return false
	}
}

func normalizeConversationStatus(status string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "", models.ConversationStatusOpen:
		return models.ConversationStatusOpen, nil
	case models.ConversationStatusPending:
		return models.ConversationStatusPending, nil
	case models.ConversationStatusResolved:
		return models.ConversationStatusResolved, nil
	default:
		return "", errors.New("invalid conversation status")
	}
}

func normalizeConversationPriority(priority string) (string, error) {
	switch strings.TrimSpace(strings.ToLower(priority)) {
	case "", models.ConversationPriorityNormal:
		return models.ConversationPriorityNormal, nil
	case models.ConversationPriorityLow:
		return models.ConversationPriorityLow, nil
	case models.ConversationPriorityHigh:
		return models.ConversationPriorityHigh, nil
	case models.ConversationPriorityUrgent:
		return models.ConversationPriorityUrgent, nil
	default:
		return "", errors.New("invalid conversation priority")
	}
}

func isValidMessageType(kind string) bool {
	switch kind {
	case models.MessageTypeText, models.MessageTypeSystem, models.MessageTypeCallEvent:
		return true
	default:
		return false
	}
}

func truncate(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func buildCallEventBody(eventType, fromDisplayName, toDisplayName string) string {
	actor := strings.TrimSpace(fromDisplayName)
	if actor == "" {
		actor = "A participant"
	}
	peer := strings.TrimSpace(toDisplayName)
	if peer == "" {
		peer = "the other participant"
	}
	switch eventType {
	case "call.accepted":
		return fmt.Sprintf("%s started a call with %s.", actor, peer)
	case "call.rejected":
		return fmt.Sprintf("%s rejected the call with %s.", actor, peer)
	case "call.ended":
		return fmt.Sprintf("%s ended the call with %s.", actor, peer)
	default:
		return fmt.Sprintf("%s call event with %s.", actor, peer)
	}
}
