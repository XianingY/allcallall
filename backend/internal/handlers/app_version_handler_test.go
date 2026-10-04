package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"1.10.0", "1.9.0", 1},
		{"2.0", "1.9.9", 1},
		{"1.0", "1.0.0", 0},
		// Build metadata and pre-release suffixes must not make a version
		// unparseable: a client sending "1.2.3+42" is on 1.2.3.
		{"1.2.3+42", "1.2.3", 0},
		{"1.2.3-rc1", "1.2.3", 0},
		// A segment with no digits at all is treated as 0 rather than failing the
		// comparison, so "1.2.x" reads as 1.2.0 and stays comparable.
		{"1.2.x", "1.2.3", -1},
		{"1.2.x", "1.2.0", 0},
	}
	for _, tc := range cases {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func newVersionTestEnv(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// A file-backed database in a per-test temp dir. ":memory:" is not reliable
	// here: gorm's pool can close the connection between statements, and a
	// shared-cache name can be reused across connections in ways that make a
	// seeded row appear in a later test.
	dsn := filepath.Join(t.TempDir(), "version-policy.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.AppVersionPolicy{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	router := gin.New()
	api := router.Group("/api/v1")
	NewAppVersionHandler(db).RegisterPublicRoutes(api)
	return router, db
}

func check(t *testing.T, router *gin.Engine, platform, version string) appVersionCheckResponse {
	t.Helper()
	url := "/api/v1/app/version-check?platform=" + platform + "&version=" + version
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, url, nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("check(%s, %s) status = %d, body %s", platform, version, rec.Code, rec.Body.String())
	}
	// JSONSuccess writes the payload directly, with no envelope.
	var got appVersionCheckResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	return got
}

// No policy must not mean "update required": a typo in a platform name would
// otherwise lock every installed client out of its own app.
func TestVersionCheckWithoutPolicyAllowsTheClient(t *testing.T) {
	router, _ := newVersionTestEnv(t)

	got := check(t, router, "android", "1.0.0")
	if got.PolicyConfigured {
		t.Error("policy_configured should be false when no row exists")
	}
	if !got.UpdateAllowed {
		t.Error("a client must be allowed to run when no policy is configured")
	}
	if got.UpdateRequired {
		t.Error("update_required must not be inferred from a missing policy")
	}
}

func TestVersionCheckBlocksBuildsBelowTheSupportedFloor(t *testing.T) {
	router, db := newVersionTestEnv(t)
	now := time.Now()
	if err := db.Create(&models.AppVersionPolicy{
		Platform:            "android",
		MinSupportedVersion: "1.2.0",
		LatestVersion:       "1.3.0",
		Message:             "please update",
		ForceUpdate:         true,
		ReleasedAt:          &now,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	old := check(t, router, "android", "1.1.9")
	if old.UpdateAllowed {
		t.Error("1.1.9 is below the supported floor and must not be allowed")
	}
	if !old.UpdateRequired {
		t.Error("a blocked client should also be told an update exists")
	}
	if old.Message != "please update" {
		t.Errorf("message = %q, want the stored one", old.Message)
	}

	current := check(t, router, "android", "1.2.0")
	if !current.UpdateAllowed {
		t.Error("1.2.0 is exactly at the floor and must be allowed")
	}
	if !current.UpdateRequired {
		t.Error("1.2.0 is below the latest 1.3.0, so an update is required")
	}

	newest := check(t, router, "android", "1.3.0")
	if newest.UpdateRequired {
		t.Error("1.3.0 is the latest and should not require an update")
	}
	if newest.ForceUpdate != true {
		t.Error("force_update should be echoed so the client can present a blocking screen")
	}
}

// A malformed floor must fail open. A gate that cannot be parsed should not
// become a gate that locks everyone out.
func TestVersionCheckTreatsAnUnparseableFloorAsNoFloor(t *testing.T) {
	router, db := newVersionTestEnv(t)
	if err := db.Create(&models.AppVersionPolicy{
		Platform:            "ios",
		MinSupportedVersion: "not-a-version",
		LatestVersion:       "2.0.0",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	got := check(t, router, "ios", "1.0.0")
	if !got.UpdateAllowed {
		t.Error("an unparseable floor must not block clients")
	}
	if !got.UpdateRequired {
		t.Error("the comparison against LatestVersion should still stand")
	}
}

// Policies are per platform: an android row must not gate ios.
func TestVersionCheckIsScopedToItsPlatform(t *testing.T) {
	router, db := newVersionTestEnv(t)
	if err := db.Create(&models.AppVersionPolicy{
		Platform:            "android",
		MinSupportedVersion: "9.0.0",
		LatestVersion:       "9.0.0",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	if got := check(t, router, "ios", "1.0.0"); !got.UpdateAllowed {
		t.Error("an android policy must not gate ios clients")
	}
	if got := check(t, router, "android", "1.0.0"); got.UpdateAllowed {
		t.Error("the android policy should have blocked this build")
	}
}

func TestVersionCheckRejectsBadParameters(t *testing.T) {
	router, _ := newVersionTestEnv(t)

	for _, query := range []string{
		"/api/v1/app/version-check?platform=windows&version=1.0.0",
		"/api/v1/app/version-check?platform=android",
		"/api/v1/app/version-check?version=1.0.0",
	} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, query, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400", query, rec.Code)
		}
	}
}
