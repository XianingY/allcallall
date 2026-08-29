package runtime

import (
	"context"
	"os"
	"strings"

	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/tenant"
)

// TenantEnforceFromEnv 读取租户隔离的强制开关（TENANT_ISOLATION_ENFORCE）。
//
// 默认关闭：本产品注册用户不会自动获得组织成员身份，一旦全局强制，
// 尚无组织的用户会被账户级端点（资料、登出等）403 挡在门外。
// 推荐节奏：先全量标注（中间件始终挂载），待确认用户均有归属后再开启强制。
// TenantEnforceFromEnv reports whether unresolvable tenants must be rejected.
func TenantEnforceFromEnv() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("TENANT_ISOLATION_ENFORCE")))
	return v == "1" || v == "true" || v == "yes"
}

// TenantResolverFromService builds the production-safe tenant resolver: the
// organization is derived from the authenticated JWT principal and looked up
// through the organization service, so a client-supplied X-Organization-ID can
// never widen a request's tenant scope.
//
// Passing a nil service disables tenant annotation entirely.
func TenantResolverFromService(svc *collaboration.Service) tenant.Resolver {
	if svc == nil {
		return nil
	}
	return tenant.AuthDBResolver{
		LookupOrg: func(ctx context.Context, userID uint64) (uint64, error) {
			if userID == 0 {
				return 0, nil
			}
			// requestedID=0：取该用户的第一个归属组织，与既有 service 行为一致。
			org, _, err := svc.ResolveOrganization(ctx, userID, 0)
			if err != nil {
				return 0, err
			}
			if org == nil {
				return 0, nil
			}
			return org.ID, nil
		},
	}
}
