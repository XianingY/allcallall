package collaboration

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/pagination"
)

func (s *Service) ListConversations(ctx context.Context, organizationID, userID uint64, filter string, contactID *uint64, page pagination.Page) (pagination.Result[ConversationSummary], error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return pagination.Result[ConversationSummary]{}, err
	}
	np := page.Normalize()
	// buildQuery returns a fresh, filter-applied query so the same predicate
	// set drives both the COUNT and the paged FIND without duplication.
	buildQuery := func() *gorm.DB {
		q := s.db.WithContext(ctx).
			Table("conversations").
			Joins("JOIN conversation_members ON conversation_members.conversation_id = conversations.id").
			Where("conversations.organization_id = ? AND conversation_members.user_id = ?", organizationID, userID)
		switch strings.TrimSpace(strings.ToLower(filter)) {
		case "", "all":
		case "my":
			q = q.Where("conversations.assignee_user_id = ?", userID)
		case models.ConversationStatusOpen, models.ConversationStatusPending, models.ConversationStatusResolved:
			q = q.Where("conversations.status = ?", filter)
		case "channels":
			q = q.Where("conversations.type = ?", models.ConversationTypeChannel)
		}
		if contactID != nil && *contactID != 0 {
			q = q.Where("conversations.contact_id = ?", *contactID)
		}
		return q
	}

	var total int64
	if err := buildQuery().Count(&total).Error; err != nil {
		return pagination.Result[ConversationSummary]{}, err
	}

	var convs []models.Conversation
	if err := buildQuery().
		Order("conversations.last_message_at DESC, conversations.updated_at DESC").
		Scopes(np.Scope).
		Find(&convs).Error; err != nil {
		return pagination.Result[ConversationSummary]{}, err
	}
	result := make([]ConversationSummary, 0, len(convs))
	if len(convs) == 0 {
		return pagination.NewResult(result, total, np), nil
	}
	// Batch-load every peer/assignee user up front so the per-conversation
	// summary loop performs no per-row JOIN to the users table (N+1 fix).
	users, peerByConv, err := s.loadConversationSummaryUsers(ctx, convs, userID)
	if err != nil {
		return pagination.Result[ConversationSummary]{}, err
	}
	for _, conv := range convs {
		item, err := s.buildConversationSummaryWithUsers(ctx, conv, userID, users, peerByConv)
		if err != nil {
			return pagination.Result[ConversationSummary]{}, err
		}
		result = append(result, item)
	}
	return pagination.NewResult(result, total, np), nil
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
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
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
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return &summary, nil
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
		if err := s.db.WithContext(ctx).Where("message_id = ? AND user_id = ?", last.ID, userID).FirstOrCreate(&read).Error; err != nil {
			s.logger.Warn().Err(err).Uint64("conversation_id", conversationID).Uint64("user_id", userID).Msg("failed to record message read receipt")
		}
	}
	return nil
}
