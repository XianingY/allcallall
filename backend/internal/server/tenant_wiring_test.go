package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/allcallall/backend/internal/tenant"
)

// stubResolver 用固定身份模拟"从认证主体解析出租户"。
type stubResolver struct {
	orgID  uint64
	userID uint64
	ok     bool
}

func (r stubResolver) Resolve(*gin.Context) (uint64, uint64, bool) {
	return r.orgID, r.userID, r.ok
}

// 回归点（P0-1）：租户隔离此前是死代码——internal/tenant 整包零外部引用，
// 中间件从未挂到任何路由。本测试锁死"受保护路由链必须包含租户中间件"这一契约。
func TestProtectedMiddlewaresIncludeTenant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("resolver wired", func(t *testing.T) {
		deps := RouteDependencies{
			AuthMiddleware: func(c *gin.Context) { c.Next() },
			TenantResolver: stubResolver{orgID: 77, userID: 9, ok: true},
		}
		chain := protectedMiddlewares(deps)
		if len(chain) != 2 {
			t.Fatalf("expected auth + tenant middleware, got %d", len(chain))
		}
		// 执行整条链，确认租户上下文被写入。
		router := gin.New()
		for _, mw := range chain {
			router.Use(mw)
		}
		var org, user uint64
		router.GET("/x", func(c *gin.Context) {
			org = tenant.OrgID(c)
			user = tenant.UserID(c)
			c.Status(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if org != 77 || user != 9 {
			t.Fatalf("tenant context not propagated: org=%d user=%d want 77/9", org, user)
		}
	})

	t.Run("resolver nil keeps chain auth-only", func(t *testing.T) {
		deps := RouteDependencies{AuthMiddleware: func(c *gin.Context) { c.Next() }}
		chain := protectedMiddlewares(deps)
		if len(chain) != 1 {
			t.Fatalf("expected auth-only chain, got %d", len(chain))
		}
	})
}

// 强制与非强制两种模式的行为差异。
func TestTenantMiddlewareEnforceModes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	unresolved := stubResolver{ok: false}

	t.Run("enforce rejects unresolved", func(t *testing.T) {
		router := gin.New()
		router.Use(tenant.TenantMiddlewareWithConfig(unresolved, tenant.Config{Enforce: true}))
		router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("enforce mode: got %d want 403", rec.Code)
		}
	})

	// 非强制放行是刻意设计：新注册用户尚无组织成员身份，
	// 强制会把他们挡在账户级端点（资料、登出）之外。
	t.Run("non-enforce passes without context", func(t *testing.T) {
		router := gin.New()
		router.Use(tenant.TenantMiddlewareWithConfig(unresolved, tenant.Config{Enforce: false}))
		var org uint64
		router.GET("/x", func(c *gin.Context) {
			org = tenant.OrgID(c)
			c.Status(http.StatusOK)
		})

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("non-enforce mode must not block, got %d", rec.Code)
		}
		if org != 0 {
			t.Fatalf("unresolved tenant must leave org id zero, got %d", org)
		}
	})
}
