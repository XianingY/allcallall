package commerce

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// ErrOrgPlanNotFound indicates no current plan for the organization.
var ErrOrgPlanNotFound = errors.New("commerce: organization plan not found")

// OrgBillingService manages B2B organization plans and org-level usage.
type OrgBillingService struct {
	repo *OrgRepository
}

// NewOrgBillingService builds an OrgBillingService.
func NewOrgBillingService(repo *OrgRepository) *OrgBillingService {
	return &OrgBillingService{repo: repo}
}

func periodKeyNow() string { return time.Now().UTC().Format("2006-01") }

// EnsureOrganizationPlan upserts the org's current plan with its billing window.
func (s *OrgBillingService) EnsureOrganizationPlan(ctx context.Context, orgID uint64, planID, planName, cycle string, seats int, start, end time.Time) (*models.OrganizationPlan, error) {
	if seats < 1 {
		seats = 1
	}
	if cycle == "" {
		cycle = "monthly"
	}
	plan := &models.OrganizationPlan{
		OrganizationID:     orgID,
		PlanID:             planID,
		PlanName:           planName,
		Status:             "active",
		BillingCycle:       cycle,
		CurrentPeriodStart: start,
		CurrentPeriodEnd:   end,
		Seats:              seats,
	}
	if err := s.repo.UpsertOrganizationPlan(ctx, plan); err != nil {
		return nil, err
	}
	return s.repo.GetOrganizationPlan(ctx, orgID)
}

// RecordOrganizationUsage increments org-level usage within a transaction and
// enforces the plan's QuotaPolicy. Unlimited policies are not enforced.
func (s *OrgBillingService) RecordOrganizationUsage(ctx context.Context, orgID uint64, feature string, delta int64) error {
	if delta <= 0 {
		return nil
	}
	periodKey := periodKeyNow()
	return s.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ledger models.OrganizationUsageLedger
		err := tx.Where("organization_id = ? AND feature = ? AND period_key = ?", orgID, feature, periodKey).Take(&ledger).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ledger = models.OrganizationUsageLedger{OrganizationID: orgID, Feature: feature, PeriodKey: periodKey}
			if err := tx.Create(&ledger).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		var plan models.OrganizationPlan
		if err := tx.Where("organization_id = ?", orgID).First(&plan).Error; err == nil {
			var policy models.QuotaPolicy
			if err := tx.Where("plan_id = ? AND feature = ?", plan.PlanID, feature).Take(&policy).Error; err == nil {
				if !policy.Unlimited && policy.LimitUnits > 0 && ledger.Units+delta > policy.LimitUnits {
					return ErrQuotaExceeded
				}
			}
		}

		ledger.Units += delta
		return tx.Save(&ledger).Error
	})
}

// GetOrganizationUsage returns org-level usage snapshots for the current period.
func (s *OrgBillingService) GetOrganizationUsage(ctx context.Context, orgID uint64) ([]UsageSnapshot, error) {
	periodKey := periodKeyNow()
	var ledgers []models.OrganizationUsageLedger
	if err := s.repo.db.WithContext(ctx).
		Where("organization_id = ? AND period_key = ?", orgID, periodKey).
		Find(&ledgers).Error; err != nil {
		return nil, err
	}
	out := make([]UsageSnapshot, 0, len(ledgers))
	for _, l := range ledgers {
		remaining := l.LimitUnits - l.Units
		if remaining < 0 {
			remaining = 0
		}
		out = append(out, UsageSnapshot{
			Feature:        l.Feature,
			PeriodKey:      l.PeriodKey,
			Unit:           "units",
			UsedUnits:      l.Units,
			LimitUnits:     l.LimitUnits,
			Unlimited:      l.LimitUnits == 0,
			RemainingUnits: remaining,
		})
	}
	return out, nil
}
