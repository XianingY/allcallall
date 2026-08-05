package commerce

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/metrics"
	"github.com/allcallall/backend/internal/models"
)

func setupTestDB(t *testing.T, name string) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	if err := db.AutoMigrate(
		&models.UserEntitlement{},
		&models.UsageLedger{},
		&models.TranslationUsageSlice{},
	); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}

func TestEnsureDefaultEntitlement(t *testing.T) {
	db := setupTestDB(t, "entitlement.db")
	repo := NewRepository(db)
	svc := NewEntitlementService(repo, metrics.NewNoopRecorder())
	ctx := context.Background()

	// 1. Should create a free tier for a new user
	ent1, err := svc.EnsureDefaultEntitlement(ctx, 1)
	if err != nil {
		t.Fatalf("failed to ensure default entitlement: %v", err)
	}
	if ent1.Tier != models.EntitlementFree {
		t.Errorf("expected Free tier, got %v", ent1.Tier)
	}

	// 2. Should return the same entitlement if called again
	ent2, err := svc.EnsureDefaultEntitlement(ctx, 1)
	if err != nil {
		t.Fatalf("failed second call: %v", err)
	}
	if ent1.ID != ent2.ID {
		t.Errorf("expected same ID %d, got %d", ent1.ID, ent2.ID)
	}

	// 3. Should return existing premium if present
	premium := &models.UserEntitlement{
		UserID:      2,
		Entitlement: models.EntitlementPremium,
		Tier:        models.EntitlementPremium,
		Status:      "active",
	}
	if err := db.Create(premium).Error; err != nil {
		t.Fatal(err)
	}

	ent3, err := svc.EnsureDefaultEntitlement(ctx, 2)
	if err != nil {
		t.Fatalf("failed premium user: %v", err)
	}
	if ent3.Tier != models.EntitlementPremium {
		t.Errorf("expected Premium tier, got %v", ent3.Tier)
	}
}

func TestActiveTier(t *testing.T) {
	db := setupTestDB(t, "activetier.db")
	repo := NewRepository(db)
	svc := NewEntitlementService(repo, metrics.NewNoopRecorder())
	ctx := context.Background()

	// New user -> Free
	tier, err := svc.ActiveTier(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if tier != models.EntitlementFree {
		t.Errorf("expected free, got %s", tier)
	}

	// User with active Premium
	premium := &models.UserEntitlement{
		UserID:      2,
		Entitlement: models.EntitlementPremium,
		Tier:        models.EntitlementPremium,
		Status:      "active",
	}
	db.Create(premium)

	tier2, err := svc.ActiveTier(ctx, 2)
	if err != nil || tier2 != models.EntitlementPremium {
		t.Errorf("expected premium, got %s, err: %v", tier2, err)
	}
}

func TestConsumeTranslationSeconds(t *testing.T) {
	db := setupTestDB(t, "consumesec.db")
	repo := NewRepository(db)
	svc := NewEntitlementService(repo, metrics.NewNoopRecorder())
	ctx := context.Background()

	// 1. Consume 100 seconds for a free user
	err := svc.ConsumeTranslationSeconds(ctx, 1, 100)
	if err != nil {
		t.Fatalf("consume 100 failed: %v", err)
	}

	usage, err := svc.GetUsage(ctx, 1)
	if err != nil || len(usage) == 0 {
		t.Fatalf("get usage failed: %v", err)
	}
	if usage[0].UsedUnits != 100 {
		t.Errorf("expected 100 used units, got %d", usage[0].UsedUnits)
	}

	// 2. Consume over quota should fail
	err = svc.ConsumeTranslationSeconds(ctx, 1, 2000) // limit is 1800
	if err != ErrTranslationQuotaExhausted {
		t.Errorf("expected ErrTranslationQuotaExhausted, got %v", err)
	}

	// 3. Premium users are not charged
	db.Create(&models.UserEntitlement{
		UserID:      2,
		Entitlement: models.EntitlementPremium,
		Tier:        models.EntitlementPremium,
		Status:      "active",
	})

	err = svc.ConsumeTranslationSeconds(ctx, 2, 5000) // way over free quota
	if err != nil {
		t.Errorf("premium user should not fail on consume: %v", err)
	}

	usage2, _ := svc.GetUsage(ctx, 2)
	if usage2[0].UsedUnits != 0 {
		t.Errorf("premium user should have 0 usage recorded, got %d", usage2[0].UsedUnits)
	}
}
