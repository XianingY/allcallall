package commerce

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

const (
	translationFreeSecondsPerMonth = int64(1800)
	translationSliceSeconds        = int64(30)
	translationSliceMilliseconds   = translationSliceSeconds * 1000

	translationUsageFeature = "translation_seconds"

	premiumMonthlyProductID = "premium_monthly"
	premiumYearlyProductID  = "premium_yearly"

	legalTermsVersion   = "2026-04-11"
	legalPrivacyVersion = "2026-04-11"

	revenueCatSource = "revenuecat"
)

var (
	ErrUserBlocked               = errors.New("user is blocked")
	ErrSubscriptionRequired      = errors.New("premium subscription required")
	ErrTranslationQuotaExhausted = errors.New("translation quota exhausted")
	ErrWebhookAlreadyProcessed   = errors.New("billing webhook already processed")
	ErrInvalidReportCategory     = errors.New("invalid abuse report category")
	ErrSupportTokenNotConfigured = errors.New("support token is not configured")
	ErrFollowupNotFound          = errors.New("call follow-up not found")
)

type Service struct {
	repo    *Repository
	metrics counterRecorder
}

type counterRecorder interface {
	Inc(name string)
	Add(name string, delta int64)
}

func NewService(db *gorm.DB, metrics ...counterRecorder) *Service {
	var recorder counterRecorder
	if len(metrics) > 0 {
		recorder = metrics[0]
	}
	return &Service{repo: NewRepository(db), metrics: recorder}
}

func NewServiceWithRepository(repo *Repository, metrics ...counterRecorder) *Service {
	var recorder counterRecorder
	if len(metrics) > 0 {
		recorder = metrics[0]
	}
	return &Service{repo: repo, metrics: recorder}
}

var allowedReportCategories = map[string]struct{}{
	"spam":           {},
	"harassment":     {},
	"impersonation":  {},
	"fraud":          {},
	"sexual_content": {},
	"other":          {},
}

var allowedFollowUpTaskTypes = map[string]struct{}{
	models.FollowupTaskTypeCallback:         {},
	models.FollowupTaskTypeSendMessage:      {},
	models.FollowupTaskTypeScheduleNextCall: {},
}

var allowedFollowUpTaskStatuses = map[string]struct{}{
	models.FollowupTaskStatusOpen:      {},
	models.FollowupTaskStatusDone:      {},
	models.FollowupTaskStatusSnoozed:   {},
	models.FollowupTaskStatusCancelled: {},
}

type LegalDocumentSet struct {
	TermsVersion       string `json:"terms_version"`
	PrivacyVersion     string `json:"privacy_version"`
	TermsURL           string `json:"terms_url"`
	PrivacyPolicyURL   string `json:"privacy_policy_url"`
	SupportEmail       string `json:"support_email"`
	AccountDeletionURL string `json:"account_deletion_url"`
}

func (s *Service) CurrentLegal() LegalDocumentSet {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("PUBLIC_WEB_BASE_URL")), "/")
	if baseURL == "" {
		baseURL = "https://allcallall.app"
	}
	supportEmail := strings.TrimSpace(os.Getenv("SUPPORT_EMAIL"))
	if supportEmail == "" {
		supportEmail = "support@allcallall.app"
	}
	return LegalDocumentSet{
		TermsVersion:       legalTermsVersion,
		PrivacyVersion:     legalPrivacyVersion,
		TermsURL:           baseURL + "/legal/terms",
		PrivacyPolicyURL:   baseURL + "/legal/privacy",
		SupportEmail:       supportEmail,
		AccountDeletionURL: baseURL + "/legal/delete-account",
	}
}

func (s *Service) AcceptLegal(ctx context.Context, userID uint64) error {
	return s.repo.UpsertLegalAcceptance(ctx, userID, legalTermsVersion, legalPrivacyVersion)
}

func (s *Service) GetLegalAcceptance(ctx context.Context, userID uint64) (*models.LegalAcceptance, error) {
	return s.repo.GetLegalAcceptance(ctx, userID)
}

func (s *Service) EnsureDefaultEntitlement(ctx context.Context, userID uint64) (*models.UserEntitlement, error) {
	now := time.Now().UTC()
	entitlement, err := s.repo.GetActivePremiumEntitlement(ctx, userID)
	if err == nil {
		return entitlement, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	entitlement = &models.UserEntitlement{
		UserID:       userID,
		Entitlement:  models.EntitlementFree,
		Tier:         models.EntitlementFree,
		Source:       "system",
		Status:       "active",
		LastSyncedAt: &now,
	}

	if err := s.repo.FirstOrCreateFreeEntitlement(ctx, entitlement); err != nil {
		return nil, err
	}
	return entitlement, nil
}

func (s *Service) GetEntitlements(ctx context.Context, userID uint64) ([]models.UserEntitlement, error) {
	entitlements, err := s.repo.GetEntitlements(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(entitlements) == 0 {
		entitlement, err := s.EnsureDefaultEntitlement(ctx, userID)
		if err != nil {
			return nil, err
		}
		return []models.UserEntitlement{*entitlement}, nil
	}
	return entitlements, nil
}

func (s *Service) ActiveTier(ctx context.Context, userID uint64) (string, error) {
	entitlements, err := s.GetEntitlements(ctx, userID)
	if err != nil {
		return "", err
	}
	for _, item := range entitlements {
		if item.Status != "active" {
			continue
		}
		if item.Entitlement == models.EntitlementPremium {
			if item.ExpiresAt == nil || item.ExpiresAt.After(time.Now().UTC()) {
				return models.EntitlementPremium, nil
			}
		}
	}
	return models.EntitlementFree, nil
}

type UsageSnapshot struct {
	Feature        string `json:"feature"`
	PeriodKey      string `json:"period_key"`
	Unit           string `json:"unit"`
	UsedUnits      int64  `json:"used_units"`
	LimitUnits     int64  `json:"limit_units"`
	Unlimited      bool   `json:"unlimited"`
	RemainingUnits int64  `json:"remaining_units"`
}

type CallHistoryEntry struct {
	models.CallSession
	FollowupStatus string     `json:"followup_status,omitempty"`
	NextTaskDueAt  *time.Time `json:"next_task_due_at,omitempty"`
	IsOverdue      bool       `json:"is_overdue"`
}

type FollowupResponse struct {
	Followup *models.CallFollowup  `json:"followup"`
	Tasks    []models.FollowUpTask `json:"tasks"`
}

type FollowUpListItem struct {
	Task      models.FollowUpTask    `json:"task"`
	Call      *models.CallSession    `json:"call,omitempty"`
	Followup  *models.CallFollowup   `json:"followup,omitempty"`
	Peer      *models.User           `json:"peer,omitempty"`
	Contact   *models.ContactProfile `json:"contact,omitempty"`
	IsOverdue bool                   `json:"is_overdue"`
}

func periodKey(now time.Time) string {
	return now.UTC().Format("2006-01")
}

func reportCategoryList() []string {
	return []string{
		"spam",
		"harassment",
		"impersonation",
		"fraud",
		"sexual_content",
		"other",
	}
}

func normalizeReportCategory(category string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(category))
	if _, ok := allowedReportCategories[normalized]; !ok {
		return "", ErrInvalidReportCategory
	}
	return normalized, nil
}

func normalizeFollowUpTaskType(taskType string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(taskType))
	if _, ok := allowedFollowUpTaskTypes[normalized]; !ok {
		return "", errors.New("invalid follow-up task type")
	}
	return normalized, nil
}

func normalizeFollowUpTaskStatus(status string) (string, error) {
	normalized := strings.TrimSpace(strings.ToLower(status))
	if _, ok := allowedFollowUpTaskStatuses[normalized]; !ok {
		return "", errors.New("invalid follow-up task status")
	}
	return normalized, nil
}

func (s *Service) lookupUsageLedgerUnits(ctx context.Context, userID uint64, key string) (int64, error) {
	ledger, err := s.repo.GetUsageLedger(ctx, userID, translationUsageFeature, key)
	if err == nil {
		return ledger.Units, nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, err
	}

	legacy, legacyErr := s.repo.GetUsageLedger(ctx, userID, "translation_minutes", key)
	if legacyErr == nil {
		return legacy.Units * 60, nil
	}
	if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
		return 0, legacyErr
	}
	return 0, nil
}

func (s *Service) GetUsage(ctx context.Context, userID uint64) ([]UsageSnapshot, error) {
	tier, err := s.ActiveTier(ctx, userID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	key := periodKey(now)
	usedUnits, err := s.lookupUsageLedgerUnits(ctx, userID, key)
	if err != nil {
		return nil, err
	}

	limit := translationFreeSecondsPerMonth
	unlimited := tier == models.EntitlementPremium
	if unlimited {
		limit = 0
	}
	remaining := limit - usedUnits
	if remaining < 0 {
		remaining = 0
	}
	return []UsageSnapshot{
		{
			Feature:        translationUsageFeature,
			PeriodKey:      key,
			Unit:           "seconds",
			UsedUnits:      usedUnits,
			LimitUnits:     limit,
			Unlimited:      unlimited,
			RemainingUnits: remaining,
		},
	}, nil
}

func (s *Service) consumeTranslationSecondsTx(ctx context.Context, userID uint64, deltaSeconds int64, key string) error {
	if deltaSeconds <= 0 {
		return nil
	}

	ledger, err := s.repo.GetUsageLedger(ctx, userID, translationUsageFeature, key)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		legacyUnits := int64(0)
		legacy, legacyErr := s.repo.GetUsageLedger(ctx, userID, "translation_minutes", key)
		if legacyErr == nil {
			legacyUnits = legacy.Units * 60
		} else if legacyErr != nil && !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
			return legacyErr
		}
		ledger = &models.UsageLedger{
			UserID:    userID,
			Feature:   translationUsageFeature,
			PeriodKey: key,
			Units:     legacyUnits,
		}
		if err := s.repo.FirstOrCreateUsageLedger(ctx, ledger); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	if ledger.Units+deltaSeconds > translationFreeSecondsPerMonth {
		return ErrTranslationQuotaExhausted
	}
	ledger.Units += deltaSeconds
	return s.repo.SaveUsageLedger(ctx, ledger)
}

func (s *Service) ConsumeTranslationMinutes(ctx context.Context, userID uint64, delta int64) error {
	return s.ConsumeTranslationSeconds(ctx, userID, delta*60)
}

func (s *Service) ConsumeTranslationSeconds(ctx context.Context, userID uint64, deltaSeconds int64) error {
	if deltaSeconds <= 0 {
		return nil
	}
	tier, err := s.ActiveTier(ctx, userID)
	if err != nil {
		return err
	}
	if tier == models.EntitlementPremium {
		return nil
	}

	key := periodKey(time.Now().UTC())
	return s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		return s.consumeTranslationSecondsTx(ctx, userID, deltaSeconds, key)
	})
}

func (s *Service) RecordTranslationUsageSlice(ctx context.Context, userID uint64, callID string, eventTimestampMS int64) (bool, error) {
	if userID == 0 || strings.TrimSpace(callID) == "" {
		return false, errors.New("user_id and call_id are required")
	}
	if eventTimestampMS <= 0 {
		eventTimestampMS = time.Now().UnixMilli()
	}

	tier, err := s.ActiveTier(ctx, userID)
	if err != nil {
		return false, err
	}

	callID = strings.TrimSpace(callID)
	sliceIndex := eventTimestampMS / translationSliceMilliseconds
	key := periodKey(time.UnixMilli(eventTimestampMS).UTC())
	charged := false

	err = s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		slice := &models.TranslationUsageSlice{
			UserID:           userID,
			CallID:           callID,
			SliceIndex:       sliceIndex,
			EventTimestampMS: eventTimestampMS,
			DurationSeconds:  translationSliceSeconds,
			Tier:             tier,
		}

		rowsAffected, createErr := s.repo.FirstOrCreateTranslationUsageSlice(ctx, slice)
		if createErr != nil {
			return createErr
		}
		if rowsAffected == 0 {
			return nil
		}

		charged = true
		if tier == models.EntitlementPremium {
			return nil
		}

		return s.consumeTranslationSecondsTx(ctx, userID, translationSliceSeconds, key)
	})
	if err != nil {
		return false, err
	}
	return charged, nil
}

func (s *Service) RegisterCallInvite(ctx context.Context, callID string, caller *models.User, callee *models.User) error {
	now := time.Now().UTC()
	record := &models.CallSession{
		CallID:            callID,
		CallerID:          caller.ID,
		CalleeID:          callee.ID,
		CallerEmail:       caller.Email,
		CalleeEmail:       callee.Email,
		CallerDisplayName: caller.DisplayName,
		CalleeDisplayName: callee.DisplayName,
		Status:            models.CallStatusInvited,
		StartedAt:         now,
		LastEventAt:       now,
	}

	return s.repo.RegisterCallInvite(ctx, record)
}

func (s *Service) RecordTranscriptSegment(ctx context.Context, segment models.CallTranscriptSegment) error {
	if strings.TrimSpace(segment.CallID) == "" || segment.UserID == 0 || segment.PeerUserID == 0 {
		return errors.New("call transcript segment requires call and user ids")
	}
	segment.CallID = strings.TrimSpace(segment.CallID)
	segment.FromEmail = strings.TrimSpace(strings.ToLower(segment.FromEmail))
	segment.ToEmail = strings.TrimSpace(strings.ToLower(segment.ToEmail))
	if segment.TimestampMS <= 0 {
		segment.TimestampMS = time.Now().UnixMilli()
	}
	return s.repo.CreateTranscriptSegment(ctx, &segment)
}

func (s *Service) MarkFollowupSecondCallCompleted(ctx context.Context, userID, peerUserID uint64, callID string, completedAt time.Time) error {
	if userID == 0 || peerUserID == 0 {
		return nil
	}
	callID = strings.TrimSpace(callID)
	windowStart := completedAt.Add(-7 * 24 * time.Hour)
	return s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		count, err := s.repo.CountRecentCallsBetweenUsers(ctx, userID, peerUserID, windowStart, callID)
		if err != nil {
			return err
		}
		if count == 0 {
			return nil
		}
		return s.repo.UpdateFollowUpTasksByUserPeerType(ctx, userID, peerUserID, models.FollowupTaskTypeCallback, models.FollowupTaskStatusOpen, map[string]any{
			"status":       models.FollowupTaskStatusDone,
			"completed_at": completedAt.UTC(),
			"updated_at":   completedAt.UTC(),
		})
	})
}

func (s *Service) UpdateCallStatus(ctx context.Context, callID string, status string, endReason string) error {
	now := time.Now().UTC()
	updates := map[string]any{
		"status":        status,
		"last_event_at": now,
		"updated_at":    now,
	}
	if endReason != "" {
		updates["end_reason"] = endReason
	}
	switch status {
	case models.CallStatusAnswered:
		updates["answered_at"] = now
	case models.CallStatusEnded, models.CallStatusRejected, models.CallStatusMissed, models.CallStatusFailed:
		updates["ended_at"] = now
	}
	if err := s.repo.UpdateCallStatus(ctx, callID, updates); err != nil {
		return err
	}
	if status == models.CallStatusEnded || status == models.CallStatusRejected || status == models.CallStatusMissed || status == models.CallStatusFailed {
		return s.GenerateFollowupForCall(ctx, callID, false)
	}
	return nil
}

func (s *Service) ListCallHistory(ctx context.Context, userID uint64, days int) ([]CallHistoryEntry, error) {
	if days <= 0 {
		days = 30
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	sessions, err := s.repo.ListCallSessionsByUser(ctx, userID, since)
	if err != nil || len(sessions) == 0 {
		return []CallHistoryEntry{}, err
	}

	callIDs := make([]string, 0, len(sessions))
	for _, session := range sessions {
		callIDs = append(callIDs, session.CallID)
	}
	followups, err := s.repo.GetCallFollowupsByCalls(ctx, callIDs, userID)
	if err != nil {
		return nil, err
	}
	followupMap := make(map[string]models.CallFollowup, len(followups))
	for _, item := range followups {
		followupMap[item.CallID] = item
	}
	tasks, err := s.repo.GetFollowUpTasksByCalls(ctx, callIDs, userID, []string{models.FollowupTaskStatusOpen, models.FollowupTaskStatusSnoozed})
	if err != nil {
		return nil, err
	}
	taskMap := make(map[string]models.FollowUpTask, len(tasks))
	for _, item := range tasks {
		if existing, ok := taskMap[item.CallID]; ok && existing.DueAt != nil && item.DueAt != nil && existing.DueAt.Before(*item.DueAt) {
			continue
		}
		taskMap[item.CallID] = item
	}

	now := time.Now().UTC()
	result := make([]CallHistoryEntry, 0, len(sessions))
	for _, session := range sessions {
		entry := CallHistoryEntry{CallSession: session}
		if followup, ok := followupMap[session.CallID]; ok {
			entry.FollowupStatus = followup.Status
		}
		if task, ok := taskMap[session.CallID]; ok {
			entry.NextTaskDueAt = task.DueAt
			entry.IsOverdue = task.DueAt != nil && task.Status == models.FollowupTaskStatusOpen && task.DueAt.Before(now)
			if entry.FollowupStatus == "" {
				entry.FollowupStatus = task.Status
			}
		}
		result = append(result, entry)
	}
	return result, nil
}

func (s *Service) CreateBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	block := &models.UserBlock{
		BlockerID:     blockerID,
		BlockedUserID: blockedUserID,
	}
	return s.repo.FirstOrCreateUserBlock(ctx, block)
}

func (s *Service) RemoveBlock(ctx context.Context, blockerID, blockedUserID uint64) error {
	return s.repo.DeleteUserBlock(ctx, blockerID, blockedUserID)
}

func (s *Service) ListBlocks(ctx context.Context, blockerID uint64) ([]models.UserBlock, error) {
	return s.repo.ListUserBlocks(ctx, blockerID)
}

func (s *Service) AreUsersBlocked(ctx context.Context, userA, userB uint64) (bool, error) {
	count, err := s.repo.CountUserBlocksBetweenUsers(ctx, userA, userB)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Service) CreateReport(ctx context.Context, reporterID, reportedUserID uint64, category, details string) error {
	normalizedCategory, err := normalizeReportCategory(category)
	if err != nil {
		return err
	}
	report := &models.AbuseReport{
		ReporterID:     reporterID,
		ReportedUserID: reportedUserID,
		Category:       normalizedCategory,
		Details:        strings.TrimSpace(details),
		Status:         "open",
	}
	return s.repo.CreateAbuseReport(ctx, report)
}

type RevenueCatWebhook struct {
	Event struct {
		ID                 string   `json:"id"`
		Type               string   `json:"type"`
		AppUserID          string   `json:"app_user_id"`
		ProductID          string   `json:"product_id"`
		EntitlementIDs     []string `json:"entitlement_ids"`
		CancelReason       string   `json:"cancel_reason"`
		ExpirationReason   string   `json:"expiration_reason"`
		NewProductID       string   `json:"new_product_id"`
		ExpirationAtMillis int64    `json:"expiration_at_ms"`
	} `json:"event"`
}

func (s *Service) HandleRevenueCatWebhook(ctx context.Context, payload RevenueCatWebhook, raw []byte) error {
	eventID := strings.TrimSpace(payload.Event.ID)
	if eventID == "" {
		return errors.New("billing event id is required")
	}

	now := time.Now().UTC()
	eventRecord := &models.BillingWebhookEvent{
		EventID:     eventID,
		AppUserID:   payload.Event.AppUserID,
		EventType:   payload.Event.Type,
		PayloadJSON: string(raw),
	}

	return s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		existingEvent, err := s.repo.GetBillingWebhookEvent(ctx, eventID)
		if err == nil {
			return ErrWebhookAlreadyProcessed
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		_ = existingEvent

		if err := s.repo.CreateBillingWebhookEvent(ctx, eventRecord); err != nil {
			return err
		}

		userID, parseErr := parseAppUserID(payload.Event.AppUserID)
		if parseErr != nil {
			return parseErr
		}

		entitlementTier := models.EntitlementFree
		entitlementName := models.EntitlementFree
		status := "inactive"
		source := revenueCatSource
		var expiresAt *time.Time

		productID := strings.TrimSpace(payload.Event.ProductID)
		switch productID {
		case premiumMonthlyProductID, premiumYearlyProductID:
			entitlementTier = models.EntitlementPremium
			entitlementName = models.EntitlementPremium
		default:
			productID = ""
		}

		switch strings.TrimSpace(payload.Event.Type) {
		case "INITIAL_PURCHASE", "RENEWAL", "UNCANCELLATION", "NON_RENEWING_PURCHASE", "TRANSFER", "SUBSCRIPTION_EXTENDED", "TEMPORARY_ENTITLEMENT_GRANT":
			status = "active"
		case "EXPIRATION":
			status = "inactive"
		case "BILLING_ISSUE", "SUBSCRIPTION_PAUSED":
			status = "active"
		case "CANCELLATION":
			if strings.TrimSpace(payload.Event.CancelReason) == "CUSTOMER_SUPPORT" {
				status = "inactive"
			} else if payload.Event.ExpirationAtMillis > now.UnixMilli() {
				status = "active"
			} else {
				status = "inactive"
			}
		case "PRODUCT_CHANGE":
			status = "active"
			if nextProductID := strings.TrimSpace(payload.Event.NewProductID); nextProductID != "" {
				switch nextProductID {
				case premiumMonthlyProductID, premiumYearlyProductID:
					productID = nextProductID
				}
			}
		case "SUBSCRIBER_ALIAS":
			status = ""
		default:
			// Unknown events still get stored for support, but do not mutate entitlements.
		}

		if payload.Event.ExpirationAtMillis > 0 {
			t := time.UnixMilli(payload.Event.ExpirationAtMillis).UTC()
			expiresAt = &t
			if status == "inactive" && t.After(now) && payload.Event.Type == "CANCELLATION" {
				status = "active"
			}
		}

		if entitlementName != models.EntitlementFree && status != "" {
			existing, err := s.repo.GetEntitlementByType(ctx, userID, entitlementName)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				existing = &models.UserEntitlement{
					UserID:      userID,
					Entitlement: entitlementName,
				}
			} else if err != nil {
				return err
			}
			existing.Tier = entitlementTier
			existing.ProductID = productID
			existing.Source = source
			existing.Status = status
			existing.ExpiresAt = expiresAt
			existing.LastSyncedAt = &now
			if err := s.repo.SaveEntitlement(ctx, existing); err != nil {
				return err
			}
		}

		eventRecord.ProcessedAt = &now
		return s.repo.SaveBillingWebhookEvent(ctx, eventRecord)
	})
}

type SupportReportRecord struct {
	Report        models.AbuseReport `json:"report"`
	ReporterEmail string             `json:"reporter_email"`
	ReportedEmail string             `json:"reported_email"`
	ReporterName  string             `json:"reporter_name"`
	ReportedName  string             `json:"reported_name"`
}

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

type SupportRefreshSessionRevocation struct {
	UserID          uint64  `json:"user_id"`
	SessionID       *uint64 `json:"session_id,omitempty"`
	RevokedSessions int64   `json:"revoked_sessions"`
}

type SupportCallDetails struct {
	Call               models.CallSession             `json:"call"`
	TranslationSlices  []models.TranslationUsageSlice `json:"translation_slices"`
	TranscriptSegments []models.CallTranscriptSegment `json:"transcript_segments"`
	Followup           *models.CallFollowup           `json:"followup,omitempty"`
	Tasks              []models.FollowUpTask          `json:"tasks"`
}

func (s *Service) ListSupportReports(ctx context.Context, limit int) ([]SupportReportRecord, error) {
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

func (s *Service) GetSupportUserSummary(ctx context.Context, userID uint64) (*SupportUserSummary, error) {
	userModel, err := s.repo.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	entitlements, err := s.GetEntitlements(ctx, userID)
	if err != nil {
		return nil, err
	}
	usage, err := s.GetUsage(ctx, userID)
	if err != nil {
		return nil, err
	}
	calls, err := s.ListCallHistory(ctx, userID, 365)
	if err != nil {
		return nil, err
	}
	blocks, err := s.ListBlocks(ctx, userID)
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

func (s *Service) getSupportRefreshSessionSummary(ctx context.Context, userID uint64) (SupportRefreshSessionSummary, error) {
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

func (s *Service) RevokeSupportRefreshSessions(ctx context.Context, userID uint64, sessionID *uint64) (*SupportRefreshSessionRevocation, error) {
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

func (s *Service) GetSupportCall(ctx context.Context, callID string) (*SupportCallDetails, error) {
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

func (s *Service) ReportCategories() []string {
	return reportCategoryList()
}

func (s *Service) GetFollowup(ctx context.Context, userID uint64, callID string) (*FollowupResponse, error) {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return nil, ErrFollowupNotFound
	}
	followup, err := s.repo.GetCallFollowup(ctx, callID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrFollowupNotFound
		}
		return nil, err
	}
	var tasks []models.FollowUpTask
	if err := s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		var taskErr error
		tasks, taskErr = s.repo.GetFollowUpTasksByCalls(ctx, []string{callID}, userID, nil)
		return taskErr
	}); err != nil {
		return nil, err
	}
	return &FollowupResponse{
		Followup: followup,
		Tasks:    tasks,
	}, nil
}

func (s *Service) ListFollowUpTasks(ctx context.Context, userID uint64) ([]FollowUpListItem, error) {
	tasks, err := s.repo.ListFollowUpTasksByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(tasks) == 0 {
		return []FollowUpListItem{}, nil
	}

	callIDs := make([]string, 0, len(tasks))
	peerIDs := make([]uint64, 0, len(tasks))
	callIDSeen := make(map[string]struct{})
	peerIDSeen := make(map[uint64]struct{})
	for _, task := range tasks {
		if task.CallID != "" {
			if _, ok := callIDSeen[task.CallID]; !ok {
				callIDs = append(callIDs, task.CallID)
				callIDSeen[task.CallID] = struct{}{}
			}
		}
		if _, ok := peerIDSeen[task.PeerUserID]; !ok {
			peerIDs = append(peerIDs, task.PeerUserID)
			peerIDSeen[task.PeerUserID] = struct{}{}
		}
	}

	callMap := make(map[string]models.CallSession)
	if len(callIDs) > 0 {
		var calls []models.CallSession
		if err := s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
			for _, callID := range callIDs {
				call, callErr := s.repo.GetCallSession(ctx, callID)
				if callErr != nil && !errors.Is(callErr, gorm.ErrRecordNotFound) {
					return callErr
				}
				if call != nil {
					calls = append(calls, *call)
				}
			}
			return nil
		}); err != nil {
			return nil, err
		}
		for _, item := range calls {
			callMap[item.CallID] = item
		}
	}
	followupMap := make(map[string]models.CallFollowup)
	if len(callIDs) > 0 {
		followups, err := s.repo.GetCallFollowupsByCalls(ctx, callIDs, userID)
		if err != nil {
			return nil, err
		}
		for _, item := range followups {
			followupMap[item.CallID] = item
		}
	}
	peerMap := make(map[uint64]models.User)
	if len(peerIDs) > 0 {
		peers, err := s.repo.GetUsersByIDs(ctx, peerIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range peers {
			peerMap[item.ID] = item
		}
	}
	contactMap := make(map[uint64]models.ContactProfile)
	if len(peerIDs) > 0 {
		contacts, err := s.repo.GetContactProfilesByOwnerAndContacts(ctx, userID, peerIDs)
		if err != nil {
			return nil, err
		}
		for _, item := range contacts {
			contactMap[item.ContactUserID] = item
		}
	}

	now := time.Now().UTC()
	items := make([]FollowUpListItem, 0, len(tasks))
	for _, task := range tasks {
		item := FollowUpListItem{
			Task:      task,
			IsOverdue: task.DueAt != nil && task.Status == models.FollowupTaskStatusOpen && task.DueAt.Before(now),
		}
		if call, ok := callMap[task.CallID]; ok {
			item.Call = &call
		}
		if followup, ok := followupMap[task.CallID]; ok {
			item.Followup = &followup
		}
		if peer, ok := peerMap[task.PeerUserID]; ok {
			item.Peer = &peer
		}
		if contact, ok := contactMap[task.PeerUserID]; ok {
			item.Contact = &contact
		}
		items = append(items, item)
	}

	sort.SliceStable(items, func(i, j int) bool {
		priority := func(item FollowUpListItem) int {
			if item.IsOverdue {
				return 0
			}
			if item.Task.DueAt != nil {
				nowDate := now.Format("2006-01-02")
				dueDate := item.Task.DueAt.UTC().Format("2006-01-02")
				if dueDate == nowDate {
					return 1
				}
				return 2
			}
			if item.Task.Status == models.FollowupTaskStatusDone {
				return 4
			}
			return 3
		}
		left := priority(items[i])
		right := priority(items[j])
		if left != right {
			return left < right
		}
		return items[i].Task.CreatedAt.After(items[j].Task.CreatedAt)
	})

	return items, nil
}

func (s *Service) CreateFollowUpTask(ctx context.Context, task *models.FollowUpTask) (*models.FollowUpTask, error) {
	if task == nil || task.UserID == 0 || task.PeerUserID == 0 || strings.TrimSpace(task.Type) == "" {
		return nil, errors.New("invalid follow-up task payload")
	}
	normalizedType, err := normalizeFollowUpTaskType(task.Type)
	if err != nil {
		return nil, err
	}
	task.Type = normalizedType
	task.Status = strings.TrimSpace(task.Status)
	if task.Status == "" {
		task.Status = models.FollowupTaskStatusOpen
	}
	normalizedStatus, err := normalizeFollowUpTaskStatus(task.Status)
	if err != nil {
		return nil, err
	}
	task.Status = normalizedStatus
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		task.Title = "跟进联系人"
	}
	task.Description = strings.TrimSpace(task.Description)
	if err := s.repo.CreateFollowUpTask(ctx, task); err != nil {
		return nil, err
	}
	if s.metrics != nil && task.Status == models.FollowupTaskStatusOpen {
		s.metrics.Inc("followup_task_open_total")
	}
	return task, nil
}

func (s *Service) UpdateFollowUpTask(ctx context.Context, userID, taskID uint64, updates map[string]any) (*models.FollowUpTask, error) {
	if taskID == 0 {
		return nil, errors.New("task id is required")
	}
	task, err := s.repo.GetFollowUpTask(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	patch := map[string]any{"updated_at": time.Now().UTC()}
	if status, ok := updates["status"].(string); ok && strings.TrimSpace(status) != "" {
		normalizedStatus, err := normalizeFollowUpTaskStatus(status)
		if err != nil {
			return nil, err
		}
		patch["status"] = normalizedStatus
		if normalizedStatus == models.FollowupTaskStatusDone {
			patch["completed_at"] = time.Now().UTC()
		}
	}
	if dueAt, ok := updates["due_at"].(*time.Time); ok {
		patch["due_at"] = dueAt
	}
	if reminderMode, ok := updates["reminder_mode"].(string); ok {
		patch["reminder_mode"] = strings.TrimSpace(reminderMode)
	}
	if description, ok := updates["description"].(string); ok {
		patch["description"] = strings.TrimSpace(description)
	}
	if err := s.repo.UpdateFollowUpTask(ctx, taskID, patch); err != nil {
		return nil, err
	}
	task, err = s.repo.GetFollowUpTask(ctx, taskID, userID)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (s *Service) GenerateFollowupForCall(ctx context.Context, callID string, force bool) error {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return errors.New("call id is required")
	}

	call, err := s.repo.GetCallSession(ctx, callID)
	if err != nil {
		return err
	}
	if call.Status == models.CallStatusInvited {
		return nil
	}

	for _, userID := range []uint64{call.CallerID, call.CalleeID} {
		if err := s.generateFollowupForUser(ctx, *call, userID, force); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) generateFollowupForUser(ctx context.Context, call models.CallSession, userID uint64, force bool) error {
	peerID := call.CalleeID
	peerEmail := call.CalleeEmail
	peerName := call.CalleeDisplayName
	if userID == call.CalleeID {
		peerID = call.CallerID
		peerEmail = call.CallerEmail
		peerName = call.CallerDisplayName
	}

	existing, err := s.repo.GetCallFollowup(ctx, call.CallID, userID)
	if err == nil && !force {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	_ = existing

	wasAnswered := call.AnsweredAt != nil
	var transcriptSegments []models.CallTranscriptSegment
	if wasAnswered || call.Status == models.CallStatusAnswered {
		transcriptSegments, err = s.repo.GetTranscriptSegmentsByCallAndUser(ctx, call.CallID, userID)
		if err != nil {
			return err
		}
	}

	durationSeconds := int64(0)
	if call.AnsweredAt != nil && call.EndedAt != nil {
		durationSeconds = int64(call.EndedAt.Sub(*call.AnsweredAt).Seconds())
	}

	followup := models.CallFollowup{
		CallID:          call.CallID,
		UserID:          userID,
		PeerUserID:      peerID,
		Status:          models.FollowupStatusReady,
		Source:          "metadata",
		TranscriptCount: int64(len(transcriptSegments)),
	}

	if wasAnswered || call.Status == models.CallStatusAnswered {
		if len(transcriptSegments) >= 6 || durationSeconds >= 45 {
			followup.Source = "rules"
			followup.SummaryCN = fmt.Sprintf("与 %s 的通话已完成，围绕业务沟通形成了可复用的跟进摘要。", peerNameOrEmail(peerName, peerEmail))
			followup.SummaryEN = fmt.Sprintf("The call with %s completed and is ready for a follow-up.", peerNameOrEmail(peerName, peerEmail))
			keyPoints := []string{}
			actionItems := []string{"发送一条简短双语跟进消息，确认下一步。"}
			riskFlags := []string{}
			if len(transcriptSegments) > 0 {
				keyPoints = append(keyPoints, truncateSentence(transcriptSegments[0].TranslatedText, 140))
				last := transcriptSegments[len(transcriptSegments)-1]
				if last.TranslatedText != "" && last.TranslatedText != transcriptSegments[0].TranslatedText {
					keyPoints = append(keyPoints, truncateSentence(last.TranslatedText, 140))
				}
			}
			if len(keyPoints) == 0 {
				keyPoints = append(keyPoints, "本次通话已完成，建议尽快发送后续确认消息。")
			}
			if strings.Contains(strings.ToLower(strings.Join(extractTexts(transcriptSegments), " ")), "tomorrow") ||
				strings.Contains(strings.Join(extractTexts(transcriptSegments), " "), "明天") {
				actionItems = []string{"根据通话约定，安排下一次沟通时间。"}
				followup.NextStep = "安排下一次通话时间"
			} else {
				followup.NextStep = "发送双语跟进消息"
			}
			if durationSeconds < 60 {
				riskFlags = append(riskFlags, "通话较短，建议确认关键需求是否理解一致。")
			}
			followup.KeyPointsJSON = mustJSON(keyPoints)
			followup.ActionItemsJSON = mustJSON(actionItems)
			followup.RiskFlagsJSON = mustJSON(riskFlags)
			followup.FollowupDraftCN = fmt.Sprintf("你好，感谢刚才的沟通。我整理了本次通话的重点，建议我们按约定推进下一步。")
			followup.FollowupDraftEN = "Thanks for the call. I have summarized the key points and suggest we move on to the agreed next step."
		} else {
			if s.metrics != nil {
				s.metrics.Inc("followup_metadata_fallback_total")
			}
			followup.SummaryCN = fmt.Sprintf("与 %s 的通话已结束，可手动补充本次业务跟进。", peerNameOrEmail(peerName, peerEmail))
			followup.SummaryEN = fmt.Sprintf("The call with %s has ended. Add a manual follow-up if needed.", peerNameOrEmail(peerName, peerEmail))
			followup.NextStep = "手动确认后续动作"
			followup.KeyPointsJSON = mustJSON([]string{"暂无足够字幕内容，建议手动补充重点。"})
			followup.ActionItemsJSON = mustJSON([]string{"回顾本次通话，并记录下一步动作。"})
			followup.RiskFlagsJSON = mustJSON([]string{})
			followup.FollowupDraftCN = "你好，刚才的通话我已记录。方便的话，我们确认一下下一步安排。"
			followup.FollowupDraftEN = "I noted our recent call. Please let me know the best next step when convenient."
		}
	} else {
		followup.SummaryCN = fmt.Sprintf("与 %s 的通话未完成，建议尽快回拨。", peerNameOrEmail(peerName, peerEmail))
		followup.SummaryEN = fmt.Sprintf("The call with %s did not complete. A callback is recommended.", peerNameOrEmail(peerName, peerEmail))
		followup.NextStep = "安排回拨"
		followup.KeyPointsJSON = mustJSON([]string{"通话未接通，建议尽快回访。"})
		followup.ActionItemsJSON = mustJSON([]string{"重新联系对方并确认可沟通时间。"})
		followup.RiskFlagsJSON = mustJSON([]string{"未完成首次沟通，业务上下文可能中断。"})
		followup.FollowupDraftCN = "你好，刚才未能接通，方便时请回复一个适合沟通的时间。"
		followup.FollowupDraftEN = "We missed each other on the last call. Please share a suitable time for a callback."
	}

	now := time.Now().UTC()
	followup.GeneratedAt = &now

	return s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		current, txErr := s.repo.GetCallFollowup(ctx, call.CallID, userID)
		if txErr == nil {
			followup.ID = current.ID
			if err := s.repo.UpdateCallFollowup(ctx, current, map[string]any{
				"status":          followup.Status,
				"source":          followup.Source,
				"summary_cn":      followup.SummaryCN,
				"summary_en":      followup.SummaryEN,
				"key_points_json": followup.KeyPointsJSON,
				"action_items_json": followup.ActionItemsJSON,
				"next_step":       followup.NextStep,
				"risk_flags_json": followup.RiskFlagsJSON,
				"followup_draft_cn": followup.FollowupDraftCN,
				"followup_draft_en": followup.FollowupDraftEN,
				"generated_at":    followup.GeneratedAt,
				"transcript_count": followup.TranscriptCount,
				"updated_at":      now,
			}); err != nil {
				if s.metrics != nil {
					s.metrics.Inc("followup_generate_fail_total")
				}
				return err
			}
		} else if errors.Is(txErr, gorm.ErrRecordNotFound) {
			if err := s.repo.SaveCallFollowup(ctx, &followup); err != nil {
				if s.metrics != nil {
					s.metrics.Inc("followup_generate_fail_total")
				}
				return err
			}
		} else {
			if s.metrics != nil {
				s.metrics.Inc("followup_generate_fail_total")
			}
			return txErr
		}

		if !wasAnswered || call.Status == models.CallStatusRejected || call.Status == models.CallStatusMissed || call.Status == models.CallStatusFailed {
			if err := s.ensureDefaultFollowupTask(ctx, call, userID, peerID, models.FollowupTaskTypeCallback, "回拨该联系人", "通话未接通，建议尽快回拨。", time.Now().UTC().Add(2*time.Hour)); err != nil {
				return err
			}
		} else {
			taskType := models.FollowupTaskTypeSendMessage
			title := "发送双语跟进消息"
			description := "发送一条双语跟进消息，确认关键结论与下一步。"
			textBlob := strings.ToLower(strings.Join(extractTexts(transcriptSegments), " "))
			if strings.Contains(textBlob, "tomorrow") || strings.Contains(strings.Join(extractTexts(transcriptSegments), " "), "明天") {
				taskType = models.FollowupTaskTypeScheduleNextCall
				title = "安排下一次通话"
				description = "根据本次通话内容，安排下一次沟通时间。"
			}
			if err := s.ensureDefaultFollowupTask(ctx, call, userID, peerID, taskType, title, description, time.Now().UTC().Add(24*time.Hour)); err != nil {
				return err
			}
		}
		if s.metrics != nil {
			s.metrics.Inc("followup_generate_total")
		}
		if len(transcriptSegments) > 0 {
			if err := s.repo.DeleteTranscriptSegmentsByCallAndUser(ctx, call.CallID, userID); err != nil {
				return err
			}
			if s.metrics != nil {
				s.metrics.Add("transcript_segment_purged_total", int64(len(transcriptSegments)))
			}
		}
		return nil
	})
}

func (s *Service) ensureDefaultFollowupTask(ctx context.Context, call models.CallSession, userID, peerID uint64, taskType, title, description string, dueAt time.Time) error {
	existing, err := s.repo.GetFollowUpTaskByCallAndType(ctx, call.CallID, userID, taskType)
	if err == nil {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	_ = existing
	task := &models.FollowUpTask{
		UserID:       userID,
		PeerUserID:   peerID,
		CallID:       call.CallID,
		Type:         taskType,
		Status:       models.FollowupTaskStatusOpen,
		Title:        title,
		Description:  description,
		DueAt:        &dueAt,
		ReminderMode: "default",
	}
	if err := s.repo.CreateFollowUpTask(ctx, task); err != nil {
		return err
	}
	if s.metrics != nil {
		s.metrics.Inc("followup_task_open_total")
	}
	return nil
}

func mustJSON(values []string) string {
	raw, _ := json.Marshal(values)
	return string(raw)
}

func extractTexts(segments []models.CallTranscriptSegment) []string {
	texts := make([]string, 0, len(segments))
	for _, segment := range segments {
		if strings.TrimSpace(segment.TranslatedText) != "" {
			texts = append(texts, strings.TrimSpace(segment.TranslatedText))
			continue
		}
		if strings.TrimSpace(segment.OriginalText) != "" {
			texts = append(texts, strings.TrimSpace(segment.OriginalText))
		}
	}
	return texts
}

func truncateSentence(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return strings.TrimSpace(value[:limit]) + "..."
}

func peerNameOrEmail(name, email string) string {
	if strings.TrimSpace(name) != "" {
		return strings.TrimSpace(name)
	}
	return strings.TrimSpace(email)
}

func parseAppUserID(value string) (uint64, error) {
	var userID uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "user:%d", &userID); err == nil && userID > 0 {
		return userID, nil
	}
	if _, err := fmt.Sscanf(strings.TrimSpace(value), "%d", &userID); err == nil && userID > 0 {
		return userID, nil
	}
	return 0, errors.New("invalid app_user_id")
}

func (s *Service) DeleteAccount(ctx context.Context, userID uint64) (*models.DeletionAudit, error) {
	audit := &models.DeletionAudit{
		UserID:    userID,
		DeletedAt: time.Now().UTC(),
	}

	err := s.repo.RunInTransaction(ctx, func(tx *gorm.DB) error {
		var err error
		if audit.ContactsDeleted, err = s.repo.DeleteContactsByUser(ctx, userID); err != nil {
			return err
		}
		if audit.CallsDeleted, err = s.repo.DeleteCallsByUser(ctx, userID); err != nil {
			return err
		}
		if audit.BlocksDeleted, err = s.repo.DeleteBlocksByUser(ctx, userID); err != nil {
			return err
		}
		if audit.ReportsDeleted, err = s.repo.DeleteAbuseReportsByUser(ctx, userID); err != nil {
			return err
		}
		if audit.LegalRecordsDeleted, err = s.repo.DeleteLegalAcceptancesByUser(ctx, userID); err != nil {
			return err
		}
		if audit.UsageRowsDeleted, err = s.repo.DeleteUsageLedgersByUser(ctx, userID); err != nil {
			return err
		}
		usageSlicesDeleted, err := s.repo.DeleteTranslationUsageSlicesByUser(ctx, userID)
		if err != nil {
			return err
		}
		audit.UsageRowsDeleted += usageSlicesDeleted
		if audit.EntitlementsDeleted, err = s.repo.DeleteEntitlementsByUser(ctx, userID); err != nil {
			return err
		}

		if _, err = s.repo.DeleteEmailVerificationCodesByUserEmail(ctx, userID); err != nil {
			return err
		}

		updates := map[string]any{
			"email":         fmt.Sprintf("deleted-user-%d@deleted.local", userID),
			"display_name":  "Deleted User",
			"password_hash": "deleted",
			"fcm_token":     "",
			"status":        models.UserStatusDeleted,
			"deleted_at":    audit.DeletedAt,
			"updated_at":    audit.DeletedAt,
		}
		if err := s.repo.UpdateUser(ctx, userID, updates); err != nil {
			return err
		}
		return s.repo.CreateDeletionAudit(ctx, audit)
	})

	if err != nil {
		return nil, err
	}
	return audit, nil
}

func EncodePayload(v any) []byte {
	raw, _ := json.Marshal(v)
	return raw
}
