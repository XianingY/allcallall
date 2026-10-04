package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// AppVersionHandler answers "may this build keep running, or must it update".
//
// Registered on the unauthenticated route group on purpose: a client that is
// too old to complete a normal request still has to be able to ask, and the
// answer is a version number rather than user data. The endpoints are
// rate-limited like the rest of the public surface.
type AppVersionHandler struct {
	db *gorm.DB
}

func NewAppVersionHandler(db *gorm.DB) *AppVersionHandler {
	return &AppVersionHandler{db: db}
}

func (h *AppVersionHandler) RegisterPublicRoutes(group *gin.RouterGroup) {
	group.GET("/app/version-check", h.handleCheckVersion)
}

type appVersionCheckResponse struct {
	Platform string `json:"platform"`
	// CurrentVersion echoes what the client reported, so a mismatched or
	// truncated value is visible in the logs instead of silently skewing the
	// comparison.
	CurrentVersion      string `json:"current_version"`
	MinSupportedVersion string `json:"min_supported_version"`
	LatestVersion       string `json:"latest_version"`
	// UpdateRequired means "a newer release exists".
	UpdateRequired bool `json:"update_required"`
	// UpdateAllowed means "this build is still supported". False is the case
	// that must block: the client is below the supported floor.
	UpdateAllowed bool `json:"update_allowed"`
	// PolicyConfigured distinguishes "no policy for this platform" from "you are
	// current". A client must not treat missing policy as a requirement to
	// update, or a typo in the platform name would lock everyone out.
	PolicyConfigured bool   `json:"policy_configured"`
	ForceUpdate      bool   `json:"force_update"`
	Message          string `json:"message"`
}

// handleCheckVersion compares the caller's build against the stored policy.
//
// Query parameters:
//   - platform: ios | android (required)
//   - version:  the caller's version string (required)
func (h *AppVersionHandler) handleCheckVersion(c *gin.Context) {
	platform := strings.ToLower(strings.TrimSpace(c.Query("platform")))
	version := strings.TrimSpace(c.Query("version"))

	if platform != "ios" && platform != "android" {
		JSONError(c, http.StatusBadRequest, "platform must be ios or android")
		return
	}
	if version == "" {
		JSONError(c, http.StatusBadRequest, "version is required")
		return
	}

	response := appVersionCheckResponse{
		Platform:         platform,
		CurrentVersion:   version,
		PolicyConfigured: false,
		// Absent a policy, allow the client to keep running. Failing closed here
		// would mean a typo in a database row takes down every installed app.
		UpdateAllowed:       true,
		MinSupportedVersion: "",
		LatestVersion:       "",
	}

	if h.db == nil {
		// No database configured (tests, migration job): report no policy rather
		// than an error, so a version check can never be the thing that breaks
		// app startup.
		JSONSuccess(c, http.StatusOK, response)
		return
	}

	var policy models.AppVersionPolicy
	if err := h.db.Where("platform = ?", platform).First(&policy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			JSONSuccess(c, http.StatusOK, response)
			return
		}
		JSONError(c, http.StatusInternalServerError, "failed to read the app version policy")
		return
	}

	response.PolicyConfigured = true
	response.MinSupportedVersion = policy.MinSupportedVersion
	response.LatestVersion = policy.LatestVersion
	response.Message = policy.Message
	response.ForceUpdate = policy.ForceUpdate

	// A malformed floor must not lock out every client. Treat an unparseable
	// value as "no floor" and let the comparison against LatestVersion stand.
	if policy.MinSupportedVersion != "" {
		if cmp := compareVersions(version, policy.MinSupportedVersion); cmp >= 0 {
			response.UpdateAllowed = true
		} else {
			response.UpdateAllowed = false
		}
	}

	if policy.LatestVersion != "" && compareVersions(version, policy.LatestVersion) < 0 {
		response.UpdateRequired = true
	}

	JSONSuccess(c, http.StatusOK, response)
}

// compareVersions compares dotted numeric versions, returning -1, 0 or 1.
//
// Deliberately not a full semver implementation: build metadata ("1.2.3+42")
// and pre-release tags are stripped rather than parsed, because a client
// sending something exotic should be treated as comparable, not rejected - a
// version gate that errors is a gate that locks everyone out of their own app.
func compareVersions(a, b string) int {
	partsA := versionParts(a)
	partsB := versionParts(b)
	for i := 0; i < 3; i++ {
		if partsA[i] != partsB[i] {
			if partsA[i] < partsB[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(value string) [3]int {
	var out [3]int
	// Drop build metadata and pre-release suffixes: "1.2.3+42" and "1.2.3-rc1"
	// both become 1.2.3.
	trimmed := strings.TrimSpace(value)
	if idx := strings.IndexAny(trimmed, "+-"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	for i, segment := range strings.SplitN(trimmed, ".", 3) {
		if i > 2 {
			break
		}
		// strconv.Atoi rejects "1a"; taking the leading digits keeps a sloppy
		// value comparable instead of failing the whole check.
		digits := strings.TrimFunc(segment, func(r rune) bool { return r < '0' || r > '9' })
		n, err := strconv.Atoi(digits)
		if err != nil {
			continue
		}
		out[i] = n
	}
	return out
}
