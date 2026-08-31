package alerting

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Router decides which providers handle a given severity.
type Router interface {
	ProvidersFor(severity Severity) []Provider
}

// Routing is a static severity -> providers map.
type Routing map[Severity][]Provider

// ProvidersFor implements Router.
func (r Routing) ProvidersFor(severity Severity) []Provider {
	if ps, ok := r[severity]; ok {
		return ps
	}
	return r[SeverityP3] // default fan-out target for unknown severities
}

// Service emits and routes alerts.
type Service struct {
	router      Router
	dedupWindow time.Duration
	mu          sync.Mutex
	seen        map[string]time.Time
}

// Option configures a Service.
type Option func(*Service)

// WithDedupWindow sets how long duplicate fingerprints are suppressed.
func WithDedupWindow(d time.Duration) Option {
	return func(s *Service) { s.dedupWindow = d }
}

// NewService builds an alerting Service.
func NewService(router Router, opts ...Option) *Service {
	s := &Service{
		router:      router,
		dedupWindow: 5 * time.Minute,
		seen:        make(map[string]time.Time),
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// Emit routes an alert to the providers configured for its severity. Duplicate
// fingerprints within the dedup window are suppressed (but counted). A zero
// Fingerprint is auto-derived from Title+Labels so callers need not compute it.
func (s *Service) Emit(ctx context.Context, a Alert) error {
	if a.Fingerprint == "" {
		a.Fingerprint = deriveFingerprint(a.Title, a.Labels)
	}
	if a.ID == "" {
		a.ID = uuid.NewString()
	}
	if a.FirstSeen.IsZero() {
		a.FirstSeen = time.Now().UTC()
	}
	if a.Count == 0 {
		a.Count = 1
	}

	if s.suppressed(a.Fingerprint) {
		return nil
	}

	var lastErr error
	for _, p := range s.router.ProvidersFor(a.Severity) {
		if err := p.Notify(ctx, a); err != nil && lastErr == nil {
			lastErr = err
		}
	}
	return lastErr
}

// suppressed records the fingerprint and returns true if it was seen recently.
func (s *Service) suppressed(fp string) bool {
	if fp == "" || s.dedupWindow <= 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if t, ok := s.seen[fp]; ok && now.Sub(t) < s.dedupWindow {
		return true
	}
	s.seen[fp] = now
	// Best-effort GC of stale fingerprints to bound memory.
	if len(s.seen) > 4096 {
		for k, v := range s.seen {
			if now.Sub(v) >= s.dedupWindow {
				delete(s.seen, k)
			}
		}
	}
	return false
}

func deriveFingerprint(title string, labels map[string]string) string {
	fp := title
	for k, v := range labels {
		fp += "|" + k + "=" + v
	}
	return fp
}
