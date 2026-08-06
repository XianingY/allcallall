package collaboration

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// eraseBatchLimit 控制单次擦除扫描的批量大小，避免一次性锁住过多行。
// eraseBatchLimit caps each erase scan to keep row locks short.
const eraseBatchLimit = 500

var (
	// ErrErasureForbidden 表示调用者无权执行擦除（既非本人也非组织 owner/admin）。
	// ErrErasureForbidden means the caller may neither self-erase nor admin-erase.
	ErrErasureForbidden = errors.New("only the user themselves or an organization owner/admin can erase messages")
)

// PurgeUserMessages 行使「被遗忘权」：擦除某用户在组织内的全部消息正文与附件。
//
// 权限模型（与撤回不同，擦除范围更宽、强制力更强）：
//   - 用户本人可擦除自己发送的消息（自行行使删除权）；
//   - 组织 owner/admin 可擦除组织内任意用户的消息（合规下架 / 用户投诉处置）。
//
// 擦除后正文、metadata、信封、附件对象、搜索副本全部销毁，仅保留消息骨架
// 供审计与时间线引用，并写入 erased_at/erased_by 作为事后审计的唯一依据。
// PurgeUserMessages erases all of a user's message content in an org (right to be forgotten).
func (s *Service) PurgeUserMessages(ctx context.Context, organizationID, operatorID, targetUserID uint64) (int64, error) {
	if err := s.authorizeErasure(ctx, organizationID, operatorID, targetUserID); err != nil {
		return 0, err
	}
	now := time.Now()
	total, err := s.eraseMessages(ctx, now, func(db *gorm.DB) *gorm.DB {
		return db.Where("organization_id = ? AND sender_id = ?", organizationID, targetUserID)
	}, operatorID)
	if err != nil {
		return total, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return total, nil
}

// PurgeOrganizationMessages 组织级一键擦除：销毁组织内全部消息正文与附件
// （组织注销 / 全盘合规下架）。仅 owner/admin 可执行，保留消息骨架供审计。
// PurgeOrganizationMessages performs an organization-wide erasure; owner/admin only.
func (s *Service) PurgeOrganizationMessages(ctx context.Context, organizationID, operatorID uint64) (int64, error) {
	if err := s.requireErasureAdmin(ctx, organizationID, operatorID); err != nil {
		return 0, err
	}
	now := time.Now()
	total, err := s.eraseMessages(ctx, now, func(db *gorm.DB) *gorm.DB {
		return db.Where("organization_id = ?", organizationID)
	}, operatorID)
	if err != nil {
		return total, err
	}
	s.invalidateOrganizationAdminSummary(ctx, organizationID)
	return total, nil
}

// eraseMessages 分批把匹配 scope 的消息正文全部清空，并一并销毁其附件与搜索副本。
// scope 只描述「擦除范围」（按组织 / 按发送者），erased_at IS NULL 的兜底由循环自身负责，
// 保证重复调用幂等——已经擦除过的消息不会再被处理。
// eraseMessages batch-clears every matching message body plus its media and search copies.
func (s *Service) eraseMessages(ctx context.Context, now time.Time, scope func(*gorm.DB) *gorm.DB, operatorID uint64) (int64, error) {
	var total int64
	for {
		var batch []models.Message
		if err := s.db.WithContext(ctx).
			Where("erased_at IS NULL").
			Scopes(scope).
			Order("id ASC").
			Limit(eraseBatchLimit).
			Find(&batch).Error; err != nil {
			return total, err
		}
		if len(batch) == 0 {
			break
		}
		ids := make([]uint64, 0, len(batch))
		for i := range batch {
			ids = append(ids, batch[i].ID)
		}
		updates := map[string]any{
			"body":                "",
			"metadata_json":       "",
			"encryption_metadata": "",
			"erased_at":           now,
			"erased_by":           operatorID,
			"updated_at":          now,
		}
		if err := s.db.WithContext(ctx).Model(&models.Message{}).
			Where("id IN ? AND erased_at IS NULL", ids).
			Updates(updates).Error; err != nil {
			return total, err
		}
		total += int64(len(batch))

		// 附件：物理删除这些消息的附件对象（复用撤回/留存共用的销毁路径）。
		// Attachments: physically delete the backing objects via the shared purge path.
		var attachments []models.Attachment
		if err := s.db.WithContext(ctx).
			Where("message_id IN ? AND purged_at IS NULL", ids).
			Find(&attachments).Error; err != nil {
			return total, err
		}
		if len(attachments) > 0 {
			if _, _, err := s.purgeAttachmentRecords(ctx, attachments, now); err != nil {
				s.logger.Warn().Err(err).Msg("failed to purge attachments during erasure")
			}
		}
		// 搜索副本：覆盖写空正文，避免「数据库已擦除、索引仍可检索」的留存漏洞。
		// Search copies: overwrite with the emptied body so the index stops matching.
		for i := range batch {
			if err := s.enqueueSearchReindex(ctx, batch[i], "erased"); err != nil {
				s.logger.Warn().Err(err).Uint64("message_id", batch[i].ID).Msg("failed to reindex during erasure")
			}
		}
		if len(batch) < eraseBatchLimit {
			break
		}
	}
	return total, nil
}

// authorizeErasure 校验擦除权限：本人擦除自己，或管理员擦除组织内任意用户。
// authorizeErasure validates erasure permission: self or admin.
func (s *Service) authorizeErasure(ctx context.Context, organizationID, operatorID, targetUserID uint64) error {
	if operatorID == targetUserID {
		// 自行擦除仍需是组织成员。
		// Self-erasure still requires org membership.
		if _, _, err := s.ResolveOrganization(ctx, operatorID, organizationID); err != nil {
			return ErrErasureForbidden
		}
		return nil
	}
	return s.requireErasureAdmin(ctx, organizationID, operatorID)
}

// requireErasureAdmin 校验调用者是否为组织 owner/admin。
// requireErasureAdmin ensures the caller is an organization owner or admin.
func (s *Service) requireErasureAdmin(ctx context.Context, organizationID, operatorID uint64) error {
	_, role, err := s.ResolveOrganization(ctx, operatorID, organizationID)
	if err != nil {
		return err
	}
	if role != models.OrganizationRoleOwner && role != models.OrganizationRoleAdmin {
		return ErrErasureForbidden
	}
	return nil
}
