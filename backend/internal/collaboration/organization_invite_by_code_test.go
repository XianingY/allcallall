package collaboration

import (
	"context"
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// TestGetOrganizationInviteByCode pins the lookup contract the mobile accept
// screen depends on: only a pending, unexpired invite is resolvable by code,
// and every other state has a distinct, client-safe error.
func TestGetOrganizationInviteByCode(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	owner := createTestUser(t, db, "invite-by-code-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Invite By Code Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}

	t.Run("pending invite resolves", func(t *testing.T) {
		invite := mustCreateInvite(t, svc, ctx, org.ID, owner.ID, "pending-by-code@example.com")
		got, err := svc.GetOrganizationInviteByCode(ctx, invite.Code)
		if err != nil {
			t.Fatalf("lookup failed: %v", err)
		}
		if got.Code != invite.Code || got.ID != invite.ID {
			t.Fatalf("got invite id=%d code=%s want id=%d code=%s", got.ID, got.Code, invite.ID, invite.Code)
		}
		if got.OrganizationID != org.ID {
			t.Fatalf("got organization_id=%d want %d", got.OrganizationID, org.ID)
		}
	})

	t.Run("empty code is rejected", func(t *testing.T) {
		if _, err := svc.GetOrganizationInviteByCode(ctx, "   "); err == nil || err.Error() != "invite code is required" {
			t.Fatalf("err=%v want invite code is required", err)
		}
	})

	t.Run("unknown code reports not found", func(t *testing.T) {
		if _, err := svc.GetOrganizationInviteByCode(ctx, "no-such-invite-code"); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("err=%v want gorm.ErrRecordNotFound", err)
		}
	})

	t.Run("revoked invite is not resolvable", func(t *testing.T) {
		invite := mustCreateInvite(t, svc, ctx, org.ID, owner.ID, "revoked-by-code@example.com")
		if err := svc.RevokeOrganizationInvite(ctx, org.ID, owner.ID, invite.ID); err != nil {
			t.Fatalf("revoke failed: %v", err)
		}
		if _, err := svc.GetOrganizationInviteByCode(ctx, invite.Code); err == nil || err.Error() != "invite is no longer pending" {
			t.Fatalf("err=%v want invite is no longer pending", err)
		}
	})

	t.Run("expired invite is not resolvable", func(t *testing.T) {
		invite := mustCreateInvite(t, svc, ctx, org.ID, owner.ID, "expired-by-code@example.com")
		past := time.Now().Add(-time.Hour)
		if err := db.Model(&models.OrganizationInvite{}).
			Where("id = ?", invite.ID).
			Update("expires_at", past).Error; err != nil {
			t.Fatalf("expire invite failed: %v", err)
		}
		if _, err := svc.GetOrganizationInviteByCode(ctx, invite.Code); err == nil || err.Error() != "invite has expired" {
			t.Fatalf("err=%v want invite has expired", err)
		}
	})
}
