package commerce

import (
	"github.com/allcallall/backend/internal/metrics"

	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/allcallall/backend/internal/models"
)

const (
	premiumMonthlyProductID = "premium_monthly"
	premiumYearlyProductID  = "premium_yearly"

	revenueCatSource = "revenuecat"
)

// RevenueCatWebhook is the payload structure for RevenueCat webhook events.
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

// BillingWebhookService processes RevenueCat billing webhooks and updates entitlements.
type BillingWebhookService struct {
	repo    *Repository
	metrics metrics.Recorder
}

// NewBillingWebhookService creates a new BillingWebhookService.
func NewBillingWebhookService(repo *Repository, metrics metrics.Recorder) *BillingWebhookService {
	return &BillingWebhookService{repo: repo, metrics: metrics}
}

// HandleRevenueCatWebhook processes a RevenueCat billing event and updates entitlements.
func (s *BillingWebhookService) HandleRevenueCatWebhook(ctx context.Context, payload RevenueCatWebhook, raw []byte) error {
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
