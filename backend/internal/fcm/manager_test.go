package fcm

import (
	"context"
	"testing"

	"github.com/rs/zerolog"
)

func TestManager(t *testing.T) {
	mgr := NewManager(zerolog.Nop())

	if err := mgr.SendCallNotification(context.Background(), "", "alice@example.com", "Alice", "call-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.SendCallNotification(context.Background(), "token", "alice@example.com", "Alice", "call-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.SendMissedCallNotification(context.Background(), "", "alice@example.com", "Alice"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mgr.SendMissedCallNotification(context.Background(), "token", "alice@example.com", "Alice"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
