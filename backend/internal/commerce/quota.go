package commerce

import (
	"context"
	"strconv"

	"github.com/allcallall/backend/internal/alerting"
	"github.com/allcallall/backend/internal/models"
)

// premiumGatedFeatures require a premium (paid) user or org plan to use.
var premiumGatedFeatures = map[string]bool{
	"advanced_analytics":  true,
	"custom_integrations": true,
}

// QuotaService unifies organization plan quotas with per-user entitlements.
type QuotaService struct {
	repo        *OrgRepository
	entitlement *EntitlementService
	alerter     *alerting.Service
}

// NewQuotaService builds a QuotaService.
func NewQuotaService(repo *OrgRepository, entitlement *EntitlementService) *QuotaService {
	return &QuotaService{repo: repo, entitlement: entitlement}
}

// WithAlerter 接入告警服务。配额熔断（org_quota_exceeded）时按 P2 上报，
// 由 alerting 的去重窗口抑制同一租户/功能的重复告警，避免请求级刷屏。
// WithAlerter wires an alerting service so quota breaches are observable.
func (s *QuotaService) WithAlerter(svc *alerting.Service) *QuotaService {
	s.alerter = svc
	return s
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
						decision := AccessDecision{Allowed: false, Reason: "org_quota_exceeded"}
						s.emitQuotaBreach(orgID, userID, feature, policy.LimitUnits, ledger.Units)
						return decision, nil
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

// emitQuotaBreach 在租户配额熔断时按 P2 上报。alerting 的去重窗口会抑制同一
// 租户/功能在短时间内重复触发的告警，避免请求级刷屏。未配置 alerter 时静默。
// emitQuotaBreach notifies on-call when a tenant's quota circuit-breaker trips.
func (s *QuotaService) emitQuotaBreach(orgID, userID uint64, feature string, limit, used int64) {
	if s.alerter == nil {
		return
	}
	if err := s.alerter.Emit(context.Background(), alerting.Alert{
		Severity: alerting.SeverityP2,
		Title:    "tenant quota exceeded",
		Detail:   "feature " + feature + " blocked: usage " + strconv.FormatInt(used, 10) + " >= limit " + strconv.FormatInt(limit, 10),
		Labels: map[string]string{
			"component": "quota",
			"org_id":    strconv.FormatUint(orgID, 10),
			"user_id":   strconv.FormatUint(userID, 10),
			"feature":   feature,
		},
	}); err != nil {
		// 告警失败不应阻断主流程；Emit 已内部记录。
		_ = err
	}
}
