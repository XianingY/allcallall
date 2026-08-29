package commerce

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/alerting"
	"github.com/allcallall/backend/internal/models"
)

// captureProvider 是测试用的告警接收器，记录所有发出的告警。
type captureProvider struct {
	mu     sync.Mutex
	alerts []alerting.Alert
}

func (c *captureProvider) Notify(_ context.Context, a alerting.Alert) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.alerts = append(c.alerts, a)
	return nil
}

func (c *captureProvider) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.alerts)
}

func (c *captureProvider) lastTitle() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.alerts) == 0 {
		return ""
	}
	return c.alerts[len(c.alerts)-1].Title
}

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
	captured := &captureProvider{}
	alerter := alerting.NewService(alerting.Routing{alerting.SeverityP2: {captured}})
	qs := NewQuotaService(repo, ent).WithAlerter(alerter)

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
	_, _ = NewOrgBillingService(repo).EnsureOrganizationPlan(ctx, 10, "business", "Business", "monthly", 5, start, start.AddDate(0, 1, 0))
	_ = repo.UpsertQuotaPolicy(ctx, &models.QuotaPolicy{PlanID: "business", Feature: "advanced_analytics", Unlimited: true})
	dec, err = qs.CheckFeatureAccess(ctx, 10, 7, "advanced_analytics")
	if err != nil {
		t.Fatal(err)
	}
	if !dec.Allowed {
		t.Fatal("expected org-granted access for premium feature")
	}

	// Org quota exceeded -> denied AND a P2 alert is emitted.
	start2 := time.Now().UTC()
	_, _ = NewOrgBillingService(repo).EnsureOrganizationPlan(ctx, 20, "team", "Team", "monthly", 5, start2, start2.AddDate(0, 1, 0))
	_ = repo.UpsertQuotaPolicy(ctx, &models.QuotaPolicy{PlanID: "team", Feature: "translation_seconds", LimitUnits: 100})
	_ = NewOrgBillingService(repo).RecordOrganizationUsage(ctx, 20, "translation_seconds", 100)
	before := captured.count()
	dec, err = qs.CheckFeatureAccess(ctx, 20, 3, "translation_seconds")
	if err != nil {
		t.Fatal(err)
	}
	if dec.Allowed {
		t.Fatal("expected org_quota_exceeded denial")
	}
	if dec.Reason != "org_quota_exceeded" {
		t.Fatalf("expected org_quota_exceeded reason, got %q", dec.Reason)
	}
	if captured.count() != before+1 {
		t.Fatalf("expected exactly one quota-breach alert, got %d", captured.count()-before)
	}
	if captured.lastTitle() != "tenant quota exceeded" {
		t.Fatalf("unexpected alert title: %q", captured.lastTitle())
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
