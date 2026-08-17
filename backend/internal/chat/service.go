package chat

import (
	"context"
	"errors"
	"strings"

	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

// EventPublisher 接收实时事件并投递给指定用户（由 collaboration.ChatHub 实现，
// 已内置跨实例 Redis 桥接，因此本服务无需关心节点拓扑）。
type EventPublisher = collaboration.EventPublisher

// Service 即时通讯群聊服务：群组、消息漫游、富媒体、已读回执、实时投递。
type Service struct {
	db      *gorm.DB
	pub     EventPublisher
	outbox  *events.Store
	metrics metrics.Recorder
	logger  zerolog.Logger
}

// NewService 构造群聊服务。
func NewService(db *gorm.DB, pub EventPublisher) *Service {
	svc := &Service{db: db, pub: pub}
	svc.metrics = metrics.NewCounterStore()
	svc.logger = zerolog.Nop()
	return svc
}

// WithLogger 注入结构化日志。
func (s *Service) WithLogger(logger zerolog.Logger) *Service {
	s.logger = logger
	return s
}

// WithMetrics 注入指标采集器。
func (s *Service) WithMetrics(rec metrics.Recorder) *Service {
	if rec != nil {
		s.metrics = rec
	}
	return s
}

// WithOutbox 接入事件总线，发送消息时把 chat.message.created 落地到 outbox，
// 供下游（如 Kafka 桥接）生产化消费。未设置则不产生 outbox 事件。
func (s *Service) WithOutbox(store *events.Store) *Service {
	s.outbox = store
	return s
}

var (
	ErrGroupNotFound      = errors.New("chat group not found")
	ErrNotGroupMember     = errors.New("not a member of this group")
	ErrOnlyOwnerCanDelete = errors.New("only group owner can remove members")
	ErrMessageNotFound    = errors.New("chat message not found")
	ErrNotMessageSender   = errors.New("only message sender can edit")
)

// ---------- 群组管理 ----------

// CreateGroupInput 创建群组入参。
type CreateGroupInput struct {
	Name        string
	Description string
	AvatarURL   string
	Kind        string
	MemberIDs   []uint64
}

// CreateGroup 创建群组并加入成员（创建者为 owner）。
func (s *Service) CreateGroup(ctx context.Context, orgID, creatorID uint64, in CreateGroupInput) (*GroupView, error) {
	kind := strings.TrimSpace(in.Kind)
	if kind == "" {
		kind = models.ChatGroupKindGroup
	}
	if kind != models.ChatGroupKindGroup && kind != models.ChatGroupKindDirect {
		return nil, errors.New("invalid group kind")
	}
	name := strings.TrimSpace(in.Name)
	if kind == models.ChatGroupKindGroup && name == "" {
		return nil, errors.New("group name required")
	}

	group := models.ChatGroup{
		OrganizationID: orgID,
		Kind:           kind,
		Name:           name,
		Description:    strings.TrimSpace(in.Description),
		AvatarURL:      strings.TrimSpace(in.AvatarURL),
		CreatedBy:      creatorID,
	}
	members := map[uint64]string{creatorID: models.ChatGroupRoleOwner}
	for _, m := range in.MemberIDs {
		if m == 0 || m == creatorID {
			continue
		}
		members[m] = models.ChatGroupRoleMember
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&group).Error; err != nil {
			return err
		}
		for uid, role := range members {
			m := models.ChatGroupMember{
				OrganizationID: orgID,
				GroupID:        group.ID,
				UserID:         uid,
				Role:           role,
			}
			if err := tx.Create(&m).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetGroup(ctx, orgID, creatorID, group.ID)
}

// AddMember 添加群成员（仅 owner/admin）。
func (s *Service) AddMember(ctx context.Context, orgID, actorID, groupID, userID uint64) (*MemberView, error) {
	if _, err := s.requireRole(ctx, orgID, actorID, groupID, models.ChatGroupRoleOwner, models.ChatGroupRoleAdmin); err != nil {
		return nil, err
	}
	if userID == 0 {
		return nil, errors.New("target user id required")
	}
	var existing models.ChatGroupMember
	err := s.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Take(&existing).Error
	if err == nil {
		return s.loadMemberView(ctx, existing)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	m := models.ChatGroupMember{
		OrganizationID: orgID,
		GroupID:        groupID,
		UserID:         userID,
		Role:           models.ChatGroupRoleMember,
	}
	if err := s.db.WithContext(ctx).Create(&m).Error; err != nil {
		return nil, err
	}
	return s.loadMemberView(ctx, m)
}

// RemoveMember 移除群成员（owner 可移除任何人；成员可退群）。
func (s *Service) RemoveMember(ctx context.Context, orgID, actorID, groupID, userID uint64) error {
	self, err := s.requireMember(ctx, orgID, actorID, groupID)
	if err != nil {
		return err
	}
	target, err := s.getMember(ctx, groupID, userID)
	if err != nil {
		return err
	}
	if target.Role == models.ChatGroupRoleOwner {
		return errors.New("cannot remove group owner")
	}
	// 非本人退群，必须是 owner 操作
	if actorID != userID && self.Role != models.ChatGroupRoleOwner {
		return ErrOnlyOwnerCanDelete
	}
	return s.db.WithContext(ctx).
		Where("group_id = ? AND user_id = ?", groupID, userID).
		Delete(&models.ChatGroupMember{}).Error
}

// ListGroups 列出用户所属的全部群组。
func (s *Service) ListGroups(ctx context.Context, orgID, userID uint64) ([]GroupView, error) {
	var members []models.ChatGroupMember
	if err := s.db.WithContext(ctx).
		Where("organization_id = ? AND user_id = ?", orgID, userID).
		Find(&members).Error; err != nil {
		return nil, err
	}
	views := make([]GroupView, 0, len(members))
	for _, m := range members {
		if v, err := s.GetGroup(ctx, orgID, userID, m.GroupID); err == nil {
			views = append(views, *v)
		}
	}
	return views, nil
}

// GetGroup 获取群组详情（含成员）。调用者必须是成员。
func (s *Service) GetGroup(ctx context.Context, orgID, userID, groupID uint64) (*GroupView, error) {
	if _, err := s.requireMember(ctx, orgID, userID, groupID); err != nil {
		return nil, err
	}
	var group models.ChatGroup
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", groupID, orgID).Take(&group).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrGroupNotFound
		}
		return nil, err
	}
	var members []models.ChatGroupMember
	if err := s.db.WithContext(ctx).Where("group_id = ?", groupID).Order("joined_at ASC").Find(&members).Error; err != nil {
		return nil, err
	}
	views := make([]MemberView, 0, len(members))
	for _, m := range members {
		if mv, err := s.loadMemberView(ctx, m); err == nil {
			views = append(views, *mv)
		}
	}
	return &GroupView{Group: group, Members: views}, nil
}
