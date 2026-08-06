package collaboration

import (
	"context"
	"time"

	"github.com/allcallall/backend/internal/models"
)

// PurgeExpiredAuditEvents 物理删除早于 before 的组织审计事件（分批，避免大事务锁表）。
// 审计事件的最短留存期由 config（默认 180 天）决定；到期即清理，超过后不再作为合规证据留存。
// 返回被删除的行数。
// PurgeExpiredAuditEvents deletes organization audit events older than the cutoff, in batches.
func (s *Service) PurgeExpiredAuditEvents(ctx context.Context, before time.Time, batchLimit int) (int64, error) {
	if batchLimit <= 0 {
		batchLimit = 500
	}
	var total int64
	for {
		var ids []uint64
		if err := s.db.WithContext(ctx).
			Model(&models.OrganizationAuditEvent{}).
			Where("created_at < ?", before).
			Limit(batchLimit).
			Pluck("id", &ids).Error; err != nil {
			return total, err
		}
		if len(ids) == 0 {
			break
		}
		res := s.db.WithContext(ctx).
			Where("id IN ?", ids).
			Delete(&models.OrganizationAuditEvent{})
		if res.Error != nil {
			return total, res.Error
		}
		total += res.RowsAffected
		if len(ids) < batchLimit {
			break
		}
	}
	return total, nil
}
