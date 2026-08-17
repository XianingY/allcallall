package collaboration

import (
	"context"
	"errors"
	"time"

	"github.com/allcallall/backend/internal/models"
)

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
