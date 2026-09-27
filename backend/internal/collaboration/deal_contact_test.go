package collaboration

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// seedDealContactRow creates a contacts row for contactUserID inside organizationID.
func seedDealContactRow(t *testing.T, db *gorm.DB, organizationID, ownerID, contactUserID uint64) models.Contact {
	t.Helper()
	row := models.Contact{
		OrganizationID: organizationID,
		OwnerID:        ownerID,
		ContactID:      contactUserID,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create contact failed: %v", err)
	}
	return row
}

func countDealContactRows(t *testing.T, db *gorm.DB, dealID, contactID uint64) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&models.DealContact{}).
		Where("deal_id = ? AND contact_id = ?", dealID, contactID).
		Count(&count).Error; err != nil {
		t.Fatalf("count deal contacts failed: %v", err)
	}
	return count
}

func TestAddDealContactTenantScope(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	orgAUser := createTestUser(t, db, "deal-contact-a@example.com", "Org A Owner")
	orgBUser := createTestUser(t, db, "deal-contact-b@example.com", "Org B Owner")

	orgA, err := svc.CreateOrganization(ctx, orgAUser.ID, "Deal Scope Org A")
	if err != nil {
		t.Fatalf("create organization A failed: %v", err)
	}
	orgB, err := svc.CreateOrganization(ctx, orgBUser.ID, "Deal Scope Org B")
	if err != nil {
		t.Fatalf("create organization B failed: %v", err)
	}

	dealA, err := svc.CreateDeal(ctx, orgA.ID, orgAUser.ID, DealInput{Title: "Deal A"})
	if err != nil {
		t.Fatalf("create deal A failed: %v", err)
	}
	dealB, err := svc.CreateDeal(ctx, orgB.ID, orgBUser.ID, DealInput{Title: "Deal B"})
	if err != nil {
		t.Fatalf("create deal B failed: %v", err)
	}

	contactA := seedDealContactRow(t, db, orgA.ID, orgAUser.ID, orgBUser.ID)
	contactB := seedDealContactRow(t, db, orgB.ID, orgBUser.ID, orgAUser.ID)

	t.Run("same organization deal and contact link succeeds", func(t *testing.T) {
		if err := svc.AddDealContact(ctx, orgA.ID, orgAUser.ID, dealA.ID, contactA.ID); err != nil {
			t.Fatalf("AddDealContact within one organization failed: %v", err)
		}
		if got := countDealContactRows(t, db, dealA.ID, contactA.ID); got != 1 {
			t.Fatalf("expected 1 deal_contacts row, got %d", got)
		}
	})

	t.Run("deal from another organization is rejected", func(t *testing.T) {
		err := svc.AddDealContact(ctx, orgA.ID, orgAUser.ID, dealB.ID, contactA.ID)
		if err == nil {
			t.Fatal("expected cross-tenant deal link to fail, got nil error")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected gorm.ErrRecordNotFound (same as GetDeal/UpdateDeal), got %v", err)
		}
		if got := countDealContactRows(t, db, dealB.ID, contactA.ID); got != 0 {
			t.Fatalf("cross-tenant deal link must not persist, got %d rows", got)
		}
	})

	t.Run("contact from another organization is rejected", func(t *testing.T) {
		err := svc.AddDealContact(ctx, orgA.ID, orgAUser.ID, dealA.ID, contactB.ID)
		if err == nil {
			t.Fatal("expected cross-tenant contact link to fail, got nil error")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected gorm.ErrRecordNotFound (same as GetDeal/UpdateDeal), got %v", err)
		}
		if got := countDealContactRows(t, db, dealA.ID, contactB.ID); got != 0 {
			t.Fatalf("cross-tenant contact link must not persist, got %d rows", got)
		}
	})

	t.Run("zero contact id is rejected", func(t *testing.T) {
		err := svc.AddDealContact(ctx, orgA.ID, orgAUser.ID, dealA.ID, 0)
		if err == nil {
			t.Fatal("expected contact_id=0 to be rejected, got nil error")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("contact_id=0 must be an invalid-request error, not a lookup miss: %v", err)
		}
		if got := countDealContactRows(t, db, dealA.ID, 0); got != 0 {
			t.Fatalf("contact_id=0 must not persist, got %d rows", got)
		}
	})
}
