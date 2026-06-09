package collaboration

import (
	"context"
	"errors"
	"fmt"
)

func (s *Service) publishRoomEvent(ctx context.Context, organizationID, roomID uint64, event string, payload any) {
	memberIDs, err := s.listRoomMemberIDs(ctx, roomID)
	if err != nil || len(memberIDs) == 0 {
		s.metrics.Inc("room_event_broadcast_fail_total")
		return
	}
	s.publishRealtimeEvent(ctx, organizationID, memberIDs, event, payload)
}

func (s *Service) publishConversationEvent(ctx context.Context, organizationID, conversationID uint64, event string, payload any) {
	memberIDs, err := s.listConversationMemberIDs(ctx, conversationID)
	if err != nil || len(memberIDs) == 0 {
		return
	}
	s.publishRealtimeEvent(ctx, organizationID, memberIDs, event, payload)
}

func (s *Service) publishMessageCreatedRealtime(ctx context.Context, record *MessageRecord, memberIDs []uint64) error {
	if record == nil {
		return errors.New("message record is required")
	}
	return s.publishRealtimeEventWithDedup(ctx, record.OrganizationID, memberIDs, "message.created", record, func(userID uint64) string {
		return fmt.Sprintf("message.created:%d:%d", record.ID, userID)
	})
}

func (s *Service) publishRealtimeEvent(ctx context.Context, organizationID uint64, userIDs []uint64, event string, payload any) {
	userIDs = uniqueUint64s(userIDs)
	for _, userID := range userIDs {
		record, err := s.createRealtimeEvent(ctx, organizationID, userID, event, payload)
		if err != nil {
			s.metrics.Inc("chat_realtime_delivery_fail_total")
			continue
		}
		if s.publisher == nil {
			continue
		}
		if err := s.publisher.PublishToUser(ctx, *record); err != nil {
			s.metrics.Inc("chat_realtime_delivery_fail_total")
		}
	}
}

func (s *Service) publishRealtimeEventWithDedup(ctx context.Context, organizationID uint64, userIDs []uint64, event string, payload any, dedupKeyForUser func(uint64) string) error {
	userIDs = uniqueUint64s(userIDs)
	for _, userID := range userIDs {
		dedupKey := ""
		if dedupKeyForUser != nil {
			dedupKey = dedupKeyForUser(userID)
		}
		record, err := s.createRealtimeEventWithDedup(ctx, organizationID, userID, event, payload, dedupKey)
		if err != nil {
			s.metrics.Inc("chat_realtime_delivery_fail_total")
			return err
		}
		if s.publisher == nil {
			continue
		}
		if err := s.publisher.PublishToUser(ctx, *record); err != nil {
			s.metrics.Inc("chat_realtime_delivery_fail_total")
		}
	}
	return nil
}

func (s *Service) createRealtimeEvent(ctx context.Context, organizationID, userID uint64, event string, payload any) (*RealtimeEventRecord, error) {
	return NewRealtimeEventStore(s.db).Create(ctx, organizationID, userID, event, payload)
}

func (s *Service) createRealtimeEventWithDedup(ctx context.Context, organizationID, userID uint64, event string, payload any, dedupKey string) (*RealtimeEventRecord, error) {
	return NewRealtimeEventStore(s.db).CreateWithDedup(ctx, organizationID, userID, event, payload, dedupKey)
}
