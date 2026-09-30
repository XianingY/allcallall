package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/commerce"
	"github.com/allcallall/backend/internal/models"
)

func newOrgBillingTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "org-billing.db")), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.OrganizationPlan{},
		&models.OrganizationUsageLedger{},
		&models.Invoice{},
		&models.InvoiceLine{},
		&models.QuotaPolicy{},
		// 用量看板需要统计组织成员数。
		&models.OrganizationMember{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func newOrgBillingTestRouter(t *testing.T, orgID uint64) (*gin.Engine, *commerce.OrgRepository) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db := newOrgBillingTestDB(t)
	orgRepo := commerce.NewOrgRepository(db)
	entitlement := commerce.NewEntitlementService(commerce.NewRepository(db), nil)
	handler := NewOrgBillingHandler(
		zerolog.Nop(),
		func(_ context.Context, _ uint64) (uint64, error) {
			if orgID == 0 {
				return 0, nil
			}
			return orgID, nil
		},
		commerce.NewOrgBillingService(orgRepo),
		commerce.NewUsageStatsService(orgRepo),
		commerce.NewInvoiceService(orgRepo),
		commerce.NewQuotaService(orgRepo, entitlement),
	)

	router := gin.New()
	api := router.Group("/api/v1")
	// 复刻生产链路：上游鉴权中间件写入认证主体，组织归属由认证主体派生。
	api.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: 7})
		c.Next()
	})
	handler.RegisterProtectedRoutes(api)
	return router, orgRepo
}

// 回归点（P0-4）：B2B 计费服务此前仅定义未实例化，HTTP 层无入口。
// 本测试锁死"组织级端点已接线且按租户隔离"这一契约。
func TestOrgBillingRoutes(t *testing.T) {
	t.Run("without tenant context", func(t *testing.T) {
		router, _ := newOrgBillingTestRouter(t, 0)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/org/plan", nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("org-scoped endpoint must require tenant, got %d", rec.Code)
		}
	})

	t.Run("with tenant context", func(t *testing.T) {
		router, orgRepo := newOrgBillingTestRouter(t, 4242)

		// 预置一条用量账本，验证端点确实读到该组织的数据。
		if err := commerce.NewOrgBillingService(orgRepo).
			RecordOrganizationUsage(context.Background(), 4242, "translation", 30); err != nil {
			t.Fatalf("seed usage: %v", err)
		}

		for _, path := range []string{"/api/v1/org/plan", "/api/v1/org/usage?feature=translation"} {
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("%s: got %d body=%s", path, rec.Code, rec.Body.String())
			}
		}
	})

	t.Run("invoice not found is 404", func(t *testing.T) {
		router, _ := newOrgBillingTestRouter(t, 4242)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/org/invoices/NOPE-1", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("missing invoice: got %d want 404", rec.Code)
		}
	})
}

func TestLastNPeriods(t *testing.T) {
	got := lastNPeriods("2026-08", 3)
	want := []string{"2026-06", "2026-07", "2026-08"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	// 非法周期应回退到当前时间而非 panic/空结果。
	if len(lastNPeriods("not-a-period", 2)) != 2 {
		t.Fatal("invalid period must fall back to now")
	}
}
