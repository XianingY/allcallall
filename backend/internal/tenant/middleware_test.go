package tenant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/allcallall/backend/internal/auth"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() { gin.SetMode(gin.TestMode) }

// withAuth injects a claims principal before the tenant middleware, exactly as
// the real request chain orders auth -> tenant. router.ServeHTTP builds its own
// context, so claims must be set inside the chain, never on a detached context.
func withAuth(userID uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: userID, Email: "u@example.com"})
		c.Next()
	}
}

func newRouter(resolver Resolver, userID uint64) *gin.Engine {
	router := gin.New()
	router.Use(withAuth(userID))
	router.Use(TenantMiddleware(resolver))
	router.GET("/me", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"org": OrgID(c), "user": UserID(c)})
	})
	router.GET("/res/:org", RequireSameOrganization(99), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return router
}

func TestTenantMiddlewareSetsContextAndScopesQuery(t *testing.T) {
	resolver := AuthDBResolver{LookupOrg: func(_ context.Context, userID uint64) (uint64, error) {
		if userID == 7 {
			return 42, nil
		}
		return 0, context.Canceled
	}}

	// Authenticated user 7 -> org 42
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	newRouter(resolver, 7).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body struct {
		Org  uint64 `json:"org"`
		User uint64 `json:"user"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Org != 42 || body.User != 7 {
		t.Fatalf("unexpected tenant context: org=%d user=%d", body.Org, body.User)
	}

	// Unknown user -> 403
	w2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/me", nil)
	newRouter(resolver, 99).ServeHTTP(w2, req2)
	if w2.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for unresolved tenant, got %d", w2.Code)
	}
}

func TestRequireSameOrganizationGuard(t *testing.T) {
	resolver := AuthDBResolver{LookupOrg: func(_ context.Context, userID uint64) (uint64, error) {
		return 42, nil
	}}
	// Org 42 != 99 -> forbidden
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/res/99", nil)
	newRouter(resolver, 7).ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 on tenant mismatch, got %d", w.Code)
	}
}

func TestScopeInjectsOrganizationPredicate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	type Row struct {
		ID             uint64 `gorm:"primaryKey"`
		OrganizationID uint64
		Payload        string
	}
	if err := db.AutoMigrate(&Row{}); err != nil {
		t.Fatal(err)
	}
	db.Create(&Row{OrganizationID: 42, Payload: "belongs"})
	db.Create(&Row{OrganizationID: 99, Payload: "other"})

	var rows []Row
	if err := db.Scopes(Scope(42)).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Payload != "belongs" {
		t.Fatalf("Scope did not isolate tenant, got %+v", rows)
	}
}
