package collaboration

import (
	"testing"
	"time"

	"github.com/allcallall/backend/internal/models"
)

func TestMessageRetentionPolicyNormalizedAppliesWeChatAlignedDefaults(t *testing.T) {
	policy := MessageRetentionPolicy{Enabled: true}.Normalized()
	if policy.TextTTL != 72*time.Hour {
		t.Fatalf("text ttl=%v want=72h", policy.TextTTL)
	}
	if policy.MediaTTL != 120*time.Hour {
		t.Fatalf("media ttl=%v want=120h", policy.MediaTTL)
	}
}

func TestMessageRetentionPolicyDisabledReturnsNoDeadline(t *testing.T) {
	policy := MessageRetentionPolicy{Enabled: false}
	if got := policy.RetentionUntil(time.Now(), models.MessageTypeText, false); got != nil {
		t.Fatalf("expected nil deadline when disabled, got %v", got)
	}
	if got := policy.AttachmentRetentionUntil(time.Now()); got != nil {
		t.Fatalf("expected nil attachment deadline when disabled, got %v", got)
	}
}

func TestMessageRetentionPolicyUsesMediaWindowWhenAttachmentsPresent(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	policy := MessageRetentionPolicy{Enabled: true}.Normalized()
	policy.Enabled = true

	text := policy.RetentionUntil(now, models.MessageTypeText, false)
	if text == nil || !text.Equal(now.Add(72*time.Hour)) {
		t.Fatalf("text deadline=%v want=%v", text, now.Add(72*time.Hour))
	}

	media := policy.RetentionUntil(now, models.MessageTypeText, true)
	if media == nil || !media.Equal(now.Add(120*time.Hour)) {
		t.Fatalf("media deadline=%v want=%v", media, now.Add(120*time.Hour))
	}
}

func TestMessageRetentionPolicyExemptsOperationalMessagesByDefault(t *testing.T) {
	now := time.Now()
	policy := MessageRetentionPolicy{Enabled: true}.Normalized()
	policy.Enabled = true

	for _, kind := range []string{models.MessageTypeSystem, models.MessageTypeCallEvent} {
		if got := policy.RetentionUntil(now, kind, false); got != nil {
			t.Fatalf("expected %s to be exempt from retention, got %v", kind, got)
		}
	}

	policy.PurgeSystemMessages = true
	if got := policy.RetentionUntil(now, models.MessageTypeSystem, false); got == nil {
		t.Fatal("expected system message to be purgeable once opted in")
	}
}

func TestIsOperationalMessageType(t *testing.T) {
	cases := map[string]bool{
		models.MessageTypeText:      false,
		models.MessageTypeSystem:    true,
		models.MessageTypeCallEvent: true,
	}
	for kind, want := range cases {
		if got := isOperationalMessageType(kind); got != want {
			t.Fatalf("isOperationalMessageType(%s)=%v want=%v", kind, got, want)
		}
	}
}
