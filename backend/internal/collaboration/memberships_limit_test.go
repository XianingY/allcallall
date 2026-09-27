package collaboration

import (
	"context"
	"fmt"
	"testing"

	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/pagination"
)

// TestListMembershipsBoundedByLimit 验证 listMemberships（requireCurrentOrganization
// 热路径上的查询）受上限约束：用户持有 limit+1 个组织成员关系时，只返回 limit 条，
// 且保持 organizations.id ASC 的原有顺序。
//
// 上限通过包级变量 membershipListLimit 注入（生产值 = pagination.MaxLimit），
// 测试临时调低以避免插入 501 条成员关系。
func TestListMembershipsBoundedByLimit(t *testing.T) {
	if membershipListLimit != pagination.MaxLimit {
		t.Fatalf("membershipListLimit default = %d, want pagination.MaxLimit (%d)", membershipListLimit, pagination.MaxLimit)
	}

	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()

	user := createTestUser(t, db, "memberships-limit@example.com", "Limit User")

	const loweredLimit = 3
	original := membershipListLimit
	membershipListLimit = loweredLimit
	t.Cleanup(func() { membershipListLimit = original })

	wantOrgs := loweredLimit + 1
	createdIDs := make([]uint64, 0, wantOrgs)
	for i := 0; i < wantOrgs; i++ {
		org, err := svc.CreateOrganization(ctx, user.ID, fmt.Sprintf("Limit Org %d", i))
		if err != nil {
			t.Fatalf("create organization %d failed: %v", i, err)
		}
		createdIDs = append(createdIDs, org.ID)
	}

	rows, err := svc.listMemberships(ctx, user.ID)
	if err != nil {
		t.Fatalf("listMemberships failed: %v", err)
	}
	if len(rows) != loweredLimit {
		t.Fatalf("listMemberships returned %d rows, want exactly %d (limit) when user has %d memberships", len(rows), loweredLimit, wantOrgs)
	}
	for i, row := range rows {
		if row.ID != createdIDs[i] {
			t.Fatalf("row %d: org ID = %d, want %d (ordering must remain organizations.id ASC)", i, row.ID, createdIDs[i])
		}
		if row.Role != models.OrganizationRoleOwner {
			t.Fatalf("row %d: role = %q, want %q", i, row.Role, models.OrganizationRoleOwner)
		}
	}
}
