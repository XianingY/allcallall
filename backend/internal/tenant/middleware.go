// Package tenant provides unified tenant (organization) identification and
// isolation for HTTP and data-access layers.
//
// Design invariants:
//   - Tenancy is derived ONLY from the authenticated principal. The client-supplied
//     X-Organization-ID header is treated as a routing hint at most, never as an
//     authorization source. Escaping tenant isolation must be impossible by construction.
//   - The middleware stores the resolved org/user in the request context; every
//     data-access path MUST scope queries with Scope(orgID) (or RequireSameOrganization).
package tenant

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/allcallall/backend/internal/auth"
	"gorm.io/gorm"
)

type ctxKey string

const (
	// OrgIDKey holds the resolved organization id in the gin context.
	OrgIDKey ctxKey = "tenant.org_id"
	// UserIDKey holds the authenticated user id in the gin context.
	UserIDKey ctxKey = "tenant.user_id"
)

// ErrTenantUnresolved indicates the request could not be mapped to a tenant.
var ErrTenantUnresolved = errors.New("tenant: request could not be resolved to an organization")

// Resolver extracts the organization and user identity for a request.
// Implementations must be side-effect free and must not trust client headers
// for authorization decisions.
type Resolver interface {
	Resolve(c *gin.Context) (orgID uint64, userID uint64, ok bool)
}

// AuthDBResolver derives the organization from the authenticated JWT principal
// via a caller-supplied lookup. This is the production-safe resolver because it
// ignores any client-provided organization claim.
type AuthDBResolver struct {
	// LookupOrg returns the organization id the given user belongs to.
	LookupOrg func(ctx context.Context, userID uint64) (orgID uint64, err error)
}

// Resolve implements Resolver.
func (r AuthDBResolver) Resolve(c *gin.Context) (uint64, uint64, bool) {
	if r.LookupOrg == nil {
		return 0, 0, false
	}
	claims, err := auth.GetClaimsFromContext(c)
	if err != nil || claims == nil {
		return 0, 0, false
	}
	orgID, err := r.LookupOrg(c.Request.Context(), claims.UserID)
	if err != nil || orgID == 0 {
		return 0, 0, false
	}
	return orgID, claims.UserID, true
}

// HeaderFallbackResolver trusts X-Organization-ID only when a trusted internal
// caller (e.g. another service on the private network) sets X-Trusted-Internal.
// It is NOT suitable for untrusted client traffic.
type HeaderFallbackResolver struct {
	Inner Resolver
}

// Resolve implements Resolver, falling back to the header for internal callers.
func (h HeaderFallbackResolver) Resolve(c *gin.Context) (uint64, uint64, bool) {
	if orgID, userID, ok := h.Inner.Resolve(c); ok {
		return orgID, userID, true
	}
	if c.GetHeader("X-Trusted-Internal") != "true" {
		return 0, 0, false
	}
	orgID := parseUint64(c.GetHeader("X-Organization-ID"))
	userID := parseUint64(c.GetHeader("X-User-ID"))
	if orgID == 0 {
		return 0, 0, false
	}
	return orgID, userID, true
}

// TenantMiddleware enforces that every request is bound to a single tenant and
// stores the resolved identity in the context for downstream handlers and scopes.
func TenantMiddleware(resolver Resolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		orgID, userID, ok := resolver.Resolve(c)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "tenant_unresolved",
				"message": "request is not associated with an organization",
			})
			return
		}
		c.Set(string(OrgIDKey), orgID)
		c.Set(string(UserIDKey), userID)
		c.Next()
	}
}

// OrgID returns the resolved organization id, or 0 if unset.
func OrgID(c *gin.Context) uint64 {
	if v, ok := c.Get(string(OrgIDKey)); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}

// UserID returns the resolved user id, or 0 if unset.
func UserID(c *gin.Context) uint64 {
	if v, ok := c.Get(string(UserIDKey)); ok {
		if id, ok := v.(uint64); ok {
			return id
		}
	}
	return 0
}

// RequireSameOrganization aborts with 403 when the resolved tenant does not own
// the resource identified by resourceOrgID. Use it as a guard before mutating
// any organization-scoped entity.
func RequireSameOrganization(resourceOrgID uint64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if OrgID(c) != resourceOrgID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":   "tenant_mismatch",
				"message": "resource does not belong to the requesting organization",
			})
			return
		}
		c.Next()
	}
}

// Scope returns a GORM scope that restricts a query to a single organization.
// Compose it into every tenant-aware query: db.Scopes(tenant.Scope(orgID)).
func Scope(orgID uint64) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("organization_id = ?", orgID)
	}
}

func parseUint64(s string) uint64 {
	var v uint64
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0
		}
		v = v*10 + uint64(s[i]-'0')
	}
	return v
}
