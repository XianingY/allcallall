package collaboration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/models"
)

func (s *Service) CreateRoom(ctx context.Context, organizationID, userID uint64, input CreateRoomInput) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	room := &models.CallRoom{
		OrganizationID: organizationID,
		TeamID:         input.TeamID,
		ConversationID: input.ConversationID,
		Title:          strings.TrimSpace(input.Title),
		Status:         models.RoomStatusScheduled,
		CreatedBy:      userID,
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if input.ConversationID != nil {
			if err := s.ensureConversationMemberTx(ctx, tx, organizationID, userID, *input.ConversationID); err != nil {
				return err
			}
		}
		if err := tx.Create(room).Error; err != nil {
			return err
		}
		if room.ConversationID == nil {
			conv, err := s.createMeetingConversationTx(ctx, tx, organizationID, userID, room.Title, input.TeamID, room.ID, input.ParticipantIDs)
			if err != nil {
				return err
			}
			room.ConversationID = &conv.ID
			if err := tx.Save(room).Error; err != nil {
				return err
			}
		}
		memberIDs := uniqueUint64s(append(input.ParticipantIDs, userID))
		for _, memberID := range memberIDs {
			member := models.CallRoomMember{
				RoomID: room.ID,
				UserID: memberID,
				Role:   models.OrganizationRoleMember,
			}
			if memberID == userID {
				member.Role = models.OrganizationRoleOwner
				member.JoinedAt = &now
			}
			if err := tx.Create(&member).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&models.CallRoomEvent{
			RoomID:      room.ID,
			UserID:      userID,
			Type:        "room.created",
			PayloadJSON: `{"status":"scheduled"}`,
		}).Error; err != nil {
			return err
		}
		return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, room.ConversationID, "meeting.created", fmt.Sprintf("会议“%s”已创建。", room.Title), map[string]any{
			"room_id":    room.ID,
			"room_title": room.Title,
			"status":     room.Status,
		})
	})
	if err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, room.ID)
	if err != nil {
		return nil, err
	}
	if room.ConversationID != nil {
		s.publishConversationPatchUpdate(ctx, organizationID, *room.ConversationID, map[string]any{
			"active_room_id":    room.ID,
			"active_room_title": room.Title,
			"latest_room_id":    room.ID,
			"latest_room_title": room.Title,
		})
	}
	s.publishRoomStateUpdated(ctx, organizationID, state, "meeting.created")
	return state, nil
}

func (s *Service) CreateConversationRoom(ctx context.Context, organizationID, userID, conversationID uint64, title string) (*RoomState, error) {
	if err := s.ensureConversationMember(ctx, organizationID, userID, conversationID); err != nil {
		return nil, err
	}
	memberIDs, err := s.listConversationMemberIDs(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return s.CreateRoom(ctx, organizationID, userID, CreateRoomInput{
		Title:          defaultString(strings.TrimSpace(title), "Team Meeting"),
		ConversationID: &conversationID,
		ParticipantIDs: memberIDs,
	})
}

func (s *Service) ListRooms(ctx context.Context, organizationID, userID uint64) ([]RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var rooms []models.CallRoom
	if err := s.db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		Order("updated_at DESC").
		Limit(100).
		Find(&rooms).Error; err != nil {
		return nil, err
	}
	result := make([]RoomState, 0, len(rooms))
	for _, room := range rooms {
		state, err := s.GetRoomState(ctx, organizationID, userID, room.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, *state)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].IsActive != result[j].IsActive {
			return result[i].IsActive
		}
		return result[i].Room.UpdatedAt.After(result[j].Room.UpdatedAt)
	})
	return result, nil
}

func (s *Service) JoinRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	s.metrics.Inc("meeting_join_total")
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var room models.CallRoom
		if err := tx.Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
			return err
		}
		member := models.CallRoomMember{
			RoomID:   roomID,
			UserID:   userID,
			Role:     models.OrganizationRoleMember,
			JoinedAt: &now,
			LeftAt:   nil,
		}
		if err := tx.Where("room_id = ? AND user_id = ?", roomID, userID).Assign(member).FirstOrCreate(&member).Error; err != nil {
			return err
		}
		if room.Status == models.RoomStatusScheduled {
			room.Status = models.RoomStatusActive
			room.StartedAt = &now
			if err := tx.Save(&room).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(&models.CallRoomEvent{
			RoomID:      roomID,
			UserID:      userID,
			Type:        "room.join",
			PayloadJSON: fmt.Sprintf(`{"joined_at":"%s"}`, now.Format(time.RFC3339)),
		}).Error; err != nil {
			return err
		}
		return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, room.ConversationID, "meeting.joined", "有成员加入了会议。", map[string]any{
			"room_id":   roomID,
			"user_id":   userID,
			"joined_at": now.Format(time.RFC3339),
		})
	})
	if err != nil {
		s.metrics.Inc("meeting_join_fail_total")
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err != nil {
		s.metrics.Inc("meeting_join_fail_total")
		return nil, err
	}
	if member := findRoomMember(state.Members, userID); member != nil {
		s.publishRoomMemberUpdated(ctx, organizationID, roomID, *member)
	}
	s.publishRoomStateUpdated(ctx, organizationID, state, "meeting.joined")
	if state.ConversationID != nil {
		s.publishConversationPatchUpdate(ctx, organizationID, *state.ConversationID, map[string]any{
			"active_room_id":    state.Room.ID,
			"active_room_title": state.Room.Title,
			"latest_room_id":    state.Room.ID,
			"latest_room_title": state.Room.Title,
		})
	}
	return state, nil
}

func (s *Service) HandleRoomOffer(ctx context.Context, organizationID, userID, roomID uint64, sdp string) (*RoomOfferResult, error) {
	if strings.TrimSpace(sdp) == "" {
		return nil, errors.New("sdp is required")
	}
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	if err := s.ensureRoomParticipantJoined(ctx, organizationID, userID, roomID); err != nil {
		return nil, err
	}
	if s.media == nil {
		return nil, errors.New("media engine not attached")
	}

	answerSDP, err := s.media.HandleRoomOffer(strconv.FormatUint(roomID, 10), strconv.FormatUint(userID, 10), sdp)
	if err != nil {
		return nil, err
	}
	if err := s.SaveRoomSignalEvent(ctx, organizationID, userID, roomID, "room.offer", map[string]any{
		"sdp":         sdp,
		"answered_at": time.Now().Format(time.RFC3339),
	}); err != nil {
		return nil, err
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err != nil {
		return nil, err
	}
	return &RoomOfferResult{
		State: state,
		Answer: media.OfferAnswer{
			Type: "answer",
			SDP:  answerSDP,
		},
	}, nil
}

func (s *Service) AddRoomICECandidate(ctx context.Context, organizationID, userID, roomID uint64, candidate media.ICECandidateInit) error {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return err
	}
	var memberCount int64
	if err := s.db.WithContext(ctx).Model(&models.CallRoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Count(&memberCount).Error; err != nil {
		return err
	}
	if memberCount == 0 {
		return ErrRoomAccessDenied
	}
	if s.media == nil {
		return errors.New("media engine not attached")
	}
	if err := s.media.AddRoomICECandidate(strconv.FormatUint(roomID, 10), strconv.FormatUint(userID, 10), candidate); err != nil {
		return err
	}
	return s.SaveRoomSignalEvent(ctx, organizationID, userID, roomID, "room.ice", candidate)
}

func (s *Service) LeaveRoom(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var room models.CallRoom
		if err := tx.Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.CallRoomMember{}).
			Where("room_id = ? AND user_id = ?", roomID, userID).
			Updates(map[string]any{"left_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.CallRoomEvent{
			RoomID:      roomID,
			UserID:      userID,
			Type:        "room.leave",
			PayloadJSON: fmt.Sprintf(`{"left_at":"%s"}`, now.Format(time.RFC3339)),
		}).Error; err != nil {
			return err
		}
		var activeCount int64
		if err := tx.Model(&models.CallRoomMember{}).
			Where("room_id = ? AND left_at IS NULL", roomID).
			Count(&activeCount).Error; err != nil {
			return err
		}
		if activeCount == 0 {
			if err := tx.Model(&models.CallRoom{}).
				Where("id = ?", roomID).
				Updates(map[string]any{
					"status":   models.RoomStatusEnded,
					"ended_at": now,
				}).Error; err != nil {
				return err
			}
			if s.outbox != nil {
				var memberIDs []uint64
				if err := tx.Model(&models.CallRoomMember{}).Where("room_id = ?", roomID).Pluck("user_id", &memberIDs).Error; err != nil {
					return err
				}
				startedAt := now
				if room.StartedAt != nil {
					startedAt = *room.StartedAt
				}
				durationSeconds := int64(now.Sub(startedAt).Seconds())
				if durationSeconds < 0 {
					durationSeconds = 0
				}
				for _, memberID := range uniqueUint64s(memberIDs) {
					_, err := s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
						AggregateType:  "room",
						AggregateID:    roomID,
						Event:          "settlement.room.ended",
						IdempotencyKey: fmt.Sprintf("settlement.room.ended:%d:%d", roomID, memberID),
						Payload: map[string]any{
							"event_id":          fmt.Sprintf("room:%d:user:%d:ended", roomID, memberID),
							"organization_id":   organizationID,
							"room_id":           roomID,
							"user_id":           memberID,
							"duration_seconds":  durationSeconds,
							"participant_count": int64(len(memberIDs)),
							"occurred_at":       now.Format(time.RFC3339),
						},
					})
					if err != nil && !errors.Is(err, events.ErrOutboxEventExists) {
						return err
					}
				}
			}
			return s.createConversationSystemMessageTx(ctx, tx, organizationID, userID, room.ConversationID, "meeting.ended", fmt.Sprintf("会议“%s”已结束。", room.Title), map[string]any{
				"room_id":  roomID,
				"ended_at": now.Format(time.RFC3339),
			})
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if s.media != nil {
		_ = s.media.LeaveRoomParticipant(strconv.FormatUint(roomID, 10), strconv.FormatUint(userID, 10))
	}
	state, err := s.GetRoomState(ctx, organizationID, userID, roomID)
	if err != nil {
		return nil, err
	}
	if member := findRoomMember(state.Members, userID); member != nil {
		s.publishRoomMemberUpdated(ctx, organizationID, roomID, *member)
	}
	if state.ConversationID != nil {
		changes := map[string]any{
			"latest_room_id":    state.Room.ID,
			"latest_room_title": state.Room.Title,
		}
		if state.Room.Status == models.RoomStatusEnded {
			changes["active_room_id"] = nil
			changes["active_room_title"] = ""
		}
		s.publishConversationPatchUpdate(ctx, organizationID, *state.ConversationID, changes)
	}
	if state.Room.Status == models.RoomStatusEnded {
		s.publishRoomEnded(ctx, organizationID, state)
	} else {
		s.publishRoomStateUpdated(ctx, organizationID, state, "meeting.left")
	}
	return state, nil
}

func (s *Service) SaveRoomSignalEvent(ctx context.Context, organizationID, userID, roomID uint64, eventType string, payload any) error {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return err
	}
	var memberCount int64
	if err := s.db.WithContext(ctx).Model(&models.CallRoomMember{}).
		Where("room_id = ? AND user_id = ?", roomID, userID).
		Count(&memberCount).Error; err != nil {
		return err
	}
	if memberCount == 0 {
		return ErrRoomAccessDenied
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Create(&models.CallRoomEvent{
		RoomID:      roomID,
		UserID:      userID,
		Type:        eventType,
		PayloadJSON: string(raw),
	}).Error
}

func (s *Service) UpdateRoomMediaState(ctx context.Context, organizationID, userID, roomID uint64, input RoomMediaStateInput) error {
	if input.AudioEnabled == nil && input.VideoEnabled == nil && strings.TrimSpace(input.ConnectionState) == "" {
		return errors.New("at least one media field is required")
	}
	payload := map[string]any{}
	if input.AudioEnabled != nil {
		payload["audio_enabled"] = *input.AudioEnabled
	}
	if input.VideoEnabled != nil {
		payload["video_enabled"] = *input.VideoEnabled
	}
	if value := strings.TrimSpace(input.ConnectionState); value != "" {
		payload["connection_state"] = value
	}
	if err := s.SaveRoomSignalEvent(ctx, organizationID, userID, roomID, "room.media.updated", payload); err != nil {
		s.metrics.Inc("room_media_state_update_fail_total")
		return err
	}
	s.metrics.Inc("room_media_state_update_total")
	memberSummary := RoomMemberSummary{
		CallRoomMember: models.CallRoomMember{RoomID: roomID, UserID: userID},
	}
	if state, err := s.GetRoomState(ctx, organizationID, userID, roomID); err == nil {
		if member := findRoomMember(state.Members, userID); member != nil {
			memberSummary = *member
		}
	}
	s.publishRoomMemberUpdated(ctx, organizationID, roomID, memberSummary)
	return nil
}

func (s *Service) GetRoomState(ctx context.Context, organizationID, userID, roomID uint64) (*RoomState, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	var room models.CallRoom
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", roomID, organizationID).Take(&room).Error; err != nil {
		return nil, err
	}
	var members []RoomMemberSummary
	if err := s.db.WithContext(ctx).
		Table("call_room_members").
		Select("call_room_members.*, users.email AS user_email, users.display_name AS user_display_name").
		Joins("JOIN users ON users.id = call_room_members.user_id").
		Where("call_room_members.room_id = ?", roomID).
		Order("call_room_members.created_at ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	type roomMediaState struct {
		AudioEnabled    *bool  `json:"audio_enabled"`
		VideoEnabled    *bool  `json:"video_enabled"`
		ConnectionState string `json:"connection_state"`
	}
	var mediaEvents []models.CallRoomEvent
	if err := s.db.WithContext(ctx).
		Where("room_id = ? AND type = ?", roomID, "room.media.updated").
		Order("created_at DESC").
		Limit(200).
		Find(&mediaEvents).Error; err != nil {
		return nil, err
	}
	latestMediaState := make(map[uint64]roomMediaState)
	for _, event := range mediaEvents {
		if _, exists := latestMediaState[event.UserID]; exists {
			continue
		}
		var state roomMediaState
		if err := json.Unmarshal([]byte(event.PayloadJSON), &state); err != nil {
			continue
		}
		latestMediaState[event.UserID] = state
	}
	participantCount := int64(0)
	for index := range members {
		member := &members[index]
		member.IsHost = member.Role == models.OrganizationRoleOwner
		member.Joined = member.JoinedAt != nil && member.LeftAt == nil
		member.Left = member.LeftAt != nil
		switch {
		case member.LeftAt != nil:
			member.ConnectionState = "left"
		case member.JoinedAt != nil:
			member.ConnectionState = "connected"
		default:
			member.ConnectionState = "invited"
		}
		isActiveParticipant := member.LeftAt == nil && member.JoinedAt != nil
		if isActiveParticipant {
			participantCount += 1
		}
		member.AudioEnabled = isActiveParticipant
		member.VideoEnabled = isActiveParticipant
		if mediaState, ok := latestMediaState[member.UserID]; ok {
			if mediaState.AudioEnabled != nil {
				member.AudioEnabled = *mediaState.AudioEnabled
			}
			if mediaState.VideoEnabled != nil {
				member.VideoEnabled = *mediaState.VideoEnabled
			}
			if mediaState.ConnectionState != "" {
				member.ConnectionState = mediaState.ConnectionState
			}
		}
		member.Joined = member.ConnectionState != "left" && member.JoinedAt != nil
		member.Left = member.ConnectionState == "left"
	}
	var events []models.CallRoomEvent
	if err := s.db.WithContext(ctx).Where("room_id = ?", roomID).Order("created_at DESC").Limit(50).Find(&events).Error; err != nil {
		return nil, err
	}
	var recording models.RecordingSession
	var recordingPtr *models.RecordingSession
	var latestRecordingID *uint64
	if err := s.db.WithContext(ctx).
		Where("room_id = ? AND status IN ?", roomID, []string{models.RecordingStatusRecording, models.RecordingStatusProcessing}).
		Order("id DESC").Take(&recording).Error; err == nil {
		recordingPtr = &recording
	}
	var latestRecording models.RecordingSession
	if err := s.db.WithContext(ctx).
		Where("room_id = ?", roomID).
		Order("id DESC").Take(&latestRecording).Error; err == nil {
		latestRecordingID = &latestRecording.ID
	}
	conversationTitle := ""
	if room.ConversationID != nil {
		var conv models.Conversation
		if err := s.db.WithContext(ctx).Select("title").Where("id = ?", *room.ConversationID).Take(&conv).Error; err == nil {
			conversationTitle = conv.Title
		}
	}
	return &RoomState{
		Room:              room,
		Members:           members,
		Events:            events,
		ActiveRecording:   recordingPtr,
		ConversationID:    room.ConversationID,
		ConversationTitle: conversationTitle,
		ParticipantCount:  participantCount,
		IsActive:          room.Status == models.RoomStatusActive,
		HasRecording:      recordingPtr != nil || latestRecordingID != nil,
		LatestRecordingID: latestRecordingID,
	}, nil
}
