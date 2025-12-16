package fcm

import (
	"context"

	"github.com/rs/zerolog"
)

// Manager handles Firebase Cloud Messaging operations
type Manager struct {
	logger zerolog.Logger
}

// NewManager creates a new FCM manager
func NewManager(logger zerolog.Logger) *Manager {
	return &Manager{
		logger: logger.With().Str("component", "fcm_manager").Logger(),
	}
}

// SendCallNotification sends an incoming call notification
func (m *Manager) SendCallNotification(ctx context.Context, fcmToken string, fromEmail string, displayName string, callID string) error {
	if fcmToken == "" {
		m.logger.Debug().Str("from", fromEmail).Msg("fcm token is empty, skipping notification")
		return nil
	}

	m.logger.Info().
		Str("from", fromEmail).
		Str("call_id", callID).
		Msg("call notification would be sent (Firebase SDK not yet configured)")

	return nil
}

// SendMissedCallNotification sends a missed call notification
func (m *Manager) SendMissedCallNotification(ctx context.Context, fcmToken string, fromEmail string, displayName string) error {
	if fcmToken == "" {
		m.logger.Debug().Str("from", fromEmail).Msg("fcm token is empty, skipping notification")
		return nil
	}

	m.logger.Info().
		Str("from", fromEmail).
		Msg("missed call notification would be sent (Firebase SDK not yet configured)")

	return nil
}
