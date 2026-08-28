// Package opsjobs implements commercial-operations jobs that run on a schedule:
// growth & retention analytics, annual compliance self-audit, and the quarterly
// penetration-test plan generator. Every job is environment/DB driven and
// decoupled from the central config — they are invoked by a cron worker or the
// opsaudit CLI (cmd/opsaudit), never in the request path.
package opsjobs

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// GrowthRetentionAnalyzer computes commercial growth & retention metrics from
// the organization plan and usage-ledger tables populated by Phase 2 billing.
type GrowthRetentionAnalyzer struct {
	db *gorm.DB
}

// NewGrowthRetentionAnalyzer builds the analyzer.
func NewGrowthRetentionAnalyzer(db *gorm.DB) *GrowthRetentionAnalyzer {
	return &GrowthRetentionAnalyzer{db: db}
}

// TrendPoint is a single period's active-organization count.
type TrendPoint struct {
	PeriodKey  string `json:"period_key"`
	ActiveOrgs int64  `json:"active_orgs"`
}

// RetentionCell is one follow-up period in a retention cohort.
type RetentionCell struct {
	PeriodKey    string  `json:"period_key"`
	RetainedOrgs int64   `json:"retained_orgs"`
	RetentionPct float64 `json:"retention_pct"`
}

// RetentionCohort tracks a cohort of organizations first active in CohortPeriod
// and how many remained active in subsequent periods.
type RetentionCohort struct {
	CohortPeriod  string          `json:"cohort_period"`
	CohortSize    int64           `json:"cohort_size"`
	FollowPeriods []RetentionCell `json:"follow_periods"`
}

// ChurnResult is the churn for a single billing period.
type ChurnResult struct {
	PeriodKey    string  `json:"period_key"`
	ActiveStart  int64   `json:"active_start"`
	Churned      int64   `json:"churned"`
	ChurnRatePct float64 `json:"churn_rate_pct"`
}

// GrowthReport aggregates the analytics snapshot for a review window.
type GrowthReport struct {
	GeneratedAt   time.Time         `json:"generated_at"`
	WindowPeriods int               `json:"window_periods"`
	MonthlyActive []TrendPoint      `json:"monthly_active"`
	LatestActive  int64             `json:"latest_active"`
	ActiveDelta   int64             `json:"active_delta"`
	Retention     []RetentionCohort `json:"retention"`
	Churn         []ChurnResult     `json:"churn"`
}

// monthlyPeriods returns the last n monthly period keys (YYYY-MM), oldest first.
func monthlyPeriods(n int) []string {
	if n <= 0 {
		n = 6
	}
	out := make([]string, 0, n)
	now := time.Now().UTC()
	for i := n - 1; i >= 0; i-- {
		t := now.AddDate(0, -i, 0)
		out = append(out, t.Format("2006-01"))
	}
	return out
}

// activeOrgsInPeriod counts distinct organizations with any usage in a period.
func (a *GrowthRetentionAnalyzer) activeOrgsInPeriod(ctx context.Context, periodKey string) (int64, error) {
	var count int64
	err := a.db.WithContext(ctx).
		Model(&models.OrganizationUsageLedger{}).
		Where("period_key = ?", periodKey).
		Distinct("organization_id").
		Count(&count).Error
	return count, err
}

// activeOrgSet returns the set of organization IDs active in a period.
func (a *GrowthRetentionAnalyzer) activeOrgSet(ctx context.Context, periodKey string) (map[uint64]struct{}, error) {
	var ids []uint64
	err := a.db.WithContext(ctx).
		Model(&models.OrganizationUsageLedger{}).
		Where("period_key = ?", periodKey).
		Distinct("organization_id").
		Pluck("organization_id", &ids).Error
	if err != nil {
		return nil, err
	}
	set := make(map[uint64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	return set, nil
}

// MonthlyActiveTrend returns active-org counts for the last n periods.
func (a *GrowthRetentionAnalyzer) MonthlyActiveTrend(ctx context.Context, n int) ([]TrendPoint, error) {
	periods := monthlyPeriods(n)
	out := make([]TrendPoint, 0, len(periods))
	for _, p := range periods {
		c, err := a.activeOrgsInPeriod(ctx, p)
		if err != nil {
			return nil, err
		}
		out = append(out, TrendPoint{PeriodKey: p, ActiveOrgs: c})
	}
	return out, nil
}

// RetentionCohort builds a cohort from cohortPeriod and tracks follow-up periods.
func (a *GrowthRetentionAnalyzer) RetentionCohort(ctx context.Context, cohortPeriod string, followPeriods int) (RetentionCohort, error) {
	cohort, err := a.activeOrgSet(ctx, cohortPeriod)
	if err != nil {
		return RetentionCohort{}, err
	}
	// derive follow-up monthly periods after the cohort month
	cohortTime, perr := time.Parse("2006-01", cohortPeriod)
	if perr != nil {
		return RetentionCohort{}, fmt.Errorf("invalid cohort period %q: %w", cohortPeriod, perr)
	}
	cells := make([]RetentionCell, 0, followPeriods)
	for i := 1; i <= followPeriods; i++ {
		fp := cohortTime.AddDate(0, i, 0).Format("2006-01")
		fset, err := a.activeOrgSet(ctx, fp)
		if err != nil {
			return RetentionCohort{}, err
		}
		var retained int64
		for id := range cohort {
			if _, ok := fset[id]; ok {
				retained++
			}
		}
		pct := 0.0
		if len(cohort) > 0 {
			pct = float64(retained) / float64(len(cohort)) * 100
		}
		cells = append(cells, RetentionCell{
			PeriodKey:    fp,
			RetainedOrgs: retained,
			RetentionPct: pct,
		})
	}
	return RetentionCohort{
		CohortPeriod:  cohortPeriod,
		CohortSize:    int64(len(cohort)),
		FollowPeriods: cells,
	}, nil
}

// ChurnRate approximates churn for a period: organizations whose plan is no
// longer active (canceled/past_due) with a period-end inside the window, over
// the active base at period start. This is a point-in-time proxy (no history
// table); document it when reporting.
func (a *GrowthRetentionAnalyzer) ChurnRate(ctx context.Context, periodKey string) (ChurnResult, error) {
	pt, err := time.Parse("2006-01", periodKey)
	if err != nil {
		return ChurnResult{}, fmt.Errorf("invalid period %q: %w", periodKey, err)
	}
	periodStart := time.Date(pt.Year(), pt.Month(), 1, 0, 0, 0, 0, time.UTC)
	periodEnd := periodStart.AddDate(0, 1, 0)

	var activeStart int64
	if err := a.db.WithContext(ctx).
		Model(&models.OrganizationPlan{}).
		Where("status = ?", "active").
		Count(&activeStart).Error; err != nil {
		return ChurnResult{}, err
	}

	var churned int64
	if err := a.db.WithContext(ctx).
		Model(&models.OrganizationPlan{}).
		Where("status IN ? AND current_period_end >= ? AND current_period_end < ?",
			[]string{"canceled", "past_due"}, periodStart, periodEnd).
		Count(&churned).Error; err != nil {
		return ChurnResult{}, err
	}

	pct := 0.0
	if activeStart > 0 {
		pct = float64(churned) / float64(activeStart) * 100
	}
	return ChurnResult{
		PeriodKey:    periodKey,
		ActiveStart:  activeStart,
		Churned:      churned,
		ChurnRatePct: pct,
	}, nil
}

// Report builds a full growth & retention snapshot over the last n periods.
func (a *GrowthRetentionAnalyzer) Report(ctx context.Context, n int) (GrowthReport, error) {
	if n <= 0 {
		n = 6
	}
	trend, err := a.MonthlyActiveTrend(ctx, n)
	if err != nil {
		return GrowthReport{}, err
	}
	latest := int64(0)
	prev := int64(0)
	if len(trend) > 0 {
		latest = trend[len(trend)-1].ActiveOrgs
	}
	if len(trend) > 1 {
		prev = trend[len(trend)-2].ActiveOrgs
	}

	retention := make([]RetentionCohort, 0, 3)
	// build cohorts for the 3 most recent completed months
	for i := n - 3; i < n-1; i++ {
		if i < 0 {
			continue
		}
		cohort, err := a.RetentionCohort(ctx, trend[i].PeriodKey, n-1-i)
		if err != nil {
			return GrowthReport{}, err
		}
		retention = append(retention, cohort)
	}

	churn := make([]ChurnResult, 0, n)
	for _, tp := range trend {
		cr, err := a.ChurnRate(ctx, tp.PeriodKey)
		if err != nil {
			return GrowthReport{}, err
		}
		churn = append(churn, cr)
	}

	return GrowthReport{
		GeneratedAt:   time.Now().UTC(),
		WindowPeriods: n,
		MonthlyActive: trend,
		LatestActive:  latest,
		ActiveDelta:   latest - prev,
		Retention:     retention,
		Churn:         churn,
	}, nil
}
