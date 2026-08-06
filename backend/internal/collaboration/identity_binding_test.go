package collaboration

import (
	"context"
	"errors"
	"testing"

	"github.com/allcallall/backend/internal/models"
)

func TestAcceptInviteRejectsUnverifiedWhenIdentityRequired(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	owner := createTestUser(t, db, "id-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Identity Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}

	unverified := createTestUser(t, db, "unverified@example.com", "Unverified")
	verified := createTestUser(t, db, "verified@example.com", "Verified")
	if err := db.Model(&models.User{}).Where("id = ?", verified.ID).Update("identity_verified", true).Error; err != nil {
		t.Fatalf("mark verified failed: %v", err)
	}

	require := true
	if _, err := svc.UpdateOrganizationPolicy(ctx, org.ID, owner.ID, OrganizationPolicyInput{
		RecordingMode:               models.RecordingModeOff,
		RequireIdentityVerification: &require,
	}); err != nil {
		t.Fatalf("update policy failed: %v", err)
	}

	// 未核验用户被拒绝。
	// Unverified user is rejected.
	inviteBad := mustCreateInvite(t, svc, ctx, org.ID, owner.ID, "unverified@example.com")
	if _, err := svc.AcceptOrganizationInvite(ctx, inviteBad.Code, unverified.ID, "unverified@example.com"); !errors.Is(err, ErrIdentityVerificationRequired) {
		t.Fatalf("err=%v want=ErrIdentityVerificationRequired", err)
	}

	// 已核验用户可正常加入。
	// Verified user joins successfully.
	inviteOK := mustCreateInvite(t, svc, ctx, org.ID, owner.ID, "verified@example.com")
	if _, err := svc.AcceptOrganizationInvite(ctx, inviteOK.Code, verified.ID, "verified@example.com"); err != nil {
		t.Fatalf("verified accept failed: %v", err)
	}
	var memberCount int64
	if err := db.Model(&models.OrganizationMember{}).
		Where("organization_id = ? AND user_id = ?", org.ID, verified.ID).Count(&memberCount).Error; err != nil {
		t.Fatalf("count member failed: %v", err)
	}
	if memberCount != 1 {
		t.Fatalf("verified user should be a member, got count=%d", memberCount)
	}
}

func TestUpdateOrganizationPolicyTogglesIdentityRequirement(t *testing.T) {
	svc, _, _ := newServiceTestEnv(t)
	ctx := context.Background()
	owner := createTestUser(t, svc.db, "policy-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Policy Org")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	require := true
	policy, err := svc.UpdateOrganizationPolicy(ctx, org.ID, owner.ID, OrganizationPolicyInput{
		RecordingMode:               models.RecordingModeOff,
		RequireIdentityVerification: &require,
	})
	if err != nil {
		t.Fatalf("update policy failed: %v", err)
	}
	if !policy.RequireIdentityVerification {
		t.Fatal("policy should require identity verification after update")
	}
}

func mustCreateInvite(t *testing.T, svc *Service, ctx context.Context, orgID, inviterID uint64, email string) models.OrganizationInvite {
	t.Helper()
	invite, err := svc.CreateOrganizationInvite(ctx, orgID, inviterID, OrganizationInviteInput{
		TargetEmail: email,
		Role:        models.OrganizationRoleMember,
	})
	if err != nil {
		t.Fatalf("create invite failed: %v", err)
	}
	return *invite
}
