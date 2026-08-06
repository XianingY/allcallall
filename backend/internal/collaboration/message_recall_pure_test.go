package collaboration

import (
	"testing"
	"time"
)

func TestMessageRecallPolicyNormalizedFillsWeChatDefault(t *testing.T) {
	policy := MessageRecallPolicy{Enabled: true}.Normalized()
	if policy.Window != 2*time.Minute {
		t.Fatalf("default window=%s want=2m (WeChat parity)", policy.Window)
	}
}

func TestMessageRecallPolicyNormalizedKeepsExplicitWindow(t *testing.T) {
	policy := MessageRecallPolicy{Enabled: true, Window: 24 * time.Hour}.Normalized()
	if policy.Window != 24*time.Hour {
		t.Fatalf("window=%s want=24h", policy.Window)
	}
}

func TestSenderCanRecallInsideWindow(t *testing.T) {
	policy := MessageRecallPolicy{Enabled: true, Window: 2 * time.Minute}
	created := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)

	cases := []struct {
		name string
		now  time.Time
		want bool
	}{
		{name: "immediately", now: created, want: true},
		{name: "one minute later", now: created.Add(time.Minute), want: true},
		{name: "exactly at deadline", now: created.Add(2 * time.Minute), want: true},
		{name: "one second past deadline", now: created.Add(2*time.Minute + time.Second), want: false},
		{name: "an hour later", now: created.Add(time.Hour), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.SenderCanRecall(tc.now, created); got != tc.want {
				t.Fatalf("SenderCanRecall=%t want=%t", got, tc.want)
			}
		})
	}
}

func TestSenderCanRecallRejectsWhenDisabled(t *testing.T) {
	policy := MessageRecallPolicy{Enabled: false, Window: time.Hour}
	now := time.Now()
	if policy.SenderCanRecall(now, now) {
		t.Fatal("disabled policy must never allow recall")
	}
}
