package collaboration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/events"
	"github.com/allcallall/backend/internal/models"
)

const (
	EventTranscriptSegmentEdited = "transcript.segment.edited"

	// maxTranscriptEditTextRunes 与前端输入框限制一致；转写文本没有理由超过一屏。
	maxTranscriptEditTextRunes = 4096
	// maxTranscriptEditReasonRunes 是修订理由上限，与修订表的 reason 列宽一致。
	maxTranscriptEditReasonRunes = 512
)

var (
	ErrTranscriptSegmentNotFound  = errors.New("transcript segment not found")
	ErrTranscriptEditForbidden    = errors.New("transcript segment edit forbidden")
	ErrTranscriptEditEmptyText    = errors.New("transcript segment text is required")
	ErrTranscriptEditTextTooLong  = errors.New("transcript segment text is too long")
	ErrTranscriptEditReasonLength = errors.New("transcript segment edit reason is too long")
	ErrTranscriptEditConflict     = errors.New("transcript segment was concurrently edited")
)

type TranscriptSegmentEditedPayload struct {
	OrganizationID uint64 `json:"organization_id"`
	ConversationID uint64 `json:"conversation_id"`
	RoomID         uint64 `json:"room_id"`
	RecordingID    uint64 `json:"recording_id"`
	SegmentID      uint64 `json:"segment_id"`
	RevisionNumber int    `json:"revision_number"`
}

// TranscriptSegmentEditResult 是 PATCH 的响应：更新后的段（当前 text）、
// 本次对应的修订号，以及 edited=false 表示“内容未变化”的 200 no-op。
type TranscriptSegmentEditResult struct {
	Segment        models.MeetingTranscriptSegment `json:"segment"`
	RevisionNumber int                             `json:"revision_number"`
	Edited         bool                            `json:"edited"`
}

type TranscriptSegmentRevisionPage struct {
	Revisions    []models.TranscriptSegmentRevision `json:"revisions"`
	NextBeforeID *uint64                            `json:"next_before_id,omitempty"`
}

// ValidateTranscriptEditText 校验编辑文本与可选 reason 的长度边界。
func ValidateTranscriptEditText(text, reason string) error {
	if strings.TrimSpace(text) == "" {
		return ErrTranscriptEditEmptyText
	}
	if len([]rune(text)) > maxTranscriptEditTextRunes {
		return ErrTranscriptEditTextTooLong
	}
	if len([]rune(reason)) > maxTranscriptEditReasonRunes {
		return ErrTranscriptEditReasonLength
	}
	return nil
}

func canEditTranscriptSegment(role string, userID uint64, segment models.MeetingTranscriptSegment) bool {
	if role == models.OrganizationRoleOwner || role == models.OrganizationRoleAdmin {
		return true
	}
	return segment.SpeakerUserID != nil && *segment.SpeakerUserID == userID
}

func (s *Service) loadTranscriptSegmentForRecording(ctx context.Context, organizationID, recordingID, segmentID uint64) (*models.MeetingTranscriptSegment, error) {
	var segment models.MeetingTranscriptSegment
	err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ? AND recording_session_id = ?", segmentID, organizationID, recordingID).
		Take(&segment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTranscriptSegmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &segment, nil
}

func nextTranscriptRevisionNumber(ctx context.Context, tx *gorm.DB, segmentID uint64) (int, error) {
	var maxRevision int
	// 修订表有 (segment_id, revision_number) 唯一索引兜底：并发编辑同一段时
	// 第二个事务插入失败，映射为 409 冲突，而不是悄悄覆盖历史。
	if err := tx.WithContext(ctx).Model(&models.TranscriptSegmentRevision{}).
		Where("segment_id = ?", segmentID).
		Select("COALESCE(MAX(revision_number), 0)").
		Scan(&maxRevision).Error; err != nil {
		return 0, err
	}
	return maxRevision + 1, nil
}

func transcriptTextSHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// EditTranscriptSegment 手工修订一个会议转写片段。段 text 永远保存当前值，
// 完整历史进入 transcript_segment_revisions；每次生效编辑在同一事务里写
// 修订行、组织审计事件（metadata 只放文本哈希，不放全文）与 outbox 事件。
func (s *Service) EditTranscriptSegment(ctx context.Context, organizationID, userID, recordingID, segmentID uint64, text, reason string) (*TranscriptSegmentEditResult, error) {
	if err := ValidateTranscriptEditText(text, reason); err != nil {
		return nil, err
	}
	_, role, err := s.ResolveOrganization(ctx, userID, organizationID)
	if err != nil {
		return nil, err
	}
	segment, err := s.loadTranscriptSegmentForRecording(ctx, organizationID, recordingID, segmentID)
	if err != nil {
		return nil, err
	}
	if !canEditTranscriptSegment(role, userID, *segment) {
		return nil, ErrTranscriptEditForbidden
	}
	if segment.Text == text {
		revisionCount, err := s.countTranscriptRevisions(ctx, segmentID)
		if err != nil {
			return nil, err
		}
		return &TranscriptSegmentEditResult{Segment: *segment, RevisionNumber: revisionCount, Edited: false}, nil
	}

	previousText := segment.Text
	now := time.Now().UTC()
	var updated models.MeetingTranscriptSegment
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		revisionNumber, err := nextTranscriptRevisionNumber(ctx, tx, segmentID)
		if err != nil {
			return err
		}
		result := tx.WithContext(ctx).Model(&models.MeetingTranscriptSegment{}).
			Where("id = ? AND organization_id = ?", segmentID, organizationID).
			Updates(map[string]any{
				"text":           text,
				"edited_at":      now,
				"editor_user_id": userID,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrTranscriptSegmentNotFound
		}
		revision := models.TranscriptSegmentRevision{
			OrganizationID: organizationID,
			SegmentID:      segmentID,
			RevisionNumber: revisionNumber,
			PreviousText:   previousText,
			NextText:       text,
			Reason:         strings.TrimSpace(reason),
			ActorUserID:    userID,
		}
		if err := tx.WithContext(ctx).Create(&revision).Error; err != nil {
			// (segment_id, revision_number) 唯一索引冲突 = 并发编辑，交给调用方
			// 映射为 409；其余数据库错误原样返回。
			if isTranscriptRevisionConflict(err) {
				return ErrTranscriptEditConflict
			}
			return err
		}
		metadata, _ := json.Marshal(map[string]any{
			"recording_id":         recordingID,
			"revision_number":      revisionNumber,
			"previous_text_sha256": transcriptTextSHA256(previousText),
			"next_text_sha256":     transcriptTextSHA256(text),
			"superseded":           false,
		})
		if err := tx.WithContext(ctx).Create(&models.OrganizationAuditEvent{
			OrganizationID: organizationID,
			ActorUserID:    userID,
			Action:         EventTranscriptSegmentEdited,
			TargetType:     "meeting_transcript_segment",
			TargetID:       fmt.Sprintf("%d", segmentID),
			MetadataJSON:   string(metadata),
		}).Error; err != nil {
			return err
		}
		if s.outbox == nil {
			// 没有配置 outbox（例如部分单测环境）时编辑仍然落库；实时投递
			// 由调用方决定是否补发，但主事务不能因此失败。
			return nil
		}
		_, err = s.outbox.EnqueueTx(ctx, tx, events.EnqueueInput{
			AggregateType:  "recording",
			AggregateID:    recordingID,
			Event:          EventTranscriptSegmentEdited,
			IdempotencyKey: fmt.Sprintf("%s:%d:%d", EventTranscriptSegmentEdited, segmentID, revisionNumber),
			Payload: TranscriptSegmentEditedPayload{
				OrganizationID: organizationID,
				ConversationID: segment.ConversationID,
				RoomID:         segment.RoomID,
				RecordingID:    recordingID,
				SegmentID:      segmentID,
				RevisionNumber: revisionNumber,
			},
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).
		Where("id = ? AND organization_id = ?", segmentID, organizationID).
		Take(&updated).Error; err != nil {
		return nil, err
	}
	revisionCount, err := s.countTranscriptRevisions(ctx, segmentID)
	if err != nil {
		return nil, err
	}
	return &TranscriptSegmentEditResult{Segment: updated, RevisionNumber: revisionCount, Edited: true}, nil
}

func (s *Service) countTranscriptRevisions(ctx context.Context, segmentID uint64) (int, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.TranscriptSegmentRevision{}).
		Where("segment_id = ?", segmentID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

// ListTranscriptSegmentRevisions 按新到旧分页返回一个段的修订链。
// before_id 为 0 表示第一页；游标是“上一页最后一条的修订行 ID”。
func (s *Service) ListTranscriptSegmentRevisions(ctx context.Context, organizationID, userID, recordingID, segmentID, beforeID uint64, limit int) (*TranscriptSegmentRevisionPage, error) {
	if _, _, err := s.ResolveOrganization(ctx, userID, organizationID); err != nil {
		return nil, err
	}
	if _, err := s.loadTranscriptSegmentForRecording(ctx, organizationID, recordingID, segmentID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	query := s.db.WithContext(ctx).
		Where("segment_id = ? AND organization_id = ?", segmentID, organizationID)
	if beforeID > 0 {
		query = query.Where("id < ?", beforeID)
	}
	page := &TranscriptSegmentRevisionPage{Revisions: []models.TranscriptSegmentRevision{}}
	if err := query.Order("id DESC").Limit(limit).Find(&page.Revisions).Error; err != nil {
		return nil, err
	}
	if len(page.Revisions) == limit {
		next := page.Revisions[len(page.Revisions)-1].ID
		page.NextBeforeID = &next
	}
	return page, nil
}

// PublishTranscriptSegmentEditedFromOutbox 把转写片段修订作为会话实时事件
// 投递给打开转写页的成员（payload 只含 id，客户端用现有 transcript 查询
// 拉取新文本，避免在事件通道里传播全文）。
func (s *Service) PublishTranscriptSegmentEditedFromOutbox(ctx context.Context, organizationID, conversationID, roomID, recordingID, segmentID uint64, revisionNumber int) error {
	if segmentID == 0 {
		return errors.New("transcript segment id is required")
	}
	s.publishConversationEvent(ctx, organizationID, conversationID, "transcript.segment.edited", map[string]any{
		"room_id":         roomID,
		"recording_id":    recordingID,
		"segment_id":      segmentID,
		"revision_number": revisionNumber,
	})
	return nil
}

func isTranscriptRevisionConflict(err error) bool {
	if err == nil {
		return false
	}
	// MySQL 唯一索引冲突 / SQLite 唯一约束冲突都不值得引入 driver 依赖。
	// SQLite 驱动把唯一冲突包装为 gorm.ErrDuplicatedKey（gorm 1.25+），
	// 其他方言回退到错误文本匹配。
	message := strings.ToLower(err.Error())
	return errors.Is(err, gorm.ErrDuplicatedKey) ||
		strings.Contains(message, "unique constraint") ||
		strings.Contains(message, "duplicate entry")
}
