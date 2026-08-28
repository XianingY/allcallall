package commerce

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func newCommerceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	// AutoMigrate only the tables exercised here; the full AllModels() set
	// includes MySQL-specific column definitions unsupported by the sqlite
	// test driver.
	if err := db.AutoMigrate(
		&models.OrganizationPlan{},
		&models.OrganizationUsageLedger{},
		&models.Invoice{},
		&models.InvoiceLine{},
		&models.QuotaPolicy{},
		&models.OrganizationMember{},
		&models.UserEntitlement{},
		&models.UsageLedger{},
		&models.TranslationUsageSlice{},
	); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestOrgBillingPlanAndQuota(t *testing.T) {
	db := newCommerceDB(t)
	ctx := context.Background()
	repo := NewOrgRepository(db)
	svc := NewOrgBillingService(repo)

	start := time.Now().UTC()
	end := start.AddDate(0, 1, 0)
	plan, err := svc.EnsureOrganizationPlan(ctx, 10, "business", "Business", "monthly", 5, start, end)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PlanID != "business" || plan.Seats != 5 {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	// Quota policy: 100 units for translation_seconds.
	if err := repo.UpsertQuotaPolicy(ctx, &models.QuotaPolicy{PlanID: "business", Feature: "translation_seconds", LimitUnits: 100}); err != nil {
		t.Fatal(err)
	}

	if err := svc.RecordOrganizationUsage(ctx, 10, "translation_seconds", 50); err != nil {
		t.Fatal(err)
	}
	if err := svc.RecordOrganizationUsage(ctx, 10, "translation_seconds", 60); err != ErrQuotaExceeded {
		t.Fatalf("expected ErrQuotaExceeded, got %v", err)
	}

	snaps, err := svc.GetOrganizationUsage(ctx, 10)
	if err != nil || len(snaps) != 1 {
		t.Fatalf("unexpected usage: %v err=%v", snaps, err)
	}
	if snaps[0].UsedUnits != 50 {
		t.Fatalf("expected 50 used, got %d", snaps[0].UsedUnits)
	}
}

func TestInvoiceTaxComputation(t *testing.T) {
	if got := ComputeTax(10000, 60); got != 600 {
		t.Fatalf("ComputeTax(10000,60) = %d, want 600", got)
	}
	if got := ComputeTax(10000, 0); got != 0 {
		t.Fatalf("ComputeTax(10000,0) = %d, want 0", got)
	}

	db := newCommerceDB(t)
	ctx := context.Background()
	repo := NewOrgRepository(db)
	svc := NewInvoiceService(repo)

	start := time.Now().UTC()
	end := start.AddDate(0, 1, 0)
	inv, err := svc.CreateInvoice(ctx, 10, "business", start, end, 60, "CNY", []InvoiceLineInput{
		{Description: "Seat x5", Quantity: 5, UnitMinor: 20000}, // 5 * 200.00 = 1000.00
		{Description: "Overage", Quantity: 1, UnitMinor: 5000},  // 50.00
	})
	if err != nil {
		t.Fatal(err)
	}
	if inv.SubtotalMinor != 105000 {
		t.Fatalf("subtotal = %d, want 105000", inv.SubtotalMinor)
	}
	if inv.TaxMinor != ComputeTax(105000, 60) {
		t.Fatalf("tax = %d, want %d", inv.TaxMinor, ComputeTax(105000, 60))
	}
	if inv.TotalMinor != inv.SubtotalMinor+inv.TaxMinor {
		t.Fatalf("total mismatch")
	}
	if inv.Status != "draft" {
		t.Fatalf("expected draft, got %s", inv.Status)
	}

	issued, err := svc.IssueInvoice(ctx, 10, inv.InvoiceNo)
	if err != nil {
		t.Fatal(err)
	}
	if issued.Status != "issued" || issued.IssuedAt == nil {
		t.Fatalf("issue failed: %+v", issued)
	}
}

func TestQuotaServiceFeatureAccess(t *testing.T) {
	db := newCommerceDB(t)
	ctx := context.Background()
	repo := NewOrgRepository(db)
	ent := NewEntitlementService(&Repository{db: db}, nil)
	qs := NewQuotaService(repo, ent)

	// Free user, premium-gated feature, no org grant -> denied.
	dec, err := qs.CheckFeatureAccess(ctx, 0, 7, "advanced_analytics")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Allowed {
		t.Fatal("expected premium_required denial for free user")
	}

	// Org plan with unlimited quota for the gated feature -> allowed.
	start := time.Now().UTC()
	_, _ = NewOrgBillingService(repo).EnsureOrganizationPlan(ctx, 10, "business", "Business", "monthly", 5, start, start.AddDate(0,1,0))
	_ = repo.UpsertQuotaPolicy(ctx, &models.QuotaPolicy{PlanID: "business", Feature: "advanced_analytics", Unlimited: true})
	dec, err = qs.CheckFeatureAccess(ctx, 10, 7, "advanced_analytics")
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allowed {
		t.Fatal("expected org-granted access for premium feature")
	}
}

func TestUsageStatsBreakdownAndTrend(t *testing.T) {
	db := newCommerceDB(t)
	ctx := context.Background()
	repo := NewOrgRepository(db)
	us := NewUsageStatsService(repo)

	// Org members
	db.Create(&models.OrganizationMember{OrganizationID: 10, UserID: 1})
	db.Create(&models.OrganizationMember{OrganizationID: 10, UserID: 2})

	// Org usage ledgers for two periods
	db.Create(&models.OrganizationUsageLedger{OrganizationID: 10, Feature: "translation_seconds", PeriodKey: "2026-01", Units: 30, LimitUnits: 100})
	db.Create(&models.OrganizationUsageLedger{OrganizationID: 10, Feature: "translation_seconds", PeriodKey: "2026-02", Units: 70, LimitUnits: 100})

	breakdown, members, err := us.OrganizationBreakdown(ctx, 10, "2026-01")
	if err != nil {
		t.Fatal(err)
	}
	if members != 2 || len(breakdown) != 1 || breakdown[0].UsedUnits != 30 {
		t.Fatalf("breakdown wrong: members=%d rows=%+v", members, breakdown)
	}

	trend, err := us.PeriodTrend(ctx, 10, "translation_seconds", []string{"2026-01", "2026-02"})
	if err != nil {
		t.Fatal(err)
	}
	if len(trend) != 2 || trend[0].UsedUnits != 30 || trend[1].UsedUnits != 70 {
		t.Fatalf("trend wrong: %+v", trend)
	}

	// User-level ranking
	db.Create(&models.UsageLedger{UserID: 1, Feature: "translation_seconds", PeriodKey: "2026-01", Units: 20})
	db.Create(&models.UsageLedger{UserID: 2, Feature: "translation_seconds", PeriodKey: "2026-01", Units: 10})
	top, err := us.TopUsersByFeature(ctx, 10, "translation_seconds", "2026-01", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top[0].UserID != 1 || top[0].UsedUnits != 20 {
		t.Fatalf("top users wrong: %+v", top)
	}
}
