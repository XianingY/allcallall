package collaboration

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

// TestChatHubPublishCrossNode verifies that a message published on one hub is
// delivered to a client connected to a different hub through the Redis bridge,
// while the publishing node does not double-deliver to its own local client.
func TestChatHubPublishCrossNode(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis start failed: %v", err)
	}
	defer mini.Close()

	rdb := redis.NewClient(&redis.Options{Addr: mini.Addr()})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hubA := NewChatHub(rdb, zerolog.Nop())
	hubB := NewChatHub(rdb, zerolog.Nop())

	clientA := &chatClient{userID: 1, orgID: 1, send: make(chan []byte, 8)}
	hubA.addClient(clientA)
	clientB := &chatClient{userID: 1, orgID: 1, send: make(chan []byte, 8)}
	hubB.addClient(clientB)

	hubA.Start(ctx)
	hubB.Start(ctx)

	// Give the pattern subscriptions time to become active.
	time.Sleep(200 * time.Millisecond)

	event := RealtimeEventRecord{ID: 7, UserID: 1, OrganizationID: 1, Event: "chat.message"}
	if err := hubA.PublishToUser(ctx, event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	select {
	case <-clientA.send:
		// expected: local delivery on node A
	case <-time.After(2 * time.Second):
		t.Fatal("node A local client did not receive event")
	}

	select {
	case <-clientB.send:
		// expected: cross-node delivery via Redis
	case <-time.After(2 * time.Second):
		t.Fatal("node B did not receive cross-node event")
	}

	// Node A must not receive a duplicate of its own publication echoed back
	// from Redis (dedup by node id).
	select {
	case <-clientA.send:
		t.Fatal("node A received a duplicate of its own published event")
	case <-time.After(200 * time.Millisecond):
	}
}

// TestChatHubLocalOnlyFallback verifies the hub degrades gracefully (local-only
// delivery, no panic) when Redis is unavailable.
func TestChatHubLocalOnlyFallback(t *testing.T) {
	hub := NewChatHub(nil, zerolog.Nop())
	client := &chatClient{userID: 2, orgID: 1, send: make(chan []byte, 8)}
	hub.addClient(client)

	event := RealtimeEventRecord{ID: 8, UserID: 2, OrganizationID: 1, Event: "chat.message"}
	if err := hub.PublishToUser(context.Background(), event); err != nil {
		t.Fatalf("publish failed: %v", err)
	}
	select {
	case <-client.send:
		// expected: local delivery
	case <-time.After(2 * time.Second):
		t.Fatal("local client did not receive event in local-only mode")
	}
}
