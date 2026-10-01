package handlers

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/auth"
	"github.com/allcallall/backend/internal/collaboration"
	"github.com/allcallall/backend/internal/models"
	"github.com/allcallall/backend/internal/search"
	"github.com/allcallall/backend/internal/user"
)

// stubSearchIndexer is the fake search backend for handleSearchMessages. It
// records the query it was handed so tests can assert what the handler
// forwards (or refuses to forward) and always returns an empty result set —
// FilterSearchResults runs even for empty results, which is what the error
// paths below rely on.
type stubSearchIndexer struct {
	calls     int
	lastQuery search.MessageSearchQuery
}

func (f *stubSearchIndexer) IndexMessage(_ context.Context, _ search.MessageDocument) error {
	return nil
}

func (f *stubSearchIndexer) SearchMessages(_ context.Context, query search.MessageSearchQuery) ([]search.MessageSearchResult, error) {
	f.calls++
	f.lastQuery = query
	return []search.MessageSearchResult{}, nil
}

// searchTestEnv reuses the wave-1 sqlite harness for owner/org setup, then
// wires a CollaborationHandler with the fake search backend and a capturing
// logger so both the wire response and the server-side log can be asserted.
type searchTestEnv struct {
	router  *gin.Engine
	db      *gorm.DB
	org     *models.Organization
	indexer *stubSearchIndexer
	logs    bytes.Buffer
}

func newSearchTestEnv(t *testing.T) *searchTestEnv {
	t.Helper()
	_, db, org := newCollaborationErrorTestEnv(t, false)

	env := &searchTestEnv{db: db, org: org, indexer: &stubSearchIndexer{}}
	userSvc := user.NewService(user.NewRepository(db))
	handler := NewCollaborationHandler(
		zerolog.New(&env.logs),
		collaboration.NewService(db, userSvc),
		userSvc,
		collaboration.NewChatHub(nil, zerolog.Nop()),
		nil,
	)
	handler.WithSearchService(search.NewService(env.indexer))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		auth.SetClaimsToContext(c, &auth.Claims{UserID: org.CreatedBy, Email: "owner@example.com"})
		c.Next()
	})
	handler.RegisterProtectedRoutes(router.Group("/api/v1"))
	env.router = router
	return env
}

// injectSecondOrganizationsQueryError fails the second query against the
// organizations table with err. Within one search request the first call is
// requireCurrentOrganization (which must succeed) and the second is
// FilterSearchResults re-resolving the organization, so the injected error
// surfaces exactly where the handler decides what a client may see.
func injectSecondOrganizationsQueryError(t *testing.T, db *gorm.DB, err error) {
	t.Helper()
	organizationsQueries := 0
	callback := func(tx *gorm.DB) {
		if tx.Statement.Table != "organizations" {
			return
		}
		organizationsQueries++
		if organizationsQueries == 2 {
			tx.AddError(err)
		}
	}
	if regErr := db.Callback().Query().Before("gorm:query").
		Register("test:fail_second_organizations_query", callback); regErr != nil {
		t.Fatalf("register query callback failed: %v", regErr)
	}
}

// RED wave 2: limit=-1 and limit=0 were parsed and forwarded straight to
// search.SearchMessages with no bounds check; they must be rejected like the
// recording transcript handler rejects them.
func TestHandleSearchMessagesRejectsNonPositiveLimit(t *testing.T) {
	for _, limit := range []string{"-1", "0"} {
		t.Run("limit="+limit, func(t *testing.T) {
			env := newSearchTestEnv(t)

			rec := performJSON(t, env.router, http.MethodGet, "/api/v1/search/messages?q=deploy&limit="+limit, env.org.ID, "")

			assertServiceError(t, rec, http.StatusBadRequest, "BAD_REQUEST", "invalid limit")
			if env.indexer.calls != 0 {
				t.Fatalf("search backend must not be called for invalid limit, got %d calls", env.indexer.calls)
			}
		})
	}
}

// RED wave 2: limit=10^9 was forwarded verbatim; values above the search
// package's own ceiling (50, see search/service.go SearchMessages) must be
// clamped instead of silently rewritten to 20 by the search service.
func TestHandleSearchMessagesClampsLimitToSearchCeiling(t *testing.T) {
	env := newSearchTestEnv(t)

	rec := performJSON(t, env.router, http.MethodGet, "/api/v1/search/messages?q=deploy&limit=1000000000", env.org.ID, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for oversized limit, got %d body=%s", rec.Code, rec.Body.String())
	}
	if got := env.indexer.lastQuery.Limit; got != 50 {
		t.Fatalf("expected forwarded limit clamped to 50, got %d", got)
	}
}

// Guard: omitting limit keeps the historical default of 20.
func TestHandleSearchMessagesDefaultLimitUnchanged(t *testing.T) {
	env := newSearchTestEnv(t)

	rec := performJSON(t, env.router, http.MethodGet, "/api/v1/search/messages?q=deploy", env.org.ID, "")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for default limit, got %d body=%s", rec.Code, rec.Body.String())
	}
	if env.indexer.calls != 1 {
		t.Fatalf("expected exactly one search call, got %d", env.indexer.calls)
	}
	if got := env.indexer.lastQuery.Limit; got != 20 {
		t.Fatalf("expected default limit 20, got %d", got)
	}
}

// RED wave 2: a non-whitelisted FilterSearchResults failure used to answer
// 403 with err.Error(), putting raw internals (SQL/table names) on the wire.
func TestHandleSearchMessagesFilterFailureDoesNotLeakInternals(t *testing.T) {
	env := newSearchTestEnv(t)
	injectSecondOrganizationsQueryError(t, env.db, errors.New("no such table: organization_members"))

	rec := performJSON(t, env.router, http.MethodGet, "/api/v1/search/messages?q=deploy", env.org.ID, "")

	assertNoInternalLeak(t, rec)
	assertServiceError(t, rec, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "failed to filter search results")

	logs := env.logs.String()
	if !strings.Contains(logs, "no such table: organization_members") {
		t.Fatalf("expected the raw error to be logged server-side, logs=%q", logs)
	}
	if !strings.Contains(logs, "/api/v1/search/messages") {
		t.Fatalf("expected the request path to be logged, logs=%q", logs)
	}
}

// RED wave 2: a whitelisted sentinel must be answered with the sentinel's own
// message — err.Error() carried the wrapper prefix ("resolve memberships: ...").
func TestHandleSearchMessagesFilterAccessDeniedKeepsWhitelistedMessage(t *testing.T) {
	env := newSearchTestEnv(t)
	injectSecondOrganizationsQueryError(t, env.db,
		fmt.Errorf("resolve memberships: %w", collaboration.ErrOrganizationAccessDenied))

	rec := performJSON(t, env.router, http.MethodGet, "/api/v1/search/messages?q=deploy", env.org.ID, "")

	assertServiceError(t, rec, http.StatusForbidden, "FORBIDDEN", "organization access denied")
	assertNoInternalLeak(t, rec, "resolve memberships")
}
