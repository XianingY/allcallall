package collaboration

import (
	"context"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) countRoomParticipants(ctx context.Context, roomID uint64) int64 {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.CallRoomMember{}).Where("room_id = ?", roomID).Count(&count).Error; err != nil {
		s.logger.Warn().Err(err).Uint64("room_id", roomID).Msg("failed to count room participants")
	}
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
