package collaboration

import (
	"context"
	"encoding/json"
	"strings"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// collectSummaryUserIDs is a pure helper that, given a batch of conversations,
// returns the conversation ids whose peer user must be looked up (direct
// conversations without a title) and the deduplicated set of assignee user ids
// to fetch. It performs no I/O so it can be unit tested in isolation.
func collectSummaryUserIDs(convs []models.Conversation, userID uint64) (directConvIDs, assigneeIDs []uint64) {
	seen := make(map[uint64]struct{})
	addAssignee := func(id uint64) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		assigneeIDs = append(assigneeIDs, id)
	}
	for _, conv := range convs {
		if conv.Type == models.ConversationTypeDirect && strings.TrimSpace(conv.Title) == "" {
			directConvIDs = append(directConvIDs, conv.ID)
		}
		if conv.AssigneeUserID != nil {
			addAssignee(*conv.AssigneeUserID)
		}
	}
	return directConvIDs, assigneeIDs
}

// loadConversationSummaryUsers fetches every peer and assignee user needed to
// build summaries for the given conversations in a constant number of queries
// (one for peer membership, one for the users table) instead of one per
// conversation.
func (s *Service) loadConversationSummaryUsers(ctx context.Context, convs []models.Conversation, userID uint64) (map[uint64]models.User, map[uint64]uint64, error) {
	directConvIDs, userIDs := collectSummaryUserIDs(convs, userID)
	peerByConv := make(map[uint64]uint64)
	if len(directConvIDs) > 0 {
		var rows []struct {
			ConversationID uint64 `gorm:"column:conversation_id"`
			UserID         uint64 `gorm:"column:user_id"`
		}
		if err := s.db.WithContext(ctx).
			Table("conversation_members").
			Select("conversation_id, user_id").
			Where("conversation_id IN ? AND user_id <> ?", directConvIDs, userID).
			Find(&rows).Error; err != nil {
			return nil, nil, err
		}
		seen := make(map[uint64]struct{})
		for _, r := range rows {
			if _, ok := peerByConv[r.ConversationID]; !ok {
				peerByConv[r.ConversationID] = r.UserID
			}
			if _, ok := seen[r.UserID]; ok {
				continue
			}
			seen[r.UserID] = struct{}{}
			userIDs = append(userIDs, r.UserID)
		}
	}
	if len(userIDs) == 0 {
		return map[uint64]models.User{}, peerByConv, nil
	}
	var users []models.User
	if err := s.db.WithContext(ctx).Select("id, email, display_name").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, nil, err
	}
	userCache := make(map[uint64]models.User, len(users))
	for _, u := range users {
		userCache[u.ID] = u
	}
	return userCache, peerByConv, nil
}

// buildConversationSummary constructs a conversation summary, querying peer
// and assignee users on demand (one DB round-trip each). Prefer
// buildConversationSummaryBatch when building summaries for many conversations
// at once to avoid the N+1 query pattern.
func (s *Service) buildConversationSummary(ctx context.Context, conv models.Conversation, userID uint64) (ConversationSummary, error) {
	return s.buildConversationSummaryWithUsers(ctx, conv, userID, nil, nil)
}

// buildConversationSummaryWithUsers is like buildConversationSummary but uses a
// preloaded user cache (and per-conversation peer mapping) so the per-peer and
// per-assignee queries are served from memory instead of the database. The
// cache is optional: when a user/peer is missing it falls back to a direct
// query, which keeps single-conversation callers correct.
func (s *Service) buildConversationSummaryWithUsers(ctx context.Context, conv models.Conversation, userID uint64, users map[uint64]models.User, peerByConv map[uint64]uint64) (ConversationSummary, error) {
	if strings.TrimSpace(conv.Status) == "" {
		conv.Status = models.ConversationStatusOpen
	}
	if strings.TrimSpace(conv.Priority) == "" {
		conv.Priority = models.ConversationPriorityNormal
	}
	item := ConversationSummary{Conversation: conv}
	if conv.Type == models.ConversationTypeDirect && strings.TrimSpace(conv.Title) == "" {
		var peer models.User
		if peerID, ok := peerByConv[conv.ID]; ok {
			if u, ok := users[peerID]; ok {
				peer = u
			}
		}
		if peer.ID == 0 {
			// Fallback to a direct query (single-conversation callers and any
			// cache miss). Errors are non-fatal: we simply skip the title.
			_ = s.db.WithContext(ctx).
				Table("conversation_members").
				Select("users.*").
				Joins("JOIN users ON users.id = conversation_members.user_id").
				Where("conversation_members.conversation_id = ? AND conversation_members.user_id <> ?", conv.ID, userID).
				Take(&peer).Error
		}
		if peer.ID != 0 {
			if strings.TrimSpace(peer.DisplayName) != "" {
				item.Title = peer.DisplayName
			} else {
				item.Title = peer.Email
			}
		}
	}
	if conv.AssigneeUserID != nil {
		var assignee models.User
		if u, ok := users[*conv.AssigneeUserID]; ok {
			assignee = u
		}
		if assignee.ID == 0 {
			_ = s.db.WithContext(ctx).Select("email, display_name").Where("id = ?", *conv.AssigneeUserID).Take(&assignee).Error
		}
		if assignee.ID != 0 {
			item.AssigneeEmail = assignee.Email
			item.AssigneeDisplayName = assignee.DisplayName
		}
	}
	var last models.Message
	if err := s.db.WithContext(ctx).Where("conversation_id = ?", conv.ID).Order("created_at DESC").Take(&last).Error; err == nil {
		// 会话列表的「最后一条消息」预览同样来自加密列，必须解密后再截断。
		// The conversation list preview also reads the encrypted column.
		s.decryptMessageInPlace(&last)
		item.LastMessagePreview = truncate(last.Body, 120)
		item.LastMessageType = last.Type
	}
	var unread int64
	var member models.ConversationMember
	if err := s.db.WithContext(ctx).Where("conversation_id = ? AND user_id = ?", conv.ID, userID).Take(&member).Error; err == nil {
		query := s.db.WithContext(ctx).Model(&models.Message{}).Where("conversation_id = ? AND sender_id <> ?", conv.ID, userID)
		if member.LastReadAt != nil {
			query = query.Where("created_at > ?", *member.LastReadAt)
		}
		if err := query.Count(&unread).Error; err != nil {
			s.logger.Warn().Err(err).Uint64("conversation_id", conv.ID).Uint64("user_id", userID).Msg("failed to count unread messages")
		}
		item.UnreadCount = unread
	}

	var activeRoom models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ? AND status IN ?", conv.OrganizationID, conv.ID, []string{models.RoomStatusScheduled, models.RoomStatusActive}).
		Order("updated_at DESC").
		Take(&activeRoom).Error; err == nil {
		item.ActiveRoomID = &activeRoom.ID
	}
	var recording models.RecordingSession
	if err := s.db.WithContext(ctx).
		Table("recording_sessions").
		Select("recording_sessions.*").
		Joins("JOIN call_rooms ON call_rooms.id = recording_sessions.room_id").
		Where("call_rooms.organization_id = ? AND call_rooms.conversation_id = ?", conv.OrganizationID, conv.ID).
		Order("recording_sessions.id DESC").
		Take(&recording).Error; err == nil {
		item.LatestRecordingID = &recording.ID
	}
	var latestRoom models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", conv.OrganizationID, conv.ID).
		Order("updated_at DESC").
		Take(&latestRoom).Error; err == nil {
		item.LatestRoomID = &latestRoom.ID
		item.LatestRoomTitle = latestRoom.Title
	}
	if item.ActiveRoomID != nil {
		var activeRoom models.CallRoom
		if err := s.db.WithContext(ctx).Select("title").Where("id = ?", *item.ActiveRoomID).Take(&activeRoom).Error; err == nil {
			item.ActiveRoomTitle = activeRoom.Title
		}
	}
	return item, nil
}
func (s *Service) latestConversationRoom(ctx context.Context, organizationID, conversationID uint64) (*RoomListItem, error) {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Take(&room).Error; err != nil {
		return nil, err
	}
	return s.latestRoomByID(ctx, organizationID, room.ID)
}
func (s *Service) latestRoomByID(ctx context.Context, organizationID, roomID uint64) (*RoomListItem, error) {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND id = ?", organizationID, roomID).
		Take(&room).Error; err != nil {
		return nil, err
	}
	item := &RoomListItem{
		ID:             room.ID,
		OrganizationID: room.OrganizationID,
		TeamID:         room.TeamID,
		ConversationID: room.ConversationID,
		Title:          room.Title,
		Status:         room.Status,
		CreatedBy:      room.CreatedBy,
		StartedAt:      room.StartedAt,
		EndedAt:        room.EndedAt,
		CreatedAt:      room.CreatedAt,
		UpdatedAt:      room.UpdatedAt,
	}
	if room.ConversationID != nil {
		var conv models.Conversation
		if err := s.db.WithContext(ctx).Select("title").Where("id = ?", *room.ConversationID).Take(&conv).Error; err == nil {
			item.ConversationTitle = conv.Title
		}
	}
	return item, nil
}
func (s *Service) latestConversationFollowup(ctx context.Context, conversationID uint64) (*ConversationFollowupSummary, error) {
	var message models.Message
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND type = ?", conversationID, models.MessageTypeCallEvent).
		Order("created_at DESC").
		Take(&message).Error; err != nil {
		return nil, err
	}
	callID := ""
	if strings.TrimSpace(message.MetadataJSON) != "" {
		var metadata map[string]any
		if err := json.Unmarshal([]byte(message.MetadataJSON), &metadata); err == nil {
			callID, _ = metadata["call_id"].(string)
		}
	}
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var followup struct {
		SummaryCN       string
		SummaryEN       string
		ActionItemsJSON string
		NextStep        string
	}
	if err := s.db.WithContext(ctx).
		Table("call_followups").
		Select("summary_cn, summary_en, action_items_json, next_step").
		Where("call_id = ?", callID).
		Order("generated_at DESC").
		Take(&followup).Error; err != nil {
		return nil, err
	}
	var actionItems []string
	if strings.TrimSpace(followup.ActionItemsJSON) != "" {
		_ = json.Unmarshal([]byte(followup.ActionItemsJSON), &actionItems)
	}
	return &ConversationFollowupSummary{
		CallID:      callID,
		SummaryCN:   followup.SummaryCN,
		SummaryEN:   followup.SummaryEN,
		ActionItems: actionItems,
		NextStep:    followup.NextStep,
	}, nil
}
