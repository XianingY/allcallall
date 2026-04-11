package fcm

import (
	"context"
	"errors"
	"fmt"
	"os"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/rs/zerolog"
	"google.golang.org/api/option"
)

// Manager handles Firebase Cloud Messaging operations
type Manager struct {
	logger   zerolog.Logger
	client   *messaging.Client
	enabled  bool
	disabled string
}

// NewManager creates a new FCM manager.
func NewManager(ctx context.Context, logger zerolog.Logger, serviceAccountPath string) (*Manager, error) {
	manager := &Manager{
		logger: logger.With().Str("component", "fcm_manager").Logger(),
	}

	if serviceAccountPath == "" {
		manager.disabled = "FCM disabled: FCM_SERVICE_ACCOUNT_PATH is not configured"
		manager.logger.Info().Msg(manager.disabled)
		return manager, nil
	}

	if _, err := os.Stat(serviceAccountPath); err != nil {
		return nil, fmt.Errorf("stat fcm service account: %w", err)
	}

	app, err := firebase.NewApp(ctx, nil, option.WithCredentialsFile(serviceAccountPath))
	if err != nil {
		return nil, fmt.Errorf("initialize firebase app: %w", err)
	}

	client, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("initialize firebase messaging: %w", err)
	}

	manager.client = client
	manager.enabled = true
	manager.logger.Info().Str("credentials_path", serviceAccountPath).Msg("fcm enabled")
	return manager, nil
}

// NewDisabledManagerForTests creates a manager that safely no-ops without external dependencies.
func NewDisabledManagerForTests(logger zerolog.Logger) *Manager {
	return &Manager{
		logger:   logger.With().Str("component", "fcm_manager").Logger(),
		disabled: "FCM disabled for test",
	}
}

// SendCallNotification sends an incoming call notification
func (m *Manager) SendCallNotification(ctx context.Context, fcmToken string, fromEmail string, displayName string, callID string) error {
	if fcmToken == "" {
		m.logger.Debug().Str("from", fromEmail).Msg("fcm token is empty, skipping notification")
		return nil
	}

	if !m.enabled || m.client == nil {
		m.logger.Debug().
			Str("from", fromEmail).
			Str("call_id", callID).
			Msg("fcm disabled, skipping call notification")
		return nil
	}

	message := &messaging.Message{
		Token: fcmToken,
		Notification: &messaging.Notification{
			Title: "Incoming call",
			Body:  fmt.Sprintf("%s is calling you", displayName),
		},
		Data: map[string]string{
			"type":         "incoming_call",
			"call_id":      callID,
			"from_user":    displayName,
			"display_name": displayName,
			"from_email":   fromEmail,
		},
		Android: &messaging.AndroidConfig{
			Priority: "high",
			Data: map[string]string{
				"type":         "incoming_call",
				"call_id":      callID,
				"from_user":    displayName,
				"display_name": displayName,
				"from_email":   fromEmail,
			},
		},
		APNS: &messaging.APNSConfig{
			Headers: map[string]string{
				"apns-priority":  "10",
				"apns-push-type": "alert",
			},
			Payload: &messaging.APNSPayload{
				Aps: &messaging.Aps{
					Sound: "default",
				},
				CustomData: map[string]any{
					"type":         "incoming_call",
					"call_id":      callID,
					"from_user":    displayName,
					"display_name": displayName,
					"from_email":   fromEmail,
				},
			},
		},
	}

	messageID, err := m.client.Send(ctx, message)
	if err != nil {
		return fmt.Errorf("send call notification: %w", err)
	}

	m.logger.Info().
		Str("from", fromEmail).
		Str("call_id", callID).
		Str("message_id", messageID).
		Msg("call notification sent")
	return nil
}

// SendMissedCallNotification sends a missed call notification
func (m *Manager) SendMissedCallNotification(ctx context.Context, fcmToken string, fromEmail string, displayName string) error {
	if fcmToken == "" {
		m.logger.Debug().Str("from", fromEmail).Msg("fcm token is empty, skipping notification")
		return nil
	}

	if !m.enabled || m.client == nil {
		m.logger.Debug().
			Str("from", fromEmail).
			Msg("fcm disabled, skipping missed call notification")
		return nil
	}

	message := &messaging.Message{
		Token: fcmToken,
		Notification: &messaging.Notification{
			Title: "Missed call",
			Body:  fmt.Sprintf("You missed a call from %s", displayName),
		},
		Data: map[string]string{
			"type":         "call_ended",
			"from_user":    displayName,
			"display_name": displayName,
			"from_email":   fromEmail,
		},
	}

	if _, err := m.client.Send(ctx, message); err != nil {
		return fmt.Errorf("send missed call notification: %w", err)
	}

	return nil
}

// Enabled reports whether FCM delivery is active.
func (m *Manager) Enabled() bool {
	return m != nil && m.enabled && m.client != nil
}

// DisabledReason returns the configured disabled reason, if any.
func (m *Manager) DisabledReason() error {
	if m == nil || m.disabled == "" {
		return nil
	}
	return errors.New(m.disabled)
}
