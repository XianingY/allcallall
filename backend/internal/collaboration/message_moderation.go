package collaboration

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/allcallall/backend/internal/models"
)

// ModerationService 内容审核抽象。默认提供基于关键词的实现，
// 但接口允许替换为第三方审核 API（异步、按组织策略分级等）。
// 实现必须自行处理超时与降级——审核绝不阻塞消息投递。
// ModerationService abstracts content moderation; the default is keyword-based.
type ModerationService interface {
	// ModerateMessage 对一条消息正文做审核，返回结构化结论。无违规时 Allowed=true。
	// ModerateMessage inspects a message body and returns a structured verdict.
	ModerateMessage(ctx context.Context, organizationID, conversationID, messageID uint64, body string) (*ModerationResult, error)
}

// ModerationResult 审核结论。
// ModerationResult is the verdict of a moderation check.
type ModerationResult struct {
	Allowed  bool     `json:"allowed"`
	Category string   `json:"category"`
	Matched  []string `json:"matched"`
}

// KeywordModerationService 基于关键词命中的默认审核实现（处置违法不良信息）。
// 命中即标记，不阻断投递——即时消息「先发后审」模型下，审核与投递并行进行。
// KeywordModerationService is the default keyword-based moderation implementation.
type KeywordModerationService struct {
	keywords map[string]struct{}
}

// NewKeywordModerationService 构造关键词审核器。关键词统一小写以便不区分大小写匹配。
// NewKeywordModerationService builds a case-insensitive keyword moderator.
func NewKeywordModerationService(keywords ...string) *KeywordModerationService {
	set := make(map[string]struct{}, len(keywords))
	for _, kw := range keywords {
		kw = strings.ToLower(strings.TrimSpace(kw))
		if kw != "" {
			set[kw] = struct{}{}
		}
	}
	return &KeywordModerationService{keywords: set}
}

// ModerateMessage 检查正文是否命中任一关键词。
// ModerateMessage reports whether the body hits any configured keyword.
func (m *KeywordModerationService) ModerateMessage(_ context.Context, _ uint64, _ uint64, _ uint64, body string) (*ModerationResult, error) {
	lower := strings.ToLower(body)
	var matched []string
	for kw := range m.keywords {
		if strings.Contains(lower, kw) {
			matched = append(matched, kw)
		}
	}
	if len(matched) > 0 {
		return &ModerationResult{Allowed: false, Category: "keyword", Matched: matched}, nil
	}
	return &ModerationResult{Allowed: true}, nil
}

// WithModerationService 注入内容审核器（由 runtime 依据 config 装配；不注入则跳过审核）。
// WithModerationService injects the moderation service; nil disables moderation.
func (s *Service) WithModerationService(m ModerationService) *Service {
	s.moderation = m
	return s
}

// runModerationAsync 在消息已落库后异步触发审核，绝不阻塞投递路径。
// 使用独立 context（带超时）避免请求 ctx 取消导致审核丢失。
// runModerationAsync triggers a non-blocking moderation check after the message is stored.
func (s *Service) runModerationAsync(orgID, conversationID, messageID uint64, body string) {
	if s.moderation == nil || body == "" {
		return
	}
	go func() {
		mctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result, err := s.moderation.ModerateMessage(mctx, orgID, conversationID, messageID, body)
		if err != nil {
			// 审核失败只告警不阻断：审核是增强项，不能因为审核服务抖动而丢弃消息。
			// A moderation failure only warns; it must never drop the message.
			s.logger.Warn().Err(err).Uint64("message_id", messageID).Msg("moderation check failed")
			return
		}
		if result == nil || result.Allowed {
			return
		}
		s.handleModerationHit(mctx, orgID, conversationID, messageID, result)
	}()
}

// handleModerationHit 命中违规词后的处置：计数、写审计事件、广播标记事件供客户端/管理员感知。
// handleModerationHit records and broadcasts a moderation flag when a message is hit.
func (s *Service) handleModerationHit(ctx context.Context, orgID, conversationID, messageID uint64, result *ModerationResult) {
	s.metrics.Inc("message_moderation_flagged_total")
	meta, _ := json.Marshal(map[string]any{"category": result.Category, "matched": result.Matched})
	s.recordOrganizationAuditEvent(ctx, orgID, 0, "message.moderated", "message", strconv.FormatUint(messageID, 10), string(meta))
	s.publishConversationEvent(ctx, orgID, conversationID, "message.moderation_flagged", map[string]any{
		"message_id": messageID,
		"category":   result.Category,
		"matched":    result.Matched,
	})
}

// recordOrganizationAuditEvent 写入一条组织审计事件（供合规回溯与 P2-9 留存清理复用）。
// recordOrganizationAuditEvent appends an organization audit trail entry.
func (s *Service) recordOrganizationAuditEvent(ctx context.Context, orgID, actorUserID uint64, action, targetType, targetID, metadataJSON string) {
	if err := s.db.WithContext(ctx).Create(&models.OrganizationAuditEvent{
		OrganizationID: orgID,
		ActorUserID:    actorUserID,
		Action:         action,
		TargetType:     targetType,
		TargetID:       targetID,
		MetadataJSON:   metadataJSON,
	}).Error; err != nil {
		s.logger.Warn().Err(err).Uint64("organization_id", orgID).Msg("failed to record audit event")
	}
}
