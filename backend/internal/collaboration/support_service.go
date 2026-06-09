package collaboration

import (
	"context"

	"github.com/allcallall/backend/internal/models"
)

func (s *Service) GetSupportRoom(ctx context.Context, roomID uint64) (*SupportRoomView, error) {
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ?", roomID).Take(&room).Error; err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, room.OrganizationID, room.CreatedBy, roomID)
	if err != nil {
		return nil, err
	}
	var events []models.CallRoomEvent
	if err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("created_at DESC").
		Limit(100).
		Find(&events).Error; err != nil {
		return nil, err
	}
	var latestSession models.RecordingSession
	var recording *RecordingView
	if err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("id DESC").
		Take(&latestSession).Error; err == nil {
		if view, err := s.GetRecording(ctx, room.OrganizationID, room.CreatedBy, latestSession.ID); err == nil {
			recording = view
		}
	}
	return &SupportRoomView{
		State:        state,
		RecentEvents: events,
		Recording:    recording,
	}, nil
}

func (s *Service) GetSupportRecording(ctx context.Context, recordingID uint64) (*SupportRecordingView, error) {
	var session models.RecordingSession
	if err := s.db.WithContext(ctx).Where("id = ?", recordingID).Take(&session).Error; err != nil {
		return nil, err
	}
	view, err := s.GetRecording(ctx, session.OrganizationID, session.StartedBy, recordingID)
	if err != nil {
		return nil, err
	}
	var roomItem *RoomListItem
	if room, err := s.latestRoomByID(ctx, session.OrganizationID, session.RoomID); err == nil {
		roomItem = room
	}
	var policy models.OrganizationPolicy
	var policyPtr *models.OrganizationPolicy
	if err := s.db.WithContext(ctx).Where("organization_id = ?", session.OrganizationID).Take(&policy).Error; err == nil {
		policyPtr = &policy
	}
	var exports []models.RecordingExport
	if err := s.db.WithContext(ctx).
		Where("recording_session_id = ?", session.ID).
		Order("created_at DESC").
		Limit(50).
		Find(&exports).Error; err != nil {
		return nil, err
	}
	return &SupportRecordingView{
		Recording: *view,
		Room:      roomItem,
		Policy:    policyPtr,
		Exports:   exports,
	}, nil
}
