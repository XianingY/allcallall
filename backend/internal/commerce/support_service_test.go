package commerce

import (
	"testing"
	"time"
)

func TestSupportRefreshSessionRisk(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		summary    SupportRefreshSessionSummary
		wantLevel  string
		wantReason string // one of the expected reasons must be present
	}{
		{
			name:      "no signal",
			summary:   SupportRefreshSessionSummary{},
			wantLevel: "none",
		},
		{
			name:       "many active sessions",
			summary:    SupportRefreshSessionSummary{ActiveCount: 5},
			wantLevel:  "low",
			wantReason: "many_active_sessions",
		},
		{
			name:       "single invalid use",
			summary:    SupportRefreshSessionSummary{InvalidUseCount: 1},
			wantLevel:  "medium",
			wantReason: "refresh_token_reuse_detected",
		},
		{
			name:       "repeated invalid use",
			summary:    SupportRefreshSessionSummary{InvalidUseCount: 3},
			wantLevel:  "high",
			wantReason: "repeated_refresh_token_reuse",
		},
		{
			name: "recent invalid use",
			summary: SupportRefreshSessionSummary{
				LastInvalidUseAt: ptrTime(now.Add(-1 * time.Hour)),
			},
			wantLevel:  "high",
			wantReason: "recent_refresh_token_reuse",
		},
		{
			name: "active sessions do not downgrade repeated reuse",
			summary: SupportRefreshSessionSummary{
				ActiveCount:     10,
				InvalidUseCount: 3,
			},
			wantLevel:  "high",
			wantReason: "repeated_refresh_token_reuse",
		},
		{
			// Boundary: exactly 24h ago must NOT trip the "recent" branch.
			name: "invalid use exactly 24h ago is not recent",
			summary: SupportRefreshSessionSummary{
				LastInvalidUseAt: ptrTime(now.Add(-24 * time.Hour)),
			},
			wantLevel: "none",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			level, reasons := supportRefreshSessionRisk(tc.summary, now)
			if level != tc.wantLevel {
				t.Fatalf("level = %q, want %q (reasons=%v)", level, tc.wantLevel, reasons)
			}
			if tc.wantReason != "" {
				found := false
				for _, r := range reasons {
					if r == tc.wantReason {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected reason %q in %v", tc.wantReason, reasons)
				}
			}
		})
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
