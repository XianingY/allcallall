package chat

import (
	"context"
	"encoding/json"
	"errors"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// ---------- 内部辅助 ----------

func (s *Service) requireMember(ctx context.Context, orgID, userID, groupID uint64) (*models.ChatGroupMember, error) {
	m, err := s.getMember(ctx, groupID, userID)
	if err != nil {
		return nil, err
	}
	if m.OrganizationID != orgID {
		return nil, ErrNotGroupMember
	}
	return m, nil
}

func (s *Service) requireRole(ctx context.Context, orgID, userID, groupID uint64, roles ...string) (*models.ChatGroupMember, error) {
	m, err := s.requireMember(ctx, orgID, userID, groupID)
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if m.Role == r {
			return m, nil
		}
	}
	return nil, errors.New("insufficient group role")
}

func (s *Service) getMember(ctx context.Context, groupID, userID uint64) (*models.ChatGroupMember, error) {
	var m models.ChatGroupMember
	if err := s.db.WithContext(ctx).Where("group_id = ? AND user_id = ?", groupID, userID).Take(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotGroupMember
		}
		return nil, err
	}
	return &m, nil
}

func (s *Service) listMemberIDs(ctx context.Context, groupID uint64) ([]uint64, error) {
	var ids []uint64
	if err := s.db.WithContext(ctx).Model(&models.ChatGroupMember{}).
		Where("group_id = ?", groupID).Pluck("user_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Service) loadMemberView(ctx context.Context, m models.ChatGroupMember) (*MemberView, error) {
	v := &MemberView{
		UserID:            m.UserID,
		Role:              m.Role,
		Muted:             m.MutedAt != nil,
		LastReadMessageID: m.LastReadMessageID,
		LastReadAt:        m.LastReadAt,
		JoinedAt:          m.JoinedAt,
	}
	var u models.User
	if err := s.db.WithContext(ctx).Where("id = ?", m.UserID).Take(&u).Error; err == nil {
		v.Email = u.Email
		v.DisplayName = u.DisplayName
	}
	return v, nil
}

func (s *Service) loadMessageView(ctx context.Context, m models.ChatMessage) (*MessageView, error) {
	v := &MessageView{ChatMessage: m}
	if m.MetadataJSON != "" {
		_ = json.Unmarshal([]byte(m.MetadataJSON), &v.Metadata)
	}
	var u models.User
	if err := s.db.WithContext(ctx).Where("id = ?", m.SenderID).Take(&u).Error; err == nil {
		v.SenderEmail = u.Email
		v.SenderName = u.DisplayName
	}
	return v, nil
}

func (s *Service) hydrateUserViews(ctx context.Context, views []ReadReceiptView) {
	ids := make([]uint64, 0, len(views))
	for _, v := range views {
		ids = append(ids, v.UserID)
	}
	if len(ids) == 0 {
		return
	}
	var users []models.User
	if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&users).Error; err != nil {
		return
	}
	byID := make(map[uint64]models.User, len(users))
	for _, u := range users {
		byID[u.ID] = u
	}
	for i := range views {
		if u, ok := byID[views[i].UserID]; ok {
			views[i].Email = u.Email
			views[i].DisplayName = u.DisplayName
		}
	}
}

func validMessageType(t string) bool {
	switch t {
	case models.ChatMessageTypeText, models.ChatMessageTypeImage, models.ChatMessageTypeFile,
		models.ChatMessageTypeAudio, models.ChatMessageTypeVideo, models.ChatMessageTypeLocation, models.ChatMessageTypeSystem:
		return true
	}
	return false
}

func uniqueUint64s(in []uint64) []uint64 {
	seen := map[uint64]bool{}
	out := make([]uint64, 0, len(in))
	for _, v := range in {
		if v == 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
