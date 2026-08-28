// Package alerting implements severity-graded alert routing and notification.
//
// Alerts are graded (P1/P2/P3/INFO) and routed to one or more Providers per
// severity. A small in-memory dedup window suppresses duplicate fingerprints so
// a flapping dependency cannot page on every request.
package alerting

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
)

// Severity grades an alert. Lower numeric rank == more urgent.
type Severity string

const (
	// SeverityP1 requires immediate page (e.g. data-loss, outage).
	SeverityP1 Severity = "P1"
	// SeverityP2 notifies on-call during business hours.
	SeverityP2 Severity = "P2"
	// SeverityP3 creates a ticket for batch follow-up.
	SeverityP3 Severity = "P3"
	// SeverityInfo is informational only.
	SeverityInfo Severity = "INFO"
)

// Alert is a single alert event.
type Alert struct {
	ID          string            `json:"id"`
	Severity    Severity          `json:"severity"`
	Title       string            `json:"title"`
	Detail      string            `json:"detail"`
	Labels      map[string]string `json:"labels,omitempty"`
	Fingerprint string            `json:"fingerprint"`
	FirstSeen   time.Time         `json:"first_seen"`
	Count       int               `json:"count"`
}

// Provider receives routed alerts. Implementations must be safe for concurrent use.
type Provider interface {
	Notify(ctx context.Context, a Alert) error
}

// LogProvider logs alerts via zerolog (always-on fallback sink).
type LogProvider struct {
	Log zerolog.Logger
}

// Notify implements Provider.
func (p LogProvider) Notify(_ context.Context, a Alert) error {
	p.Log.Error().
		Str("alert_id", a.ID).
		Str("severity", string(a.Severity)).
		Str("title", a.Title).
		Str("detail", a.Detail).
		Interface("labels", a.Labels).
		Msg("alert")
	return nil
}

// WebhookProvider POSTs alert JSON to a configured URL.
type WebhookProvider struct {
	URL        string
	HTTPClient *http.Client
}

// Notify implements Provider by POSTing the alert as JSON.
func (p WebhookProvider) Notify(ctx context.Context, a Alert) error {
	client := p.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	body, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("alerting: marshal alert: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("alerting: build webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("alerting: post webhook: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("alerting: webhook returned %d", resp.StatusCode)
	}
	return nil
}

// MultiProvider fans a single alert out to several providers.
type MultiProvider []Provider

// Notify implements Provider, returning the first error encountered.
func (m MultiProvider) Notify(ctx context.Context, a Alert) error {
	var firstErr error
	for _, p := range m {
		if err := p.Notify(ctx, a); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
