package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRealtimeTicketSingleUseAndChannelBinding(t *testing.T) {
	mini := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	service := NewRealtimeTicketService(client)

	ticket, _, err := service.Issue(context.Background(), &Claims{UserID: 7, Email: "user@example.com"}, "chat")
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	claims, err := service.Consume(context.Background(), ticket, "chat")
	if err != nil || claims.UserID != 7 {
		t.Fatalf("consume ticket: claims=%+v err=%v", claims, err)
	}
	if _, err := service.Consume(context.Background(), ticket, "chat"); !errors.Is(err, ErrRealtimeTicketInvalid) {
		t.Fatalf("expected second consume to fail, got %v", err)
	}

	wrongChannel, _, err := service.Issue(context.Background(), &Claims{UserID: 7, Email: "user@example.com"}, "chat")
	if err != nil {
		t.Fatalf("issue second ticket: %v", err)
	}
	if _, err := service.Consume(context.Background(), wrongChannel, "signaling"); !errors.Is(err, ErrRealtimeTicketInvalid) {
		t.Fatalf("expected channel mismatch, got %v", err)
	}
}
