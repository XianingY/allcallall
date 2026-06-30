package commerce

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

// SupportReportRecord is an abuse report enriched with user emails for support review.
type SupportReportRecord struct {
	Report        models.AbuseReport `json:"report"`
	ReporterEmail string             `json:"reporter_email"`
	ReportedEmail string             `json:"reported_email"`
	ReporterName  string             `json:"reporter_name"`
	ReportedName  string             `json:"reported_name"`
}

// SupportUserSummary provides a comprehensive view of a user for support diagnostics.
type SupportUserSummary struct {
	User            models.User                  `json:"user"`
	Entitlements    []models.UserEntitlement     `json:"entitlements"`
	Usage           []UsageSnapshot              `json:"usage"`
	RecentCalls     []CallHistoryEntry           `json:"recent_calls"`
	Blocks          []models.UserBlock           `json:"blocks"`
	Reports         []models.AbuseReport         `json:"reports"`
	RefreshSessions SupportRefreshSessionSummary `json:"refresh_sessions"`
	DeletionAudit   *models.DeletionAudit        `json:"deletion_audit,omitempty"`
}

// SupportRefreshSessionRecord is a single refresh session for support display.
type SupportRefreshSessionRecord struct {
	ID               uint64     `json:"id"`
	UserAgent        string     `json:"user_agent"`
	IPAddress        string     `json:"ip_address"`
	ExpiresAt        time.Time  `json:"expires_at"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	RevokedAt        *time.Time `json:"revoked_at,omitempty"`
	InvalidUseCount  int        `json:"invalid_use_count"`
	LastInvalidUseAt *time.Time `json:"last_invalid_use_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// SupportRefreshSessionSummary aggregates refresh session metrics for support review.
type SupportRefreshSessionSummary struct {
	ActiveCount      int64                         `json:"active_count"`
	RevokedCount     int64                         `json:"revoked_count"`
	ExpiredCount     int64                         `json:"expired_count"`
	InvalidUseCount  int64                         `json:"invalid_use_count"`
	LastInvalidUseAt *time.Time                    `json:"last_invalid_use_at,omitempty"`
	RiskLevel        string                        `json:"risk_level"`
	RiskReasons      []string                      `json:"risk_reasons"`
	Recent           []SupportRefreshSessionRecord `json:"recent"`
}

// SupportRefreshSessionRevocation is the result of revoking refresh sessions.
type SupportRefreshSessionRevocation struct {
	UserID          uint64  `json:"user_id"`
	SessionID       *uint64 `json:"session_id,omitempty"`
	RevokedSessions int64   `json:"revoked_sessions"`
}

// SupportCallDetails is a complete call record for support diagnostics.
type SupportCallDetails struct {
	Call               models.CallSession             `json:"call"`
	TranslationSlices  []models.TranslationUsageSlice `json:"translation_slices"`
	TranscriptSegments []models.CallTranscriptSegment `json:"transcript_segments"`
	Followup           *models.CallFollowup           `json:"followup,omitempty"`
	Tasks              []models.FollowUpTask          `json:"tasks"`
}

// SupportService aggregates data across sub-services for support diagnostics.
type SupportService struct {
	repo         *Repository
	entitlements *EntitlementService
	callHistory  *CallHistoryService
	blockAbuse   *BlockAbuseService
}

// NewSupportService creates a new SupportService.
func NewSupportService(repo *Repository, entitlements *EntitlementService, callHistory *CallHistoryService, blockAbuse *BlockAbuseService) *SupportService {
	return &SupportService{
		repo:         repo,
		entitlements: entitlements,
		callHistory:  callHistory,
		blockAbuse:   blockAbuse,
	}
}

// ListSupportReports returns abuse reports enriched with reporter/reported user info.
func (s *SupportService) ListSupportReports(ctx context.Context, limit int) ([]SupportReportRecord, error) {
	if limit <= 0 {
		limit = 50
	}

	reports, err := s.repo.ListAbuseReports(ctx, limit)
	if err != nil {
		return nil, err
	}
	if len(reports) == 0 {
		return []SupportReportRecord{}, nil
	}

	userIDs := make([]uint64, 0, len(reports)*2)
	seen := make(map[uint64]struct{})
	for _, report := range reports {
		if _, ok := seen[report.ReporterID]; !ok {
			userIDs = append(userIDs, report.ReporterID)
			seen[report.ReporterID] = struct{}{}
		}
		if _, ok := seen[report.ReportedUserID]; !ok {
			userIDs = append(userIDs, report.ReportedUserID)
			seen[report.ReportedUserID] = struct{}{}
		}
	}

	users, err := s.repo.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	userMap := make(map[uint64]models.User, len(users))
	for _, item := range users {
		userMap[item.ID] = item
	}

	result := make([]SupportReportRecord, 0, len(reports))
	for _, report := range reports {
		reporter := userMap[report.ReporterID]
		reported := userMap[report.ReportedUserID]
		result = append(result, SupportReportRecord{
			Report:        report,
			ReporterEmail: reporter.Email,
			ReportedEmail: reported.Email,
			ReporterName:  reporter.DisplayName,
			ReportedName:  reported.DisplayName,
		})
	}
	return result, nil
}

// GetSupportUserSummary returns a comprehensive user summary for support review.
func (s *SupportService) GetSupportUserSummary(ctx context.Context, userID uint64) (*SupportUserSummary, error) {
	userModel, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	entitlements, err := s.entitlements.GetEntitlements(ctx, userID)
	if err != nil {
		return nil, err
	}
	usage, err := s.entitlements.GetUsage(ctx, userID)
	if err != nil {
		return nil, err
	}
	calls, err := s.callHistory.ListCallHistory(ctx, userID, 365)
	if err != nil {
		return nil, err
	}
	blocks, err := s.blockAbuse.ListBlocks(ctx, userID)
	if err != nil {
		return nil, err
	}
	reports, err := s.repo.ListAbuseReportsByUser(ctx, userID, 50)
	if err != nil {
		return nil, err
	}
	refreshSessions, err := s.getSupportRefreshSessionSummary(ctx, userID)
	if err != nil {
		return nil, err
	}
	deletionAuditPtr, err := s.repo.GetLatestDeletionAudit(ctx, userID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	return &SupportUserSummary{
		User:            *userModel,
		Entitlements:    entitlements,
		Usage:           usage,
		RecentCalls:     calls,
		Blocks:          blocks,
		Reports:         reports,
		RefreshSessions: refreshSessions,
		DeletionAudit:   deletionAuditPtr,
	}, nil
}

func (s *SupportService) getSupportRefreshSessionSummary(ctx context.Context, userID uint64) (SupportRefreshSessionSummary, error) {
	now := time.Now().UTC()
	var summary SupportRefreshSessionSummary

	activeCount, err := s.repo.CountActiveRefreshSessions(ctx, userID, now)
	if err != nil {
		return summary, err
	}
	summary.ActiveCount = activeCount

	revokedCount, err := s.repo.CountRevokedRefreshSessions(ctx, userID)
	if err != nil {
		return summary, err
	}
	summary.RevokedCount = revokedCount

	expiredCount, err := s.repo.CountExpiredRefreshSessions(ctx, userID, now)
	if err != nil {
		return summary, err
	}
	summary.ExpiredCount = expiredCount

	invalidUseCount, err := s.repo.SumInvalidUseCountRefreshSessions(ctx, userID)
	if err != nil {
		return summary, err
	}
	summary.InvalidUseCount = invalidUseCount

	latestInvalid, err := s.repo.GetLatestInvalidRefreshSession(ctx, userID)
	if err == nil {
		summary.LastInvalidUseAt = latestInvalid.LastInvalidUseAt
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return summary, err
	}

	recent, err := s.repo.ListRecentRefreshSessions(ctx, userID, 10)
	if err != nil {
		return summary, err
	}
	summary.Recent = make([]SupportRefreshSessionRecord, 0, len(recent))
	for _, item := range recent {
		summary.Recent = append(summary.Recent, SupportRefreshSessionRecord{
			ID:               item.ID,
			UserAgent:        item.UserAgent,
			IPAddress:        item.IPAddress,
			ExpiresAt:        item.ExpiresAt,
			LastUsedAt:       item.LastUsedAt,
			RevokedAt:        item.RevokedAt,
			InvalidUseCount:  item.InvalidUseCount,
			LastInvalidUseAt: item.LastInvalidUseAt,
			CreatedAt:        item.CreatedAt,
			UpdatedAt:        item.UpdatedAt,
		})
	}
	summary.RiskLevel, summary.RiskReasons = supportRefreshSessionRisk(summary, now)

	return summary, nil
}

func supportRefreshSessionRisk(summary SupportRefreshSessionSummary, now time.Time) (string, []string) {
	level := "none"
	reasons := []string{}
	if summary.ActiveCount >= 5 {
		level = "low"
		reasons = append(reasons, "many_active_sessions")
	}
	if summary.InvalidUseCount > 0 {
		level = "medium"
		reasons = append(reasons, "refresh_token_reuse_detected")
	}
	if summary.InvalidUseCount >= 3 {
		level = "high"
		reasons = append(reasons, "repeated_refresh_token_reuse")
	}
	if summary.LastInvalidUseAt != nil && summary.LastInvalidUseAt.After(now.Add(-24*time.Hour)) {
		level = "high"
		reasons = append(reasons, "recent_refresh_token_reuse")
	}
	return level, reasons
}

// RevokeSupportRefreshSessions revokes refresh sessions for a user.
func (s *SupportService) RevokeSupportRefreshSessions(ctx context.Context, userID uint64, sessionID *uint64) (*SupportRefreshSessionRevocation, error) {
	if userID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	if sessionID != nil && *sessionID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	now := time.Now().UTC()
	revokedSessions, err := s.repo.RevokeRefreshSessions(ctx, userID, sessionID, now)
	if err != nil {
		return nil, err
	}
	return &SupportRefreshSessionRevocation{
		UserID:          userID,
		SessionID:       sessionID,
		RevokedSessions: revokedSessions,
	}, nil
}

// GetSupportCall returns full call details for support diagnostics.
func (s *SupportService) GetSupportCall(ctx context.Context, callID string) (*SupportCallDetails, error) {
	call, err := s.repo.GetCallSession(ctx, strings.TrimSpace(callID))
	if err != nil {
		return nil, err
	}
	slices, err := s.repo.GetTranslationUsageSlicesByCall(ctx, strings.TrimSpace(callID))
	if err != nil {
		return nil, err
	}
	transcripts, err := s.repo.GetTranscriptSegmentsByCall(ctx, strings.TrimSpace(callID))
	if err != nil {
		return nil, err
	}
	followupPtr, err := s.repo.GetCallFollowupByCall(ctx, strings.TrimSpace(callID))
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	var tasks []models.FollowUpTask
	if err := s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		var taskErr error
		tasks, taskErr = s.repo.GetFollowUpTasksByCalls(ctx, []string{strings.TrimSpace(callID)}, 0, nil)
		return taskErr
	}); err != nil {
		return nil, err
	}
	return &SupportCallDetails{
		Call:               *call,
		TranslationSlices:  slices,
		TranscriptSegments: transcripts,
		Followup:           followupPtr,
		Tasks:              tasks,
	}, nil
}
