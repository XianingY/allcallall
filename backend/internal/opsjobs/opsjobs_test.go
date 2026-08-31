package opsjobs

import (
	"context"
	"os"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/models"
)

func newOpsDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:opsjobs_test?mode=memory&cache=shared"),
		&gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.OrganizationPlan{}, &models.OrganizationUsageLedger{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func seedUsage(t *testing.T, db *gorm.DB, org uint64, period string) {
	t.Helper()
	if err := db.Create(&models.OrganizationUsageLedger{
		OrganizationID: org,
		Feature:        "translation",
		PeriodKey:      period,
		Units:          10,
	}).Error; err != nil {
		t.Fatalf("seed usage: %v", err)
	}
}

func TestGrowthRetentionCohort(t *testing.T) {
	db := newOpsDB(t)
	ctx := context.Background()

	seedUsage(t, db, 1, "2026-06")
	seedUsage(t, db, 2, "2026-06")
	seedUsage(t, db, 3, "2026-06")
	seedUsage(t, db, 1, "2026-07")
	seedUsage(t, db, 2, "2026-07")
	seedUsage(t, db, 1, "2026-08")

	a := NewGrowthRetentionAnalyzer(db)

	c, err := a.activeOrgsInPeriod(ctx, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	if c != 3 {
		t.Fatalf("expected 3 active in 2026-06, got %d", c)
	}

	cohort, err := a.RetentionCohort(ctx, "2026-06", 2)
	if err != nil {
		t.Fatal(err)
	}
	if cohort.CohortSize != 3 {
		t.Fatalf("cohort size 3, got %d", cohort.CohortSize)
	}
	if cohort.FollowPeriods[0].RetainedOrgs != 2 {
		t.Fatalf("expected 2 retained in 2026-07, got %d", cohort.FollowPeriods[0].RetainedOrgs)
	}
	if cohort.FollowPeriods[0].RetentionPct < 66 || cohort.FollowPeriods[0].RetentionPct > 67 {
		t.Fatalf("expected ~66.7%%, got %f", cohort.FollowPeriods[0].RetentionPct)
	}
	if cohort.FollowPeriods[1].RetainedOrgs != 1 {
		t.Fatalf("expected 1 retained in 2026-08, got %d", cohort.FollowPeriods[1].RetainedOrgs)
	}
}

func TestChurnRate(t *testing.T) {
	db := newOpsDB(t)
	ctx := context.Background()

	baseStart := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	baseEnd := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range []uint64{1, 2, 3} {
		if err := db.Create(&models.OrganizationPlan{
			OrganizationID:     id,
			Status:             "active",
			CurrentPeriodStart: baseStart,
			CurrentPeriodEnd:   baseEnd,
		}).Error; err != nil {
			t.Fatalf("seed plan: %v", err)
		}
	}
	// org 99 churned within the June window
	if err := db.Create(&models.OrganizationPlan{
		OrganizationID:     99,
		Status:             "canceled",
		CurrentPeriodStart: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		CurrentPeriodEnd:   time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC),
	}).Error; err != nil {
		t.Fatalf("seed churned plan: %v", err)
	}

	cr, err := NewGrowthRetentionAnalyzer(db).ChurnRate(ctx, "2026-06")
	if err != nil {
		t.Fatal(err)
	}
	if cr.ActiveStart != 3 {
		t.Fatalf("expected 3 active at start, got %d", cr.ActiveStart)
	}
	if cr.Churned != 1 {
		t.Fatalf("expected 1 churned, got %d", cr.Churned)
	}
	if cr.ChurnRatePct < 33 || cr.ChurnRatePct > 34 {
		t.Fatalf("expected ~33.3%%, got %f", cr.ChurnRatePct)
	}
}

func TestQuarterlyPentestPlan(t *testing.T) {
	plan := BuildQuarterlyPlan(time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC))
	if plan.Quarter != "2026-Q1" {
		t.Fatalf("expected 2026-Q1, got %s", plan.Quarter)
	}
	if len(plan.Scope) == 0 {
		t.Fatal("expected non-empty scope")
	}
	p0 := 0
	for _, it := range plan.Scope {
		if it.Priority == "P0" {
			p0++
		}
	}
	if p0 < 2 {
		t.Fatalf("expected at least 2 P0 items, got %d", p0)
	}
}

func TestAnnualAuditReflectsEnv(t *testing.T) {
	os.Setenv("AIGC_LABELING_ENABLED", "true")
	os.Setenv("ICP_LICENSE_NUMBER", "沪ICP备12345678号-1")
	os.Setenv("AI_ALGORITHM_FILING_NUMBER", "网信算备XXXXXXXXXXXXXXXX号")
	defer func() {
		os.Unsetenv("AIGC_LABELING_ENABLED")
		os.Unsetenv("ICP_LICENSE_NUMBER")
		os.Unsetenv("AI_ALGORITHM_FILING_NUMBER")
	}()

	audit := RunAnnualAudit()
	if audit.GeneratedAt.IsZero() {
		t.Fatal("generated_at must be set")
	}
	if len(audit.Items) == 0 {
		t.Fatal("expected audit items")
	}

	var foundAIGC, foundICP bool
	for _, it := range audit.Items {
		if it.ID == "aigc_labeling" && it.Status == "ok" {
			foundAIGC = true
		}
		if it.ID == "icp_license" && it.Status == "ok" {
			foundICP = true
		}
	}
	if !foundAIGC {
		t.Fatal("expected AIGC labeling control ok")
	}
	if !foundICP {
		t.Fatal("expected ICP control ok")
	}
}
