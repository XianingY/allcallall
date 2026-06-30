package commerce

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

const (
	translationFreeSecondsPerMonth = int64(1800)
	translationSliceSeconds        = int64(30)
	translationSliceMilliseconds   = translationSliceSeconds * 1000

	translationUsageFeature = "translation_seconds"
)

// UsageSnapshot represents a point-in-time view of feature usage against quotas.
type UsageSnapshot struct {
	Feature        string `json:"feature"`
	PeriodKey      string `json:"period_key"`
	Unit           string `json:"unit"`
	UsedUnits      int64  `json:"used_units"`
	LimitUnits     int64  `json:"limit_units"`
	Unlimited      bool   `json:"unlimited"`
	RemainingUnits int64  `json:"remaining_units"`
}

// EntitlementService handles entitlement checks, tier resolution, and usage tracking.
type EntitlementService struct {
	repo    *Repository
	metrics metrics.Recorder
}

// NewEntitlementService creates a new EntitlementService.
func NewEntitlementService(repo *Repository, metrics metrics.Recorder) *EntitlementService {
	return &EntitlementService{repo: repo, metrics: metrics}
}

// EnsureDefaultEntitlement returns an existing entitlement or creates a free-tier default.
func (s *EntitlementService) EnsureDefaultEntitlement(ctx context.Context, userID uint64) (*models.UserEntitlement, error) {
	now := time.Now().UTC()
	entitlement, err := s.repo.GetActivePremiumEntitlement(ctx, userID)
	if err == nil {
		return entitlement, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	entitlement = &models.UserEntitlement{
		UserID:       userID,
		Entitlement:  models.EntitlementFree,
		Tier:         models.EntitlementFree,
		Source:       "system",
		Status:       "active",
		LastSyncedAt: &now,
	}

	if err := s.repo.FirstOrCreateFreeEntitlement(ctx, entitlement); err != nil {
		return nil, err
	}
	return entitlement, nil
}

// GetEntitlements returns all entitlements for a user, ensuring a default exists.
func (s *EntitlementService) GetEntitlements(ctx context.Context, userID uint64) ([]models.UserEntitlement, error) {
	entitlements, err := s.repo.GetEntitlements(ctx, userID)
	if err != nil {
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

// ActiveTier returns the effective entitlement tier for a user.
func (s *EntitlementService) ActiveTier(ctx context.Context, userID uint64) (string, error) {
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

func periodKey(now time.Time) string {
	return now.UTC().Format("2006-01")
}

func (s *EntitlementService) lookupUsageLedgerUnits(ctx context.Context, userID uint64, key string) (int64, error) {
	ledger, err := s.repo.GetUsageLedger(ctx, userID, translationUsageFeature, key)
	if err == nil {
		return ledger.Units, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	legacy, legacyErr := s.repo.GetUsageLedger(ctx, userID, "translation_minutes", key)
	if legacyErr == nil {
		return legacy.Units * 60, nil
	}
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return 0, legacyErr
	}
	return 0, nil
}

// GetUsage returns translation usage snapshots for a user.
func (s *EntitlementService) GetUsage(ctx context.Context, userID uint64) ([]UsageSnapshot, error) {
	tier, err := s.ActiveTier(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	key := periodKey(now)
	usedUnits, err := s.lookupUsageLedgerUnits(ctx, userID, key)
	if err != nil {
		return nil, err
	}

	limit := translationFreeSecondsPerMonth
	unlimited := tier == models.EntitlementPremium
	if unlimited {
		limit = 0
	}
	remaining := limit - usedUnits
	if remaining < 0 {
		remaining = 0
	}
	return []UsageSnapshot{
		{
			Feature:        translationUsageFeature,
			PeriodKey:      key,
			Unit:           "seconds",
			UsedUnits:      usedUnits,
			LimitUnits:     limit,
			Unlimited:      unlimited,
			RemainingUnits: remaining,
		},
	}, nil
}

func (s *EntitlementService) consumeTranslationSecondsTx(ctx context.Context, userID uint64, deltaSeconds int64, key string) error {
	if deltaSeconds <= 0 {
		return nil
	}

	ledger, err := s.repo.GetUsageLedger(ctx, userID, translationUsageFeature, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		legacyUnits := int64(0)
		legacy, legacyErr := s.repo.GetUsageLedger(ctx, userID, "translation_minutes", key)
		if legacyErr == nil {
			legacyUnits = legacy.Units * 60
		} else if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
			return legacyErr
		}
		ledger = &models.UsageLedger{
			UserID:    userID,
			Feature:   translationUsageFeature,
			PeriodKey: key,
			Units:     legacyUnits,
		}
		if err := s.repo.FirstOrCreateUsageLedger(ctx, ledger); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if ledger.Units+deltaSeconds > translationFreeSecondsPerMonth {
		return ErrTranslationQuotaExhausted
	}
	ledger.Units += deltaSeconds
	return s.repo.SaveUsageLedger(ctx, ledger)
}

// ConsumeTranslationMinutes records usage in minutes (converted to seconds).
func (s *EntitlementService) ConsumeTranslationMinutes(ctx context.Context, userID uint64, delta int64) error {
	return s.ConsumeTranslationSeconds(ctx, userID, delta*60)
}

// ConsumeTranslationSeconds deducts translation seconds from the free-tier quota.
// Premium users are not charged.
func (s *EntitlementService) ConsumeTranslationSeconds(ctx context.Context, userID uint64, deltaSeconds int64) error {
	if deltaSeconds <= 0 {
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
	return s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		return s.consumeTranslationSecondsTx(ctx, userID, deltaSeconds, key)
	})
}

// RecordTranslationUsageSlice records a 30-second translation usage slice for a call.
// Returns true if the slice was charged (first occurrence).
func (s *EntitlementService) RecordTranslationUsageSlice(ctx context.Context, userID uint64, callID string, eventTimestampMS int64) (bool, error) {
	if userID == 0 || strings.TrimSpace(callID) == "" {
		return false, errors.New("user_id and call_id are required")
	}
	if eventTimestampMS <= 0 {
		eventTimestampMS = time.Now().UnixMilli()
	}

	tier, err := s.ActiveTier(ctx, userID)
	if err != nil {
		return false, err
	}

	callID = strings.TrimSpace(callID)
	sliceIndex := eventTimestampMS / translationSliceMilliseconds
	key := periodKey(time.UnixMilli(eventTimestampMS).UTC())
	charged := false

	err = s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		slice := &models.TranslationUsageSlice{
			UserID:           userID,
			CallID:           callID,
			SliceIndex:       sliceIndex,
			EventTimestampMS: eventTimestampMS,
			DurationSeconds:  translationSliceSeconds,
			Tier:             tier,
		}

		rowsAffected, createErr := s.repo.FirstOrCreateTranslationUsageSlice(ctx, slice)
		if createErr != nil {
			return createErr
		}
		if rowsAffected == 0 {
			return nil
		}

		charged = true
		if tier == models.EntitlementPremium {
			return nil
		}

		return s.consumeTranslationSecondsTx(ctx, userID, translationSliceSeconds, key)
	})
	if err != nil {
		return false, err
	}
	return charged, nil
}
