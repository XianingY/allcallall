package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

func (s *Service) ListConversations(ctx context.Context, organizationID, userID uint64, filter string, contactID *uint64) ([]ConversationSummary, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	query := s.db.WithContext(ctx).
		Table("conversations").
		Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
		Where("conversations.organization_id = ? AND conversation_members.user_id = ?", organizationID, userID)

	switch strings.TrimSpace(strings.ToLower(filter)) {
	case "", "all":
	case "my":
		query = query.Where("conversations.assignee_user_id = ?", userID)
	case models.ConversationStatusOpen, models.ConversationStatusPending, models.ConversationStatusResolved:
		query = query.Where("conversations.status = ?", filter)
	case "channels":
		query = query.Where("conversations.type = ?", models.ConversationTypeChannel)
	}
	if contactID != nil && *contactID != 0 {
		query = query.Where("conversations.contact_id = ?", *contactID)
	}

	var convs []models.Conversation
	if err := query.
		Order("conversations.last_message_at DESC, conversations.updated_at DESC").
		Find(&convs).Error; err != nil {
		return nil, err
	}
	result := make([]ConversationSummary, 0, len(convs))
	for _, conv := range convs {
		item, err := s.buildConversationSummary(ctx, conv, userID)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) GetConversation(ctx context.Context, organizationID, userID, conversationID uint64) (*ConversationDetail, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var conv models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&conv).Error; err != nil {
		return nil, err
	}
	summary, err := s.buildConversationSummary(ctx, conv, userID)
	if err != nil {
		return nil, err
	}
	detail := &ConversationDetail{Conversation: summary}

	if note, err := s.latestConversationNote(ctx, organizationID, conversationID); err == nil {
		detail.LatestNote = note
	}
	if room, err := s.latestConversationRoom(ctx, organizationID, conversationID); err == nil {
		detail.LatestRoom = room
	}
	if followup, err := s.latestConversationFollowup(ctx, conversationID); err == nil {
		detail.LatestFollowup = followup
	}
	detail.Workspace = ConversationWorkspace{
		LatestMeeting:  detail.LatestRoom,
		LatestNote:     detail.LatestNote,
		AgentContext:   s.buildConversationAgentContext(ctx, organizationID, conversationID, detail.LatestFollowup),
		AssigneeUserID: summary.AssigneeUserID,
		AssigneeLabel:  firstNonEmpty(summary.AssigneeDisplayName, summary.AssigneeEmail),
		Status:         summary.Status,
		Priority:       summary.Priority,
	}
	if detail.LatestFollowup != nil {
		detail.Workspace.MeetingSummary = &MeetingSummaryCard{
			Summary:     firstNonEmpty(detail.LatestFollowup.SummaryCN, detail.LatestFollowup.SummaryEN),
			ActionItems: append([]string{}, detail.LatestFollowup.ActionItems...),
			NextStep:    detail.LatestFollowup.NextStep,
			Assignee:    detail.Workspace.AssigneeLabel,
		}
	}
	if summary.LatestRecordingID != nil {
		if recording, err := s.GetRecording(ctx, organizationID, userID, *summary.LatestRecordingID); err == nil {
			detail.Workspace.LatestRecording = recording
		}
	}
	return detail, nil
}

func (s *Service) buildConversationAgentContext(ctx context.Context, organizationID, conversationID uint64, latestFollowup *ConversationFollowupSummary) ConversationAgentContext {
	result := ConversationAgentContext{}
	if latestFollowup != nil {
		result.LatestCallID = latestFollowup.CallID
		if result.LatestCallID != "" {
			var transcriptCount int64
			if err := s.db.WithContext(ctx).
				Model(&models.CallTranscriptSegment{}).
				Where("call_id = ?", result.LatestCallID).
				Count(&transcriptCount).Error; err == nil {
				result.TranscriptSegmentCount = int(transcriptCount)
			}
			var latestSegment models.CallTranscriptSegment
			if err := s.db.WithContext(ctx).
				Select("created_at").
				Where("call_id = ?", result.LatestCallID).
				Order("created_at DESC").
				Take(&latestSegment).Error; err == nil {
				result.LatestTranscriptAt = &latestSegment.CreatedAt
			}
		}
	}
	var memories []models.AgentMemory
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("updated_at DESC").
		Limit(10).
		Find(&memories).Error; err == nil {
		keys := make([]string, 0, len(memories))
		for _, memory := range memories {
			keys = append(keys, memory.Key)
		}
		result.LatestMemoryKeys = uniqueStrings(keys)
	}
	var workflow models.WorkflowRun
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
		Order("COALESCE(completed_at, started_at, updated_at) DESC").
		Take(&workflow).Error; err == nil {
		result.LastWorkflowID = &workflow.ID
		result.LastWorkflowPreset = workflow.Preset
		result.LastAgentStatus = workflow.Status
		switch {
		case workflow.CompletedAt != nil:
			result.LastAgentRunAt = workflow.CompletedAt
		case workflow.StartedAt != nil:
			result.LastAgentRunAt = workflow.StartedAt
		default:
			at := workflow.UpdatedAt
			result.LastAgentRunAt = &at
		}
	} else {
		var run models.AgentRun
		if err := s.db.WithContext(ctx).
			Where("organization_id = ? AND conversation_id = ?", organizationID, conversationID).
			Order("COALESCE(completed_at, started_at, updated_at) DESC").
			Take(&run).Error; err == nil {
			result.LastAgentStatus = run.Status
			switch {
			case run.CompletedAt != nil:
				result.LastAgentRunAt = run.CompletedAt
			case run.StartedAt != nil:
				result.LastAgentRunAt = run.StartedAt
			default:
				at := run.UpdatedAt
				result.LastAgentRunAt = &at
			}
		}
	}
	_ = s.db.WithContext(ctx).
		Model(&models.ToolApproval{}).
		Joins("JOIN workflow_runs ON workflow_runs.id = tool_approvals.workflow_run_id").
		Where("tool_approvals.organization_id = ? AND workflow_runs.conversation_id = ? AND tool_approvals.status = ?", organizationID, conversationID, models.ToolApprovalStatusPending).
		Count(&result.PendingApprovalCount).Error
	_ = s.db.WithContext(ctx).
		Model(&models.RAGSource{}).
		Where("organization_id = ? AND (conversation_id IS NULL OR conversation_id = ?)", organizationID, conversationID).
		Where("status = ?", models.RAGSourceStatusReady).
		Where("(dedupe_status IS NULL OR dedupe_status <> ?)", models.RAGSourceDedupeStatusConfirmedDuplicate).
		Count(&result.KnowledgeSourceCount).Error
	result.applyMeetingTranscriptionContext(s.loadLatestConversationTranscriptionContext(ctx, organizationID, conversationID))
	return result
}

func (s *Service) CreateConversation(ctx context.Context, organizationID, userID uint64, input CreateConversationInput) (*models.Conversation, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	if input.Type == "" {
		input.Type = models.ConversationTypeDirect
	}
	if !isValidConversationType(input.Type) {
		return nil, errors.New("invalid conversation type")
	}
	memberIDs := append([]uint64{}, input.MemberIDs...)
	memberIDs = append(memberIDs, userID)
	memberIDs = uniqueUint64s(memberIDs)
	if len(memberIDs) == 0 {
		return nil, errors.New("conversation members required")
	}
	conv := &models.Conversation{
		OrganizationID: organizationID,
		TeamID:         input.TeamID,
		RoomID:         input.RoomID,
		Type:           input.Type,
		Title:          strings.TrimSpace(input.Title),
		Topic:          strings.TrimSpace(input.Topic),
		Status:         models.ConversationStatusOpen,
		Priority:       models.ConversationPriorityNormal,
		CreatedBy:      userID,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.Type == models.ConversationTypeDirect {
			if len(memberIDs) != 2 {
				return errors.New("direct conversations require exactly two members")
			}
			if existing := s.findDirectConversationTx(ctx, tx, organizationID, memberIDs); existing != nil {
				*conv = *existing
				return nil
			}
		}
		if err := tx.Create(conv).Error; err != nil {
			return err
		}
		for _, memberID := range memberIDs {
			member := models.ConversationMember{
				ConversationID: conv.ID,
				UserID:         memberID,
				Role:           models.OrganizationRoleMember,
			}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *Service) UpdateConversation(ctx context.Context, organizationID, userID, conversationID uint64, input UpdateConversationInput) (*ConversationSummary, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	var conv models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&conv).Error; err != nil {
		return nil, err
	}

	plan, err := buildConversationUpdatePlan(conv, input)
	if err != nil {
		return nil, err
	}
	if plan.AssigneeUserIDToValidate != nil {
		var count int64
		if err := s.db.WithContext(ctx).Model(&models.ConversationMember{}).
			Where("conversation_id = ? AND user_id = ?", conversationID, *plan.AssigneeUserIDToValidate).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, errors.New("assignee must be a conversation member")
		}
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if len(plan.Updates) > 0 {
			plan.Updates["updated_at"] = time.Now()
			if err := tx.Model(&models.Conversation{}).
				Where("id = ? AND organization_id = ?", conversationID, organizationID).
				Updates(plan.Updates).Error; err != nil {
				return err
			}
		}
		for _, event := range plan.SystemEvents {
			if _, err := s.createMessageTx(ctx, tx, organizationID, userID, conversationID, event, false); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var updated models.Conversation
	if err := s.db.WithContext(ctx).Where("organization_id = ? AND id = ?", organizationID, conversationID).Take(&updated).Error; err != nil {
		return nil, err
	}
	summary, err := s.buildConversationSummary(ctx, updated, userID)
	if err != nil {
		return nil, err
	}
	changes := buildConversationPatchChanges(summary, plan.ChangedFields)
	s.publishConversationPatchUpdate(ctx, organizationID, conversationID, changes)
	return &summary, nil
}

func (s *Service) ListConversationNotes(ctx context.Context, organizationID, userID, conversationID uint64, limit int) ([]ConversationNoteRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var notes []ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.organization_id = ? AND conversation_notes.conversation_id = ?", organizationID, conversationID).
		Order("conversation_notes.created_at DESC").
		Limit(limit).
		Find(&notes).Error
	return notes, err
}

func (s *Service) CreateConversationNote(ctx context.Context, organizationID, userID, conversationID uint64, body string) (*ConversationNoteRecord, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("note body required")
	}
	note := &models.ConversationNote{
		OrganizationID: organizationID,
		ConversationID: conversationID,
		AuthorID:       userID,
		Body:           body,
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(note).Error; err != nil {
			return err
		}
		return tx.Model(&models.Conversation{}).
			Where("id = ? AND organization_id = ?", conversationID, organizationID).
			Updates(map[string]any{
				"last_internal_note_at": now,
				"updated_at":            now,
			}).Error
	}); err != nil {
		return nil, err
	}
	record, err := s.loadConversationNote(ctx, note.ID)
	if err != nil {
		return nil, err
	}
	s.publishConversationPatchUpdate(ctx, organizationID, conversationID, map[string]any{
		"last_internal_note_at": record.CreatedAt,
	})
	s.publishConversationEvent(ctx, organizationID, conversationID, "conversation.note.created", record)
	return record, nil
}

func (s *Service) AppendDirectCallEventByEmail(ctx context.Context, fromEmail, toEmail, callID, eventType string, metadata map[string]any) error {
	if s.users == nil {
		return nil
	}
	fromUser, err := s.users.GetByEmail(ctx, fromEmail)
	if err != nil {
		return err
	}
	toUser, err := s.users.GetByEmail(ctx, toEmail)
	if err != nil {
		return err
	}
	organizationID, err := s.findSharedOrganizationID(ctx, fromUser.ID, toUser.ID)
	if err != nil {
		if errors.Is(err, ErrOrganizationAccessDenied) {
			return nil
		}
		return err
	}
	conversation, err := s.CreateConversation(ctx, organizationID, fromUser.ID, CreateConversationInput{
		Type:      models.ConversationTypeDirect,
		MemberIDs: []uint64{toUser.ID},
	})
	if err != nil {
		return err
	}
	body := buildCallEventBody(eventType, fromUser.DisplayName, toUser.DisplayName)
	input := MessageInput{
		Type: models.MessageTypeCallEvent,
		Body: body,
		Metadata: map[string]any{
			"call_id":    callID,
			"event_type": eventType,
			"from_email": fromEmail,
			"to_email":   toEmail,
			"emitted_at": time.Now().Format(time.RFC3339),
		},
	}
	for key, value := range metadata {
		input.Metadata[key] = value
	}
	_, err = s.CreateMessage(ctx, organizationID, fromUser.ID, conversation.ID, input)
	return err
}

func (s *Service) MarkConversationRead(ctx context.Context, organizationID, userID, conversationID uint64) error {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return err
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Model(&models.ConversationMember{}).
		Where("conversation_id = ? AND user_id = ?", conversationID, userID).
		Updates(map[string]any{
			"last_read_at": now,
			"updated_at":   now,
		}).Error; err != nil {
		return err
	}
	var last models.Message
	if err := s.db.WithContext(ctx).Where("conversation_id = ?", conversationID).Order("id DESC").Take(&last).Error; err == nil {
		read := models.MessageRead{
			MessageID: last.ID,
			UserID:    userID,
			ReadAt:    now,
		}
		_ = s.db.WithContext(ctx).Where("message_id = ? AND user_id = ?", last.ID, userID).FirstOrCreate(&read).Error
	}
	return nil
}

func (s *Service) buildConversationSummary(ctx context.Context, conv models.Conversation, userID uint64) (ConversationSummary, error) {
	if strings.TrimSpace(conv.Status) == "" {
		conv.Status = models.ConversationStatusOpen
	}
	if strings.TrimSpace(conv.Priority) == "" {
		conv.Priority = models.ConversationPriorityNormal
	}
	item := ConversationSummary{Conversation: conv}
	if conv.Type == models.ConversationTypeDirect && strings.TrimSpace(conv.Title) == "" {
		var peer models.User
		err := s.db.WithContext(ctx).
			Table("conversation_members").
			Select("users.*").
			Joins("JOIN users ON users.id = conversation_members.user_id").
			Where("conversation_members.conversation_id = ? AND conversation_members.user_id <> ?", conv.ID, userID).
			Take(&peer).Error
		if err == nil {
			if strings.TrimSpace(peer.DisplayName) != "" {
				item.Title = peer.DisplayName
			} else {
				item.Title = peer.Email
			}
		}
	}
	if conv.AssigneeUserID != nil {
		var assignee models.User
		if err := s.db.WithContext(ctx).Select("email, display_name").Where("id = ?", *conv.AssigneeUserID).Take(&assignee).Error; err == nil {
			item.AssigneeEmail = assignee.Email
			item.AssigneeDisplayName = assignee.DisplayName
		}
	}
	var last models.Message
	if err := s.db.WithContext(ctx).Where("conversation_id = ?", conv.ID).Order("created_at DESC").Take(&last).Error; err == nil {
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
		_ = query.Count(&unread).Error
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

func (s *Service) latestConversationNote(ctx context.Context, organizationID, conversationID uint64) (*ConversationNoteRecord, error) {
	var note ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.organization_id = ? AND conversation_notes.conversation_id = ?", organizationID, conversationID).
		Order("conversation_notes.created_at DESC").
		Take(&note).Error
	if err != nil {
		return nil, err
	}
	return &note, nil
}

func (s *Service) loadConversationNote(ctx context.Context, noteID uint64) (*ConversationNoteRecord, error) {
	var note ConversationNoteRecord
	err := s.db.WithContext(ctx).
		Table("conversation_notes").
		Select("conversation_notes.*, users.email AS author_email, users.display_name AS author_display_name").
		Joins("JOIN users ON users.id = conversation_notes.author_id").
		Where("conversation_notes.id = ?", noteID).
		Take(&note).Error
	if err != nil {
		return nil, err
	}
	return &note, nil
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

func (s *Service) createMessageTx(ctx context.Context, tx *gorm.DB, organizationID, userID, conversationID uint64, input MessageInput, publish bool) (*models.Message, error) {
	if input.Type == "" {
		input.Type = models.MessageTypeText
	}
	if !isValidMessageType(input.Type) {
		return nil, errors.New("invalid message type")
	}
	body := strings.TrimSpace(input.Body)
	if input.Type == models.MessageTypeText && body == "" {
		return nil, errors.New("message body required")
	}
	metadataJSON := ""
	if len(input.Metadata) > 0 {
		raw, err := json.Marshal(input.Metadata)
		if err != nil {
			return nil, err
		}
		metadataJSON = string(raw)
	}
	message := &models.Message{
		OrganizationID:   organizationID,
		ConversationID:   conversationID,
		SenderID:         userID,
		ReplyToMessageID: input.ReplyToMessageID,
		Type:             input.Type,
		Body:             body,
		MetadataJSON:     metadataJSON,
	}
	if message.ReplyToMessageID != nil {
		var count int64
		if err := tx.WithContext(ctx).Model(&models.Message{}).
			Where("id = ? AND organization_id = ? AND conversation_id = ?", *message.ReplyToMessageID, organizationID, conversationID).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, errors.New("reply target not found")
		}
	}
	if err := tx.Create(message).Error; err != nil {
		return nil, err
	}
	if len(input.AttachmentIDs) > 0 {
		ids := uniqueUint64s(input.AttachmentIDs)
		result := tx.WithContext(ctx).Model(&models.Attachment{}).
			Where("organization_id = ? AND conversation_id = ? AND uploader_id = ? AND id IN ? AND message_id IS NULL", organizationID, conversationID, userID, ids).
			Update("message_id", message.ID)
		if result.Error != nil {
			return nil, result.Error
		}
		if result.RowsAffected != int64(len(ids)) {
			return nil, errors.New("one or more attachments are unavailable")
		}
	}
	now := time.Now()
	if err := tx.Model(&models.Conversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"last_message_at": now,
			"updated_at":      now,
		}).Error; err != nil {
		return nil, err
	}
	if s.outbox != nil {
		_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "message",
			AggregateID:    message.ID,
			Event:          "message.created",
			IdempotencyKey: fmt.Sprintf("message.created:%d", message.ID),
			Payload: map[string]any{
				"organization_id": organizationID,
				"conversation_id": conversationID,
				"message_id":      message.ID,
				"sender_id":       userID,
				"type":            message.Type,
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return nil, err
		}
		_, err = s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "message",
			AggregateID:    message.ID,
			Event:          "search.message.index_requested",
			IdempotencyKey: fmt.Sprintf("search.message.index_requested:%d", message.ID),
			Payload: map[string]any{
				"organization_id": organizationID,
				"conversation_id": conversationID,
				"message_id":      message.ID,
			},
		})
		if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
			return nil, err
		}
	}
	if publish {
		record, err := s.loadMessageRecord(ctx, message.ID)
		if err == nil {
			memberIDs, _ := s.listConversationMemberIDsTx(ctx, tx, conversationID)
			_ = s.publishMessageCreatedRealtime(ctx, record, memberIDs)
		}
	}
	return message, nil
}

func (s *Service) createConversationSystemMessage(ctx context.Context, organizationID, userID uint64, conversationID *uint64, eventType, body string, metadata map[string]any) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, conversationID, eventType, body, metadata)
	})
}

func (s *Service) createConversationSystemMessageTx(ctx context.Context, tx *gorm.DB, organizationID, userID uint64, conversationID *uint64, eventType, body string, metadata map[string]any) error {
	if conversationID == nil || *conversationID == 0 {
		return nil
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["event_type"] = eventType
	_, err := s.createMessageTx(ctx, tx, organizationID, userID, *conversationID, MessageInput{
		Type:     models.MessageTypeSystem,
		Body:     body,
		Metadata: metadata,
	}, false)
	return err
}

func (s *Service) ensureConversationMemberTx(ctx context.Context, tx *gorm.DB, organizationID, userID, conversationID uint64) error {
	var count int64
	err := tx.WithContext(ctx).
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

func (s *Service) countRoomParticipants(ctx context.Context, roomID uint64) int64 {
	var count int64
	_ = s.db.WithContext(ctx).Model(&models.CallRoomMember{}).Where("room_id = ?", roomID).Count(&count).Error
	return count
}

func (s *Service) listRoomMemberIDs(ctx context.Context, roomID uint64) ([]uint64, error) {
	var ids []uint64
	if err := s.db.WithContext(ctx).
		Model(&models.CallRoomMember{}).
		Where("room_id = ?", roomID).
		Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return uniqueUint64s(ids), nil
}
