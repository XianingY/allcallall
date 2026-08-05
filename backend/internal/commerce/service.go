package commerce

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

var (
	ErrUserBlocked               = errors.New("user is blocked")
	ErrSubscriptionRequired      = errors.New("premium subscription required")
	ErrTranslationQuotaExhausted = errors.New("translation quota exhausted")
	ErrWebhookAlreadyProcessed   = errors.New("billing webhook already processed")
	ErrInvalidReportCategory     = errors.New("invalid abuse report category")
	ErrSupportTokenNotConfigured = errors.New("support token is not configured")
	ErrFollowupNotFound          = errors.New("call follow-up not found")
)

// Service is the coordinator that delegates to focused sub-services.
// All callers continue to use *Service for backward compatibility.
type Service struct {
	repo        *Repository
	entitlement *EntitlementService
	legal       *LegalService
	billing     *BillingWebhookService
	support     *SupportService
	callHistory *CallHistoryService
	followUp    *FollowUpService
	blockAbuse  *BlockAbuseService
}

func NewService(db *gorm.DB, recorders ...metrics.Recorder) *Service {
	return NewServiceWithRepository(NewRepository(db), recorders...)
}

func NewServiceWithRepository(repo *Repository, recorders ...metrics.Recorder) *Service {
	var recorder metrics.Recorder
	if len(recorders) > 0 {
		recorder = recorders[0]
	}

	entitlement := NewEntitlementService(repo, recorder)
	blockAbuse := NewBlockAbuseService(repo)
	callHistory := NewCallHistoryService(repo, recorder, nil) // wire follow-up after creation
	followUp := NewFollowUpService(repo, recorder)
	callHistory.followups = followUp // break circular dependency via late binding

	return &Service{
		repo:        repo,
		entitlement: entitlement,
		legal:       NewLegalService(repo),
		billing:     NewBillingWebhookService(repo, recorder),
		support:     NewSupportService(repo, entitlement, callHistory, blockAbuse),
		callHistory: callHistory,
		followUp:    followUp,
		blockAbuse:  blockAbuse,
	}
}

// ─── Legal delegation ────────────────────────────────────────────────────────

func (s *Service) CurrentLegal() LegalDocumentSet { return s.legal.CurrentLegal() }
func (s *Service) AcceptLegal(ctx context.Context, userID uint64) error {
	return s.legal.AcceptLegal(ctx, userID)
}
func (s *Service) GetLegalAcceptance(ctx context.Context, userID uint64) (*models.LegalAcceptance, error) {
	return s.legal.GetLegalAcceptance(ctx, userID)
}

// ─── Entitlement delegation ──────────────────────────────────────────────────

func (s *Service) EnsureDefaultEntitlement(ctx context.Context, userID uint64) (*models.UserEntitlement, error) {
	return s.entitlement.EnsureDefaultEntitlement(ctx, userID)
}
func (s *Service) GetEntitlements(ctx context.Context, userID uint64) ([]models.UserEntitlement, error) {
	return s.entitlement.GetEntitlements(ctx, userID)
}
func (s *Service) ActiveTier(ctx context.Context, userID uint64) (string, error) {
	return s.entitlement.ActiveTier(ctx, userID)
}
func (s *Service) GetUsage(ctx context.Context, userID uint64) ([]UsageSnapshot, error) {
	return s.entitlement.GetUsage(ctx, userID)
}
func (s *Service) ConsumeTranslationMinutes(ctx context.Context, userID uint64, delta int64) error {
	return s.entitlement.ConsumeTranslationMinutes(ctx, userID, delta)
}
func (s *Service) ConsumeTranslationSeconds(ctx context.Context, userID uint64, deltaSeconds int64) error {
	return s.entitlement.ConsumeTranslationSeconds(ctx, userID, deltaSeconds)
}
func (s *Service) RecordTranslationUsageSlice(ctx context.Context, userID uint64, callID string, eventTimestampMS int64) (bool, error) {
	return s.entitlement.RecordTranslationUsageSlice(ctx, userID, callID, eventTimestampMS)
}

// ─── Call history delegation ─────────────────────────────────────────────────

func (s *Service) RegisterCallInvite(ctx context.Context, callID string, caller *models.User, callee *models.User) error {
	return s.callHistory.RegisterCallInvite(ctx, callID, caller, callee)
}
func (s *Service) RecordTranscriptSegment(ctx context.Context, segment models.CallTranscriptSegment) error {
	return s.callHistory.RecordTranscriptSegment(ctx, segment)
}
func (s *Service) MarkFollowupSecondCallCompleted(ctx context.Context, userID, peerUserID uint64, callID string, completedAt time.Time) error {
	return s.callHistory.MarkFollowupSecondCallCompleted(ctx, userID, peerUserID, callID, completedAt)
}
func (s *Service) UpdateCallStatus(ctx context.Context, callID string, status string, endReason string) error {
	return s.callHistory.UpdateCallStatus(ctx, callID, status, endReason)
}
func (s *Service) ListCallHistory(ctx context.Context, userID uint64, days int) ([]CallHistoryEntry, error) {
	return s.callHistory.ListCallHistory(ctx, userID, days)
}

// ─── Follow-up delegation ────────────────────────────────────────────────────

func (s *Service) GetFollowup(ctx context.Context, userID uint64, callID string) (*FollowupResponse, error) {
	return s.followUp.GetFollowup(ctx, userID, callID)
}
func (s *Service) ListFollowUpTasks(ctx context.Context, userID uint64) ([]FollowUpListItem, error) {
	return s.followUp.ListFollowUpTasks(ctx, userID)
}
func (s *Service) CreateFollowUpTask(ctx context.Context, task *models.FollowUpTask) (*models.FollowUpTask, error) {
	return s.followUp.CreateFollowUpTask(ctx, task)
}
func (s *Service) UpdateFollowUpTask(ctx context.Context, userID, taskID uint64, updates map[string]any) (*models.FollowUpTask, error) {
	return s.followUp.UpdateFollowUpTask(ctx, userID, taskID, updates)
}
func (s *Service) GenerateFollowupForCall(ctx context.Context, callID string, force bool) error {
	return s.followUp.GenerateFollowupForCall(ctx, callID, force)
}

// ─── Block/abuse delegation ──────────────────────────────────────────────────

func (s *Service) CreateBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	return s.blockAbuse.CreateBlock(ctx, blockerID, blockedUserID)
}
func (s *Service) RemoveBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	return s.blockAbuse.RemoveBlock(ctx, blockerID, blockedUserID)
}
func (s *Service) ListBlocks(ctx context.Context, blockerID uint64) ([]models.UserBlock, error) {
	return s.blockAbuse.ListBlocks(ctx, blockerID)
}
func (s *Service) AreUsersBlocked(ctx context.Context, userA, userB uint64) (bool, error) {
	return s.blockAbuse.AreUsersBlocked(ctx, userA, userB)
}
func (s *Service) CreateReport(ctx context.Context, reporterID, reportedUserID uint64, category, details string) error {
	return s.blockAbuse.CreateReport(ctx, reporterID, reportedUserID, category, details)
}
func (s *Service) ReportCategories() []string { return s.blockAbuse.ReportCategories() }

// ─── Billing delegation ──────────────────────────────────────────────────────

func (s *Service) HandleRevenueCatWebhook(ctx context.Context, payload RevenueCatWebhook, raw []byte) error {
	return s.billing.HandleRevenueCatWebhook(ctx, payload, raw)
}

// ─── Support delegation ──────────────────────────────────────────────────────

func (s *Service) ListSupportReports(ctx context.Context, limit int) ([]SupportReportRecord, error) {
	return s.support.ListSupportReports(ctx, limit)
}
func (s *Service) GetSupportUserSummary(ctx context.Context, userID uint64) (*SupportUserSummary, error) {
	return s.support.GetSupportUserSummary(ctx, userID)
}
func (s *Service) RevokeSupportRefreshSessions(ctx context.Context, userID uint64, sessionID *uint64) (*SupportRefreshSessionRevocation, error) {
	return s.support.RevokeSupportRefreshSessions(ctx, userID, sessionID)
}
func (s *Service) GetSupportCall(ctx context.Context, callID string) (*SupportCallDetails, error) {
	return s.support.GetSupportCall(ctx, callID)
}

// getSupportRefreshSessionSummary delegates to the support sub-service (used by tests).
func (s *Service) getSupportRefreshSessionSummary(ctx context.Context, userID uint64) (SupportRefreshSessionSummary, error) {
	return s.support.getSupportRefreshSessionSummary(ctx, userID)
}

// ─── Cross-cutting: DeleteAccount ────────────────────────────────────────────

func (s *Service) DeleteAccount(ctx context.Context, userID uint64) (*models.DeletionAudit, error) {
	audit := &models.DeletionAudit{
		UserID:    userID,
		DeletedAt: time.Now().UTC(),
	}

	err := s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		var err error
		if audit.ContactsDeleted, err = s.repo.DeleteContactsByUser(ctx, userID); err != nil {
			return err
		}
		if audit.CallsDeleted, err = s.repo.DeleteCallsByUser(ctx, userID); err != nil {
			return err
		}
		if audit.BlocksDeleted, err = s.repo.DeleteBlocksByUser(ctx, userID); err != nil {
			return err
		}
		if audit.ReportsDeleted, err = s.repo.DeleteAbuseReportsByUser(ctx, userID); err != nil {
			return err
		}
		if audit.LegalRecordsDeleted, err = s.repo.DeleteLegalAcceptancesByUser(ctx, userID); err != nil {
			return err
		}
		if audit.UsageRowsDeleted, err = s.repo.DeleteUsageLedgersByUser(ctx, userID); err != nil {
			return err
		}
		usageSlicesDeleted, err := s.repo.DeleteTranslationUsageSlicesByUser(ctx, userID)
		if err != nil {
			return err
		}
		audit.UsageRowsDeleted += usageSlicesDeleted
		if audit.EntitlementsDeleted, err = s.repo.DeleteEntitlementsByUser(ctx, userID); err != nil {
			return err
		}

		if _, err = s.repo.DeleteEmailVerificationCodesByUserEmail(ctx, userID); err != nil {
			return err
		}

		updates := map[string]any{
			"email":         fmt.Sprintf("deleted-user-%d@deleted.local", userID),
			"display_name":  "Deleted User",
			"password_hash": "deleted",
			"fcm_token":     "",
			"status":        models.UserStatusDeleted,
			"deleted_at":    audit.DeletedAt,
			"updated_at":    audit.DeletedAt,
		}
		if err := s.repo.UpdateUser(ctx, userID, updates); err != nil {
			return err
		}
		return s.repo.CreateDeletionAudit(ctx, audit)
	})

	if err != nil {
		return nil, err
	}
	return audit, nil
}

// ─── Utilities ───────────────────────────────────────────────────────────────

// EncodePayload JSON-encodes a value, discarding errors.
func EncodePayload(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}
