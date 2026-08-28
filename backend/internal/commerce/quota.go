package commerce

import (
	"context"

	"github.com/allcallall/backend/internal/models"
)

// premiumGatedFeatures require a premium (paid) user or org plan to use.
var premiumGatedFeatures = map[string]bool{
	"advanced_analytics": true,
	"custom_integrations": true,
}

// QuotaService unifies organization plan quotas with per-user entitlements.
type QuotaService struct {
	repo        *OrgRepository
	entitlement *EntitlementService
}

// NewQuotaService builds a QuotaService.
func NewQuotaService(repo *OrgRepository, entitlement *EntitlementService) *QuotaService {
	return &QuotaService{repo: repo, entitlement: entitlement}
}

// AccessDecision describes whether a feature may be used.
type AccessDecision struct {
	Allowed bool
	Reason  string // empty when allowed
}

// CheckFeatureAccess evaluates org-plan quota and user entitlement together.
func (s *QuotaService) CheckFeatureAccess(ctx context.Context, orgID, userID uint64, feature string) (AccessDecision, error) {
	// 1) User entitlement gate (premium-only features).
	if premiumGatedFeatures[feature] && s.entitlement != nil && userID != 0 {
		tier, err := s.entitlement.ActiveTier(ctx, userID)
		if err == nil && tier != models.EntitlementPremium {
			// Org plan may still grant access even if the individual user is free.
			if !s.orgGrantsFeature(ctx, orgID, feature) {
				return AccessDecision{Allowed: false, Reason: "premium_required"}, nil
			}
		}
	}

	// 2) Organization plan quota gate.
	if orgID != 0 {
		plan, err := s.repo.GetOrganizationPlan(ctx, orgID)
		if err == nil && plan != nil {
			policy, perr := s.repo.GetQuotaPolicy(ctx, plan.PlanID, feature)
			if perr == nil {
				if policy.Unlimited {
					return AccessDecision{Allowed: true}, nil
				}
				if policy.LimitUnits > 0 {
					periodKey := periodKeyNow()
					ledger, lerr := s.repo.GetOrgUsageLedger(ctx, orgID, feature, periodKey)
					if lerr == nil && ledger.Units >= policy.LimitUnits {
						return AccessDecision{Allowed: false, Reason: "org_quota_exceeded"}, nil
					}
				}
			}
		}
	}
	return AccessDecision{Allowed: true}, nil
}

// orgGrantsFeature reports whether the org plan's quota policy grants a feature.
func (s *QuotaService) orgGrantsFeature(ctx context.Context, orgID uint64, feature string) bool {
	plan, err := s.repo.GetOrganizationPlan(ctx, orgID)
	if err != nil || plan == nil {
		return false
	}
	policy, perr := s.repo.GetQuotaPolicy(ctx, plan.PlanID, feature)
	if perr != nil {
		return false
	}
	return policy.Unlimited || policy.LimitUnits > 0
}

// RecordUsage records consumption at both org and user levels. For the
// translation_seconds feature it also drives the existing entitlement ledger so
// the two accounting systems stay consistent.
func (s *QuotaService) RecordUsage(ctx context.Context, orgID, userID uint64, feature string, delta int64) error {
	if orgID != 0 {
		if err := NewOrgBillingService(s.repo).RecordOrganizationUsage(ctx, orgID, feature, delta); err != nil {
			return err
		}
	}
	if userID != 0 && feature == "translation_seconds" && s.entitlement != nil {
		if err := s.entitlement.ConsumeTranslationSeconds(ctx, userID, delta); err != nil {
			return err
		}
	}
	return nil
}
