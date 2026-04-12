package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

const (
	translationFreeMinutesPerMonth = int64(30)

	legalTermsVersion   = "2026-04-11"
	legalPrivacyVersion = "2026-04-11"

	revenueCatSource = "revenuecat"
)

var (
	ErrUserBlocked            = errors.New("user is blocked")
	ErrSubscriptionRequired   = errors.New("premium subscription required")
	ErrTranslationQuotaExhausted = errors.New("translation quota exhausted")
	ErrWebhookAlreadyProcessed = errors.New("billing webhook already processed")
)

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

type LegalDocumentSet struct {
	TermsVersion      string `json:"terms_version"`
	PrivacyVersion    string `json:"privacy_version"`
	TermsURL          string `json:"terms_url"`
	PrivacyPolicyURL  string `json:"privacy_policy_url"`
	SupportEmail      string `json:"support_email"`
	AccountDeletionURL string `json:"account_deletion_url"`
}

func (s *Service) CurrentLegal() LegalDocumentSet {
	return LegalDocumentSet{
		TermsVersion:       legalTermsVersion,
		PrivacyVersion:     legalPrivacyVersion,
		TermsURL:           "https://allcallall.app/legal/terms",
		PrivacyPolicyURL:   "https://allcallall.app/legal/privacy",
		SupportEmail:       "support@allcallall.app",
		AccountDeletionURL: "https://allcallall.app/legal/delete-account",
	}
}

func (s *Service) AcceptLegal(ctx context.Context, userID uint64) error {
	now := time.Now().UTC()
	record := &models.LegalAcceptance{
		UserID:         userID,
		TermsVersion:   legalTermsVersion,
		PrivacyVersion: legalPrivacyVersion,
		AcceptedAt:     now,
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.LegalAcceptance
		err := tx.Where("user_id = ?", userID).Take(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(record).Error
		}
		if err != nil {
			return err
		}
		existing.TermsVersion = legalTermsVersion
		existing.PrivacyVersion = legalPrivacyVersion
		existing.AcceptedAt = now
		return tx.Save(&existing).Error
	})
}

func (s *Service) GetLegalAcceptance(ctx context.Context, userID uint64) (*models.LegalAcceptance, error) {
	var record models.LegalAcceptance
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Take(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *Service) EnsureDefaultEntitlement(ctx context.Context, userID uint64) (*models.UserEntitlement, error) {
	now := time.Now().UTC()
	var entitlement models.UserEntitlement
	err := s.db.WithContext(ctx).
		Where("user_id = ? AND entitlement = ?", userID, models.EntitlementPremium).
		Where("status = ?", "active").
		Order("updated_at DESC").
		Take(&entitlement).Error
	if err == nil {
		return &entitlement, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	entitlement = models.UserEntitlement{
		UserID:       userID,
		Entitlement:  models.EntitlementFree,
		Tier:         models.EntitlementFree,
		Source:       "system",
		Status:       "active",
		LastSyncedAt: &now,
	}

	if err := s.db.WithContext(ctx).Where("user_id = ? AND entitlement = ?", userID, models.EntitlementFree).FirstOrCreate(&entitlement).Error; err != nil {
		return nil, err
	}
	return &entitlement, nil
}

func (s *Service) GetEntitlements(ctx context.Context, userID uint64) ([]models.UserEntitlement, error) {
	var entitlements []models.UserEntitlement
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Find(&entitlements).Error; err != nil {
		return nil, err
	}
	if len(entitlements) == 0 {
		entitlement, err := s.EnsureDefaultEntitlement(ctx, userID)
		if err != nil {
			return nil, err
		}
		return []models.UserEntitlement{*entitlement}, nil
	}
	return entitlements, nil
}

func (s *Service) ActiveTier(ctx context.Context, userID uint64) (string, error) {
	entitlements, err := s.GetEntitlements(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, item := range entitlements {
		if item.Status != "active" {
			continue
		}
		if item.Entitlement == models.EntitlementPremium {
			if item.ExpiresAt == nil || item.ExpiresAt.After(time.Now().UTC()) {
				return models.EntitlementPremium, nil
			}
		}
	}
	return models.EntitlementFree, nil
}

type UsageSnapshot struct {
	Feature        string `json:"feature"`
	PeriodKey      string `json:"period_key"`
	UsedUnits      int64  `json:"used_units"`
	LimitUnits     int64  `json:"limit_units"`
	Unlimited      bool   `json:"unlimited"`
	RemainingUnits int64  `json:"remaining_units"`
}

func periodKey(now time.Time) string {
	return now.UTC().Format("2006-01")
}

func (s *Service) GetUsage(ctx context.Context, userID uint64) ([]UsageSnapshot, error) {
	tier, err := s.ActiveTier(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	key := periodKey(now)
	var ledger models.UsageLedger
	err = s.db.WithContext(ctx).
		Where("user_id = ? AND feature = ? AND period_key = ?", userID, "translation_minutes", key).
		Take(&ledger).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	limit := translationFreeMinutesPerMonth
	unlimited := tier == models.EntitlementPremium
	if unlimited {
		limit = 0
	}
	remaining := limit - ledger.Units
	if remaining < 0 {
		remaining = 0
	}
	return []UsageSnapshot{
		{
			Feature:        "translation_minutes",
			PeriodKey:      key,
			UsedUnits:      ledger.Units,
			LimitUnits:     limit,
			Unlimited:      unlimited,
			RemainingUnits: remaining,
		},
	}, nil
}

func (s *Service) ConsumeTranslationMinutes(ctx context.Context, userID uint64, delta int64) error {
	if delta <= 0 {
		return nil
	}
	tier, err := s.ActiveTier(ctx, userID)
	if err != nil {
		return err
	}
	if tier == models.EntitlementPremium {
		return nil
	}

	key := periodKey(time.Now().UTC())
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ledger models.UsageLedger
		err := tx.Where("user_id = ? AND feature = ? AND period_key = ?", userID, "translation_minutes", key).
			First(&ledger).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ledger = models.UsageLedger{
				UserID:    userID,
				Feature:   "translation_minutes",
				PeriodKey: key,
				Units:     0,
			}
			if err := tx.Create(&ledger).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		if ledger.Units+delta > translationFreeMinutesPerMonth {
			return ErrTranslationQuotaExhausted
		}
		ledger.Units += delta
		return tx.Save(&ledger).Error
	})
}

func (s *Service) RegisterCallInvite(ctx context.Context, callID string, caller *models.User, callee *models.User) error {
	now := time.Now().UTC()
	record := &models.CallSession{
		CallID:            callID,
		CallerID:          caller.ID,
		CalleeID:          callee.ID,
		CallerEmail:       caller.Email,
		CalleeEmail:       callee.Email,
		CallerDisplayName: caller.DisplayName,
		CalleeDisplayName: callee.DisplayName,
		Status:            models.CallStatusInvited,
		StartedAt:         now,
		LastEventAt:       now,
	}

	return s.db.WithContext(ctx).
		Where("call_id = ?", callID).
		Assign(record).
		FirstOrCreate(record).Error
}

func (s *Service) UpdateCallStatus(ctx context.Context, callID string, status string, endReason string) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"status":        status,
		"last_event_at": now,
		"updated_at":    now,
	}
	if endReason != "" {
		updates["end_reason"] = endReason
	}
	switch status {
	case models.CallStatusAnswered:
		updates["answered_at"] = now
	case models.CallStatusEnded, models.CallStatusRejected, models.CallStatusMissed, models.CallStatusFailed:
		updates["ended_at"] = now
	}
	return s.db.WithContext(ctx).Model(&models.CallSession{}).Where("call_id = ?", callID).Updates(updates).Error
}

func (s *Service) ListCallHistory(ctx context.Context, userID uint64, days int) ([]models.CallSession, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	var sessions []models.CallSession
	err := s.db.WithContext(ctx).
		Where("(caller_id = ? OR callee_id = ?) AND started_at >= ?", userID, userID, since).
		Order("started_at DESC").
		Limit(100).
		Find(&sessions).Error
	return sessions, err
}

func (s *Service) CreateBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	block := &models.UserBlock{
		BlockerID:     blockerID,
		BlockedUserID: blockedUserID,
	}
	return s.db.WithContext(ctx).Where("blocker_id = ? AND blocked_user_id = ?", blockerID, blockedUserID).
		FirstOrCreate(block).Error
}

func (s *Service) RemoveBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	return s.db.WithContext(ctx).
		Where("blocker_id = ? AND blocked_user_id = ?", blockerID, blockedUserID).
		Delete(&models.UserBlock{}).Error
}

func (s *Service) ListBlocks(ctx context.Context, blockerID uint64) ([]models.UserBlock, error) {
	var blocks []models.UserBlock
	if err := s.db.WithContext(ctx).Where("blocker_id = ?", blockerID).Find(&blocks).Error; err != nil {
		return nil, err
	}
	return blocks, nil
}

func (s *Service) AreUsersBlocked(ctx context.Context, userA, userB uint64) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&models.UserBlock{}).
		Where("(blocker_id = ? AND blocked_user_id = ?) OR (blocker_id = ? AND blocked_user_id = ?)", userA, userB, userB, userA).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) CreateReport(ctx context.Context, reporterID, reportedUserID uint64, category, details string) error {
	report := &models.AbuseReport{
		ReporterID:     reporterID,
		ReportedUserID: reportedUserID,
		Category:       strings.TrimSpace(category),
		Details:        strings.TrimSpace(details),
		Status:         "open",
	}
	return s.db.WithContext(ctx).Create(report).Error
}

type RevenueCatWebhook struct {
	Event struct {
		ID                 string `json:"id"`
		Type               string `json:"type"`
		AppUserID          string `json:"app_user_id"`
		ProductID          string `json:"product_id"`
		EntitlementIDs     []string `json:"entitlement_ids"`
		ExpirationAtMillis int64  `json:"expiration_at_ms"`
	} `json:"event"`
}

func (s *Service) HandleRevenueCatWebhook(ctx context.Context, payload RevenueCatWebhook, raw []byte) error {
	eventID := strings.TrimSpace(payload.Event.ID)
	if eventID == "" {
		return errors.New("billing event id is required")
	}

	now := time.Now().UTC()
	eventRecord := &models.BillingWebhookEvent{
		EventID:     eventID,
		AppUserID:   payload.Event.AppUserID,
		EventType:   payload.Event.Type,
		PayloadJSON: string(raw),
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("event_id = ?", eventID).Take(&models.BillingWebhookEvent{}).Error; err == nil {
			return ErrWebhookAlreadyProcessed
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		if err := tx.Create(eventRecord).Error; err != nil {
			return err
		}

		userID, parseErr := parseAppUserID(payload.Event.AppUserID)
		if parseErr != nil {
			return parseErr
		}

		entitlementTier := models.EntitlementFree
		entitlementName := models.EntitlementFree
		status := "inactive"
		source := revenueCatSource
		var expiresAt *time.Time

		switch payload.Event.Type {
		case "INITIAL_PURCHASE", "RENEWAL", "UNCANCELLATION", "PRODUCT_CHANGE":
			entitlementTier = models.EntitlementPremium
			entitlementName = models.EntitlementPremium
			status = "active"
		case "CANCELLATION", "EXPIRATION", "BILLING_ISSUE", "TRANSFER":
			entitlementTier = models.EntitlementPremium
			entitlementName = models.EntitlementPremium
			status = "inactive"
		default:
			// Unknown events still get stored for support, but do not mutate entitlements.
		}

		if payload.Event.ExpirationAtMillis > 0 {
			t := time.UnixMilli(payload.Event.ExpirationAtMillis).UTC()
			expiresAt = &t
		}

		if entitlementName != models.EntitlementFree {
			var existing models.UserEntitlement
			err := tx.Where("user_id = ? AND entitlement = ?", userID, entitlementName).Take(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				existing = models.UserEntitlement{
					UserID:      userID,
					Entitlement: entitlementName,
				}
			} else if err != nil {
				return err
			}
			existing.Tier = entitlementTier
			existing.ProductID = payload.Event.ProductID
			existing.Source = source
			existing.Status = status
			existing.ExpiresAt = expiresAt
			existing.LastSyncedAt = &now
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
		}

		eventRecord.ProcessedAt = &now
		return tx.Save(eventRecord).Error
	})
}

func parseAppUserID(value string) (uint64, error) {
	var userID uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "user:%d", &userID); err == nil && userID > 0 {
		return userID, nil
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &userID); err == nil && userID > 0 {
		return userID, nil
	}
	return 0, errors.New("invalid app_user_id")
}

func (s *Service) DeleteAccount(ctx context.Context, userID uint64) (*models.DeletionAudit, error) {
	audit := &models.DeletionAudit{
		UserID:    userID,
		DeletedAt: time.Now().UTC(),
	}

	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		deleteCount := func(model any, clause string, args ...any) (int64, error) {
			result := tx.Where(clause, args...).Delete(model)
			return result.RowsAffected, result.Error
		}

		var err error
		if audit.ContactsDeleted, err = deleteCount(&models.Contact{}, "owner_id = ? OR contact_id = ?", userID, userID); err != nil {
			return err
		}
		if audit.CallsDeleted, err = deleteCount(&models.CallSession{}, "caller_id = ? OR callee_id = ?", userID, userID); err != nil {
			return err
		}
		if audit.BlocksDeleted, err = deleteCount(&models.UserBlock{}, "blocker_id = ? OR blocked_user_id = ?", userID, userID); err != nil {
			return err
		}
		if audit.ReportsDeleted, err = deleteCount(&models.AbuseReport{}, "reporter_id = ? OR reported_user_id = ?", userID, userID); err != nil {
			return err
		}
		if audit.LegalRecordsDeleted, err = deleteCount(&models.LegalAcceptance{}, "user_id = ?", userID); err != nil {
			return err
		}
		if audit.UsageRowsDeleted, err = deleteCount(&models.UsageLedger{}, "user_id = ?", userID); err != nil {
			return err
		}
		if audit.EntitlementsDeleted, err = deleteCount(&models.UserEntitlement{}, "user_id = ?", userID); err != nil {
			return err
		}

		if _, err = deleteCount(&models.EmailVerificationCode{}, "email IN (?)", tx.Model(&models.User{}).Select("email").Where("id = ?", userID)); err != nil {
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
		if err := tx.Model(&models.User{}).Where("id = ?", userID).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Create(audit).Error
	})

	if err != nil {
		return nil, err
	}
	return audit, nil
}

func EncodePayload(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}
