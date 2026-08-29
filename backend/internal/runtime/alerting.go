package runtime

import (
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/alerting"
)

// AlertingFromEnv builds the alert-routing service from environment config.
//
// The log sink is always installed so alerts are never silently dropped even
// when no webhook is configured — this package was dead code before, meaning
// P1/P2/P3 alerts reached nobody.
//
// Environment variables:
//   - ALERT_WEBHOOK_URL: when set, P1/P2 alerts are additionally POSTed there.
//   - ALERT_DEDUP_WINDOW: dedup window duration (default 15m).
func AlertingFromEnv(logger zerolog.Logger) *alerting.Service {
	logSink := alerting.LogProvider{Log: logger.With().Str("component", "alerting").Logger()}

	routing := alerting.Routing{
		alerting.SeverityP1:   {logSink},
		alerting.SeverityP2:   {logSink},
		alerting.SeverityP3:   {logSink},
		alerting.SeverityInfo: {logSink},
	}

	if url := strings.TrimSpace(os.Getenv("ALERT_WEBHOOK_URL")); url != "" {
		webhook := alerting.WebhookProvider{URL: url}
		routing[alerting.SeverityP1] = append(routing[alerting.SeverityP1], webhook)
		routing[alerting.SeverityP2] = append(routing[alerting.SeverityP2], webhook)
	}

	return alerting.NewService(routing, alerting.WithDedupWindow(dedupWindowFromEnv()))
}

func dedupWindowFromEnv() time.Duration {
	raw := strings.TrimSpace(os.Getenv("ALERT_DEDUP_WINDOW"))
	if raw == "" {
		return 15 * time.Minute
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return d
	}
	return 15 * time.Minute
}
