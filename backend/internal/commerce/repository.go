package commerce

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// Repository handles all data access operations for the commerce service.
// It separates data access from business logic in the Service layer.
type Repository struct {
	db *gorm.DB
}

// NewRepository creates a new Repository instance.
func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// ─── LegalAcceptance ─────────────────────────────────────────────────────────

// GetLegalAcceptance retrieves the legal acceptance record for a user.
func (r *Repository) GetLegalAcceptance(ctx context.Context, userID uint64) (*models.LegalAcceptance, error) {
	var record models.LegalAcceptance
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Take(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

// UpsertLegalAcceptance creates or updates a legal acceptance record.
func (r *Repository) UpsertLegalAcceptance(ctx context.Context, userID uint64, termsVersion, privacyVersion string) error {
	now := time.Now().UTC()
	record := &models.LegalAcceptance{
		UserID:         userID,
		TermsVersion:   termsVersion,
		PrivacyVersion: privacyVersion,
		AcceptedAt:     now,
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.LegalAcceptance
		err := tx.Where("user_id = ?", userID).Take(&existing).Error
		if err == gorm.ErrRecordNotFound {
			return tx.Create(record).Error
		}
		if err != nil {
			return err
		}
		existing.TermsVersion = termsVersion
		existing.PrivacyVersion = privacyVersion
		existing.AcceptedAt = now
		return tx.Save(&existing).Error
	})
}

// ─── UserEntitlement ────────────────────────────────────────────────────────

// GetActivePremiumEntitlement retrieves the active premium entitlement for a user.
func (r *Repository) GetActivePremiumEntitlement(ctx context.Context, userID uint64) (*models.UserEntitlement, error) {
	var entitlement models.UserEntitlement
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND entitlement = ?", userID, models.EntitlementPremium).
		Where("status = ?", "active").
		Order("updated_at DESC").
		Take(&entitlement).Error
	if err != nil {
		return nil, err
	}
	return &entitlement, nil
}

// GetEntitlementByType retrieves a specific entitlement type for a user.
func (r *Repository) GetEntitlementByType(ctx context.Context, userID uint64, entitlementType string) (*models.UserEntitlement, error) {
	var entitlement models.UserEntitlement
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND entitlement = ?", userID, entitlementType).
		Take(&entitlement).Error
	if err != nil {
		return nil, err
	}
	return &entitlement, nil
}

// GetEntitlements retrieves all entitlements for a user.
func (r *Repository) GetEntitlements(ctx context.Context, userID uint64) ([]models.UserEntitlement, error) {
	var entitlements []models.UserEntitlement
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Find(&entitlements).Error; err != nil {
		return nil, err
	}
	return entitlements, nil
}

// FirstOrCreateFreeEntitlement creates a free entitlement if one doesn't exist.
func (r *Repository) FirstOrCreateFreeEntitlement(ctx context.Context, entitlement *models.UserEntitlement) error {
	return r.db.WithContext(ctx).Where("user_id = ? AND entitlement = ?", entitlement.UserID, models.EntitlementFree).FirstOrCreate(entitlement).Error
}

// SaveEntitlement saves or updates an entitlement record.
func (r *Repository) SaveEntitlement(ctx context.Context, entitlement *models.UserEntitlement) error {
	return r.db.WithContext(ctx).Save(entitlement).Error
}

// ─── UsageLedger ─────────────────────────────────────────────────────────────

// GetUsageLedger retrieves usage ledger for a specific user, feature, and period.
func (r *Repository) GetUsageLedger(ctx context.Context, userID uint64, feature, periodKey string) (*models.UsageLedger, error) {
	var ledger models.UsageLedger
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND feature = ? AND period_key = ?", userID, feature, periodKey).
		Take(&ledger).Error
	if err != nil {
		return nil, err
	}
	return &ledger, nil
}

// FirstOrCreateUsageLedger creates a usage ledger entry if it doesn't exist.
func (r *Repository) FirstOrCreateUsageLedger(ctx context.Context, ledger *models.UsageLedger) error {
	return r.db.WithContext(ctx).
		Where("user_id = ? AND feature = ? AND period_key = ?", ledger.UserID, ledger.Feature, ledger.PeriodKey).
		FirstOrCreate(ledger).Error
}

// SaveUsageLedger saves or updates a usage ledger entry.
func (r *Repository) SaveUsageLedger(ctx context.Context, ledger *models.UsageLedger) error {
	return r.db.WithContext(ctx).Save(ledger).Error
}

// ─── TranslationUsageSlice ───────────────────────────────────────────────────

// FirstOrCreateTranslationUsageSlice creates a translation usage slice if it doesn't exist.
// Returns the number of rows affected (0 if already exists).
func (r *Repository) FirstOrCreateTranslationUsageSlice(ctx context.Context, slice *models.TranslationUsageSlice) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("user_id = ? AND call_id = ? AND slice_index = ?", slice.UserID, slice.CallID, slice.SliceIndex).
		FirstOrCreate(slice)
	return result.RowsAffected, result.Error
}

// GetTranslationUsageSlicesByCall retrieves all translation usage slices for a call.
func (r *Repository) GetTranslationUsageSlicesByCall(ctx context.Context, callID string) ([]models.TranslationUsageSlice, error) {
	var slices []models.TranslationUsageSlice
	if err := r.db.WithContext(ctx).
		Where("call_id = ?", callID).
		Order("slice_index ASC").
		Find(&slices).Error; err != nil {
		return nil, err
	}
	return slices, nil
}

// ─── CallSession ─────────────────────────────────────────────────────────────

// RegisterCallInvite creates or updates a call session record.
func (r *Repository) RegisterCallInvite(ctx context.Context, record *models.CallSession) error {
	return r.db.WithContext(ctx).
		Where("call_id = ?", record.CallID).
		Assign(record).
		FirstOrCreate(record).Error
}

// UpdateCallStatus updates the status of a call session.
func (r *Repository) UpdateCallStatus(ctx context.Context, callID string, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.CallSession{}).Where("call_id = ?", callID).Updates(updates).Error
}

// GetCallSession retrieves a call session by call ID.
func (r *Repository) GetCallSession(ctx context.Context, callID string) (*models.CallSession, error) {
	var call models.CallSession
	if err := r.db.WithContext(ctx).Where("call_id = ?", callID).Take(&call).Error; err != nil {
		return nil, err
	}
	return &call, nil
}

// ListCallSessionsByUser retrieves call sessions for a user within a time window.
func (r *Repository) ListCallSessionsByUser(ctx context.Context, userID uint64, since time.Time) ([]models.CallSession, error) {
	var sessions []models.CallSession
	err := r.db.WithContext(ctx).
		Where("(caller_id = ? OR callee_id = ?) AND started_at >= ?", userID, userID, since).
		Order("started_at DESC").
		Limit(100).
		Find(&sessions).Error
	return sessions, err
}

// CountRecentCallsBetweenUsers counts recent calls between two users within a time window.
func (r *Repository) CountRecentCallsBetweenUsers(ctx context.Context, userID, peerUserID uint64, since time.Time, excludeCallID string) (int64, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&models.CallSession{}).
		Where("started_at >= ?", since).
		Where("status IN ?", []string{models.CallStatusAnswered, models.CallStatusEnded})
	if excludeCallID != "" {
		query = query.Where("call_id <> ?", excludeCallID)
	}
	if err := query.Where(
		"(caller_id = ? AND callee_id = ?) OR (caller_id = ? AND callee_id = ?)",
		userID, peerUserID, peerUserID, userID,
	).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ─── CallTranscriptSegment ───────────────────────────────────────────────────

// CreateTranscriptSegment creates a new transcript segment.
func (r *Repository) CreateTranscriptSegment(ctx context.Context, segment *models.CallTranscriptSegment) error {
	return r.db.WithContext(ctx).Create(segment).Error
}

// GetTranscriptSegmentsByCallAndUser retrieves transcript segments for a call and user.
func (r *Repository) GetTranscriptSegmentsByCallAndUser(ctx context.Context, callID string, userID uint64) ([]models.CallTranscriptSegment, error) {
	var segments []models.CallTranscriptSegment
	if err := r.db.WithContext(ctx).
		Where("call_id = ? AND user_id = ?", callID, userID).
		Order("timestamp_ms ASC").
		Find(&segments).Error; err != nil {
		return nil, err
	}
	return segments, nil
}

// GetTranscriptSegmentsByCall retrieves all transcript segments for a call.
func (r *Repository) GetTranscriptSegmentsByCall(ctx context.Context, callID string) ([]models.CallTranscriptSegment, error) {
	var segments []models.CallTranscriptSegment
	if err := r.db.WithContext(ctx).
		Where("call_id = ?", callID).
		Order("timestamp_ms ASC").
		Find(&segments).Error; err != nil {
		return nil, err
	}
	return segments, nil
}

// DeleteTranscriptSegmentsByCallAndUser deletes transcript segments for a call and user.
func (r *Repository) DeleteTranscriptSegmentsByCallAndUser(ctx context.Context, callID string, userID uint64) error {
	return r.db.WithContext(ctx).
		Where("call_id = ? AND user_id = ?", callID, userID).
		Delete(&models.CallTranscriptSegment{}).Error
}

// ─── CallFollowup ────────────────────────────────────────────────────────────

// GetCallFollowup retrieves a call followup by call ID and user ID.
func (r *Repository) GetCallFollowup(ctx context.Context, callID string, userID uint64) (*models.CallFollowup, error) {
	var followup models.CallFollowup
	if err := r.db.WithContext(ctx).
		Where("call_id = ? AND user_id = ?", callID, userID).
		Take(&followup).Error; err != nil {
		return nil, err
	}
	return &followup, nil
}

// GetCallFollowupByCall retrieves the first call followup by call ID.
func (r *Repository) GetCallFollowupByCall(ctx context.Context, callID string) (*models.CallFollowup, error) {
	var followup models.CallFollowup
	if err := r.db.WithContext(ctx).Where("call_id = ?", callID).Order("user_id ASC").Take(&followup).Error; err != nil {
		return nil, err
	}
	return &followup, nil
}

// GetCallFollowupsByCalls retrieves followups for multiple calls and a user.
func (r *Repository) GetCallFollowupsByCalls(ctx context.Context, callIDs []string, userID uint64) ([]models.CallFollowup, error) {
	var followups []models.CallFollowup
	if err := r.db.WithContext(ctx).Where("call_id IN ? AND user_id = ?", callIDs, userID).Find(&followups).Error; err != nil {
		return nil, err
	}
	return followups, nil
}

// SaveCallFollowup saves or updates a call followup record.
func (r *Repository) SaveCallFollowup(ctx context.Context, followup *models.CallFollowup) error {
	return r.db.WithContext(ctx).Save(followup).Error
}

// UpdateCallFollowup updates specific fields of a call followup.
func (r *Repository) UpdateCallFollowup(ctx context.Context, followup *models.CallFollowup, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(followup).Updates(updates).Error
}

// ─── FollowUpTask ────────────────────────────────────────────────────────────

// GetFollowUpTask retrieves a followup task by ID and user ID.
func (r *Repository) GetFollowUpTask(ctx context.Context, taskID, userID uint64) (*models.FollowUpTask, error) {
	var task models.FollowUpTask
	if err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", taskID, userID).Take(&task).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

// GetFollowUpTaskByCallAndType retrieves a followup task by call ID, user ID, and type.
func (r *Repository) GetFollowUpTaskByCallAndType(ctx context.Context, callID string, userID uint64, taskType string) (*models.FollowUpTask, error) {
	var task models.FollowUpTask
	err := r.db.WithContext(ctx).
		Where("call_id = ? AND user_id = ? AND type = ?", callID, userID, taskType).
		Take(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// ListFollowUpTasksByUser retrieves all followup tasks for a user.
func (r *Repository) ListFollowUpTasksByUser(ctx context.Context, userID uint64) ([]models.FollowUpTask, error) {
	var tasks []models.FollowUpTask
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(100).
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// GetFollowUpTasksByCalls retrieves followup tasks for multiple calls and a user.
func (r *Repository) GetFollowUpTasksByCalls(ctx context.Context, callIDs []string, userID uint64, statuses []string) ([]models.FollowUpTask, error) {
	var tasks []models.FollowUpTask
	if err := r.db.WithContext(ctx).
		Where("call_id IN ? AND user_id = ? AND status IN ?", callIDs, userID, statuses).
		Order("due_at ASC").
		Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

// CreateFollowUpTask creates a new followup task.
func (r *Repository) CreateFollowUpTask(ctx context.Context, task *models.FollowUpTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

// UpdateFollowUpTask updates specific fields of a followup task.
func (r *Repository) UpdateFollowUpTask(ctx context.Context, taskID uint64, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.FollowUpTask{}).Where("id = ?", taskID).Updates(updates).Error
}

// UpdateFollowUpTasksByUserPeerType updates followup tasks by user, peer, and type.
func (r *Repository) UpdateFollowUpTasksByUserPeerType(ctx context.Context, userID, peerUserID uint64, taskType, taskStatus string, updates map[string]any) error {
	return r.db.WithContext(ctx).
		Model(&models.FollowUpTask{}).
		Where("user_id = ? AND peer_user_id = ? AND type = ? AND status = ?", userID, peerUserID, taskType, taskStatus).
		Updates(updates).Error
}

// ─── UserBlock ───────────────────────────────────────────────────────────────

// FirstOrCreateUserBlock creates a user block if it doesn't exist.
func (r *Repository) FirstOrCreateUserBlock(ctx context.Context, block *models.UserBlock) error {
	return r.db.WithContext(ctx).
		Where("blocker_id = ? AND blocked_user_id = ?", block.BlockerID, block.BlockedUserID).
		FirstOrCreate(block).Error
}

// DeleteUserBlock deletes a user block relationship.
func (r *Repository) DeleteUserBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	return r.db.WithContext(ctx).
		Where("blocker_id = ? AND blocked_user_id = ?", blockerID, blockedUserID).
		Delete(&models.UserBlock{}).Error
}

// ListUserBlocks retrieves all blocks by a user.
func (r *Repository) ListUserBlocks(ctx context.Context, blockerID uint64) ([]models.UserBlock, error) {
	var blocks []models.UserBlock
	if err := r.db.WithContext(ctx).Where("blocker_id = ?", blockerID).Find(&blocks).Error; err != nil {
		return nil, err
	}
	return blocks, nil
}

// CountUserBlocksBetweenUsers counts block relationships between two users.
func (r *Repository) CountUserBlocksBetweenUsers(ctx context.Context, userA, userB uint64) (int64, error) {
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.UserBlock{}).
		Where("(blocker_id = ? AND blocked_user_id = ?) OR (blocker_id = ? AND blocked_user_id = ?)", userA, userB, userB, userA).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ─── AbuseReport ─────────────────────────────────────────────────────────────

// CreateAbuseReport creates a new abuse report.
func (r *Repository) CreateAbuseReport(ctx context.Context, report *models.AbuseReport) error {
	return r.db.WithContext(ctx).Create(report).Error
}

// ListAbuseReports retrieves abuse reports with a limit.
func (r *Repository) ListAbuseReports(ctx context.Context, limit int) ([]models.AbuseReport, error) {
	var reports []models.AbuseReport
	if err := r.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(limit).
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

// ListAbuseReportsByUser retrieves abuse reports where a user is reporter or reported.
func (r *Repository) ListAbuseReportsByUser(ctx context.Context, userID uint64, limit int) ([]models.AbuseReport, error) {
	var reports []models.AbuseReport
	if err := r.db.WithContext(ctx).
		Where("reporter_id = ? OR reported_user_id = ?", userID, userID).
		Order("created_at DESC").
		Limit(limit).
		Find(&reports).Error; err != nil {
		return nil, err
	}
	return reports, nil
}

// ─── User ────────────────────────────────────────────────────────────────────

// GetUser retrieves a user by ID.
func (r *Repository) GetUser(ctx context.Context, userID uint64) (*models.User, error) {
	var user models.User
	if err := r.db.WithContext(ctx).Where("id = ?", userID).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUsersByIDs retrieves multiple users by their IDs.
func (r *Repository) GetUsersByIDs(ctx context.Context, userIDs []uint64) ([]models.User, error) {
	var users []models.User
	if err := r.db.WithContext(ctx).Where("id IN ?", userIDs).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// UpdateUser updates specific fields of a user.
func (r *Repository) UpdateUser(ctx context.Context, userID uint64, updates map[string]any) error {
	return r.db.WithContext(ctx).Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error
}

// ─── ContactProfile ──────────────────────────────────────────────────────────

// GetContactProfilesByOwnerAndContacts retrieves contact profiles for an owner and contact user IDs.
func (r *Repository) GetContactProfilesByOwnerAndContacts(ctx context.Context, ownerID uint64, contactUserIDs []uint64) ([]models.ContactProfile, error) {
	var contacts []models.ContactProfile
	if err := r.db.WithContext(ctx).
		Where("owner_id = ? AND contact_user_id IN ?", ownerID, contactUserIDs).
		Find(&contacts).Error; err != nil {
		return nil, err
	}
	return contacts, nil
}

// ─── Contact ─────────────────────────────────────────────────────────────────

func (r *Repository) DeleteContactsByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("owner_id = ? OR contact_id = ?", userID, userID).Delete(&models.Contact{})
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteCallsByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("caller_id = ? OR callee_id = ?", userID, userID).Delete(&models.CallSession{})
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteBlocksByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("blocker_id = ? OR blocked_user_id = ?", userID, userID).Delete(&models.UserBlock{})
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteAbuseReportsByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("reporter_id = ? OR reported_user_id = ?", userID, userID).Delete(&models.AbuseReport{})
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteLegalAcceptancesByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.LegalAcceptance{})
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteUsageLedgersByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.UsageLedger{})
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteTranslationUsageSlicesByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.TranslationUsageSlice{})
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteEntitlementsByUser(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&models.UserEntitlement{})
	return result.RowsAffected, result.Error
}

// ─── RefreshSession ──────────────────────────────────────────────────────────

// CountActiveRefreshSessions counts active refresh sessions for a user.
func (r *Repository) CountActiveRefreshSessions(ctx context.Context, userID uint64, now time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at > ?", userID, now).
		Count(&count).Error
	return count, err
}

// CountRevokedRefreshSessions counts revoked refresh sessions for a user.
func (r *Repository) CountRevokedRefreshSessions(ctx context.Context, userID uint64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NOT NULL", userID).
		Count(&count).Error
	return count, err
}

// CountExpiredRefreshSessions counts expired refresh sessions for a user.
func (r *Repository) CountExpiredRefreshSessions(ctx context.Context, userID uint64, now time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL AND expires_at <= ?", userID, now).
		Count(&count).Error
	return count, err
}

// SumInvalidUseCountRefreshSessions sums invalid use count for refresh sessions.
func (r *Repository) SumInvalidUseCountRefreshSessions(ctx context.Context, userID uint64) (int64, error) {
	var aggregate struct {
		InvalidUseCount int64
	}
	err := r.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(invalid_use_count), 0) AS invalid_use_count").
		Scan(&aggregate).Error
	return aggregate.InvalidUseCount, err
}

// GetLatestInvalidRefreshSession retrieves the latest refresh session with invalid use.
func (r *Repository) GetLatestInvalidRefreshSession(ctx context.Context, userID uint64) (*models.RefreshSession, error) {
	var session models.RefreshSession
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND last_invalid_use_at IS NOT NULL", userID).
		Order("last_invalid_use_at DESC").
		Take(&session).Error
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// ListRecentRefreshSessions retrieves recent refresh sessions for a user.
func (r *Repository) ListRecentRefreshSessions(ctx context.Context, userID uint64, limit int) ([]models.RefreshSession, error) {
	var sessions []models.RefreshSession
	err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("updated_at DESC, id DESC").
		Limit(limit).
		Find(&sessions).Error
	return sessions, err
}

// RevokeRefreshSessions revokes refresh sessions for a user.
func (r *Repository) RevokeRefreshSessions(ctx context.Context, userID uint64, sessionID *uint64, now time.Time) (int64, error) {
	query := r.db.WithContext(ctx).Model(&models.RefreshSession{}).
		Where("user_id = ? AND revoked_at IS NULL", userID)
	if sessionID != nil {
		query = query.Where("id = ?", *sessionID)
	}
	result := query.Updates(map[string]any{
		"revoked_at":   now,
		"last_used_at": now,
	})
	return result.RowsAffected, result.Error
}

// ─── BillingWebhookEvent ─────────────────────────────────────────────────────

// GetBillingWebhookEvent retrieves a billing webhook event by event ID.
func (r *Repository) GetBillingWebhookEvent(ctx context.Context, eventID string) (*models.BillingWebhookEvent, error) {
	var event models.BillingWebhookEvent
	if err := r.db.WithContext(ctx).Where("event_id = ?", eventID).Take(&event).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

// CreateBillingWebhookEvent creates a new billing webhook event.
func (r *Repository) CreateBillingWebhookEvent(ctx context.Context, event *models.BillingWebhookEvent) error {
	return r.db.WithContext(ctx).Create(event).Error
}

// SaveBillingWebhookEvent saves or updates a billing webhook event.
func (r *Repository) SaveBillingWebhookEvent(ctx context.Context, event *models.BillingWebhookEvent) error {
	return r.db.WithContext(ctx).Save(event).Error
}

// ─── DeletionAudit ───────────────────────────────────────────────────────────

// GetLatestDeletionAudit retrieves the latest deletion audit for a user.
func (r *Repository) GetLatestDeletionAudit(ctx context.Context, userID uint64) (*models.DeletionAudit, error) {
	var audit models.DeletionAudit
	if err := r.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("deleted_at DESC").
		Take(&audit).Error; err != nil {
		return nil, err
	}
	return &audit, nil
}

// CreateDeletionAudit creates a new deletion audit record.
func (r *Repository) CreateDeletionAudit(ctx context.Context, audit *models.DeletionAudit) error {
	return r.db.WithContext(ctx).Create(audit).Error
}

// ─── EmailVerificationCode ───────────────────────────────────────────────────

// DeleteEmailVerificationCodesByUserEmail deletes email verification codes by user email.
func (r *Repository) DeleteEmailVerificationCodesByUserEmail(ctx context.Context, userID uint64) (int64, error) {
	result := r.db.WithContext(ctx).
		Where("email IN (?)", r.db.WithContext(ctx).Model(&models.User{}).Select("email").Where("id = ?", userID)).
		Delete(&models.EmailVerificationCode{})
	return result.RowsAffected, result.Error
}

// ─── Transaction Helper ──────────────────────────────────────────────────────

// RunInTransaction executes a function within a database transaction.
func (r *Repository) RunInTransaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return r.db.WithContext(ctx).Transaction(fn)
}
