package commerce

import (
	"context"

	"github.com/allcallall/backend/internal/models"
)

// UsageStatsService exposes multi-dimensional organization usage analytics.
type UsageStatsService struct {
	repo *OrgRepository
}

// NewUsageStatsService builds a UsageStatsService.
func NewUsageStatsService(repo *OrgRepository) *UsageStatsService {
	return &UsageStatsService{repo: repo}
}

// FeatureBreakdown is a per-feature usage row for an organization.
type FeatureBreakdown struct {
	Feature    string `json:"feature"`
	UsedUnits  int64  `json:"used_units"`
	LimitUnits int64  `json:"limit_units"`
	Members    int    `json:"members"`
}

// OrganizationBreakdown returns per-feature usage plus member count for an org
// in the given period (defaults to the current month).
func (s *UsageStatsService) OrganizationBreakdown(ctx context.Context, orgID uint64, periodKey string) ([]FeatureBreakdown, int, error) {
	if periodKey == "" {
		periodKey = periodKeyNow()
	}
	var ledgers []models.OrganizationUsageLedger
	if err := s.repo.db.WithContext(ctx).
		Where("organization_id = ? AND period_key = ?", orgID, periodKey).
		Find(&ledgers).Error; err != nil {
		return nil, 0, err
	}
	members, err := s.repo.ListOrganizationMemberUserIDs(ctx, orgID)
	if err != nil {
		return nil, 0, err
	}
	out := make([]FeatureBreakdown, 0, len(ledgers))
	for _, l := range ledgers {
		out = append(out, FeatureBreakdown{
			Feature:    l.Feature,
			UsedUnits:  l.Units,
			LimitUnits: l.LimitUnits,
			Members:    len(members),
		})
	}
	return out, len(members), nil
}

// TopUserRow is a single user ranking row.
type TopUserRow struct {
	UserID    uint64 `json:"user_id"`
	Feature   string `json:"feature"`
	UsedUnits int64  `json:"used_units"`
}

// TopUsersByFeature ranks the highest-usage members of an org for a feature.
func (s *UsageStatsService) TopUsersByFeature(ctx context.Context, orgID uint64, feature, periodKey string, limit int) ([]TopUserRow, error) {
	if periodKey == "" {
		periodKey = periodKeyNow()
	}
	members, err := s.repo.ListOrganizationMemberUserIDs(ctx, orgID)
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.TopUserUsage(ctx, members, feature, periodKey, limit)
	if err != nil {
		return nil, err
	}
	out := make([]TopUserRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, TopUserRow{UserID: r.UserID, Feature: r.Feature, UsedUnits: r.Units})
	}
	return out, nil
}

// PeriodPoint is one point on a trend line.
type PeriodPoint struct {
	PeriodKey string `json:"period_key"`
	UsedUnits int64  `json:"used_units"`
}

// PeriodTrend returns org usage of a feature across the supplied periods
// (e.g. the last 6 month keys), enabling trend dashboards.
func (s *UsageStatsService) PeriodTrend(ctx context.Context, orgID uint64, feature string, periods []string) ([]PeriodPoint, error) {
	out := make([]PeriodPoint, 0, len(periods))
	for _, pk := range periods {
		point := PeriodPoint{PeriodKey: pk}
		ledger, err := s.repo.GetOrgUsageLedger(ctx, orgID, feature, pk)
		if err == nil {
			point.UsedUnits = ledger.Units
		}
		out = append(out, point)
	}
	return out, nil
}
