package collaboration

import (
	"context"
	"errors"
	"testing"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// seedOwnerContactRow seeds the contacts row shape production writers actually produce.
// contact/repository.go AddContact only sets OwnerID+ContactID and leaves
// OrganizationID at its 0 default; every API-facing contact id is a users.* id
// (handleListContacts returns users.* via contacts.contact_id filtered by owner_id).
// 联系人 id 就是用户 id，归属栅栏是 owner_id，contacts.organization_id 在生产中恒为 0；
// 因此校验必须走 owner_id + contact_id，不能用 organization_id 或自增的 contacts.id。
// Contact ids are user ids and the ownership fence is owner_id, because
// contacts.organization_id stays 0 in production; validation must scope on
// owner_id + contact_id, never on organization_id or the autoincrement contacts.id.
func seedOwnerContactRow(t *testing.T, db *gorm.DB, ownerID, contactUserID uint64) models.Contact {
	t.Helper()
	row := models.Contact{
		OwnerID:   ownerID,
		ContactID: contactUserID,
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create contact failed: %v", err)
	}
	return row
}

func loadConversationContact(t *testing.T, db *gorm.DB, conversationID uint64) *uint64 {
	t.Helper()
	var conv models.Conversation
	if err := db.Where("id = ?", conversationID).Take(&conv).Error; err != nil {
		t.Fatalf("reload conversation failed: %v", err)
	}
	return conv.ContactID
}

func TestUpdateConversationContactBindingIsValidatedAgainstCallerContacts(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	owner := createTestUser(t, db, "contact-fence-owner@example.com", "Owner")
	inList := createTestUser(t, db, "contact-fence-inlist@example.com", "In List")
	foreign := createTestUser(t, db, "contact-fence-foreign@example.com", "Foreign")

	org, err := svc.CreateOrganization(ctx, owner.ID, "Contact Fence Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	conv, err := svc.CreateConversation(ctx, org.ID, owner.ID, CreateConversationInput{
		Type:  models.ConversationTypeChannel,
		Title: "Contact Fence",
	})
	if err != nil {
		t.Fatalf("create conversation failed: %v", err)
	}

	// 调用者的联系人列表里只有 inList，foreign 永远不在其中。
	// The caller's contact list contains only inList; foreign is never in it.
	seedOwnerContactRow(t, db, owner.ID, inList.ID)
	nonexistent := owner.ID + 1000000

	t.Run("contact outside the caller's list is rejected and nothing is written", func(t *testing.T) {
		foreignID := foreign.ID
		_, err := svc.UpdateConversation(ctx, org.ID, owner.ID, conv.ID, UpdateConversationInput{
			ContactID: &foreignID,
		})
		if err == nil {
			t.Fatal("expected contact outside the caller's contact list to be rejected, got nil error")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected gorm.ErrRecordNotFound (whitelisted 400), got %v", err)
		}
		if got := loadConversationContact(t, db, conv.ID); got != nil {
			t.Fatalf("rejected update must write nothing, but contact_id = %d", *got)
		}
	})

	t.Run("nonexistent contact is rejected", func(t *testing.T) {
		_, err := svc.UpdateConversation(ctx, org.ID, owner.ID, conv.ID, UpdateConversationInput{
			ContactID: &nonexistent,
		})
		if err == nil {
			t.Fatal("expected nonexistent contact_id to be rejected, got nil error")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected gorm.ErrRecordNotFound (whitelisted 400), got %v", err)
		}
		if got := loadConversationContact(t, db, conv.ID); got != nil {
			t.Fatalf("rejected update must write nothing, but contact_id = %d", *got)
		}
	})

	t.Run("contact in the caller's own list is accepted", func(t *testing.T) {
		inListID := inList.ID
		updated, err := svc.UpdateConversation(ctx, org.ID, owner.ID, conv.ID, UpdateConversationInput{
			ContactID: &inListID,
		})
		if err != nil {
			t.Fatalf("binding a caller-owned contact failed: %v", err)
		}
		if updated.ContactID == nil || *updated.ContactID != inListID {
			t.Fatalf("expected contact %d to be bound, got %#v", inListID, updated.ContactID)
		}
	})

	t.Run("zero contact id still unbinds to null", func(t *testing.T) {
		zero := uint64(0)
		updated, err := svc.UpdateConversation(ctx, org.ID, owner.ID, conv.ID, UpdateConversationInput{
			ContactID: &zero,
		})
		if err != nil {
			t.Fatalf("unbind contact failed: %v", err)
		}
		if updated.ContactID != nil {
			t.Fatalf("expected contact_id to be cleared to NULL, got %d", *updated.ContactID)
		}
		if got := loadConversationContact(t, db, conv.ID); got != nil {
			t.Fatalf("expected NULL contact_id in database, got %d", *got)
		}
	})
}
