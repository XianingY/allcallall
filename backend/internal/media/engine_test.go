package media

import (
	"context"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
)

func TestEngineLifecycle(t *testing.T) {
	engine, err := NewEngine(zerolog.Nop(), &Config{WebRTCConfig: webrtc.Configuration{}})
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}

	pc, err := engine.CreatePeerConnection(context.Background(), "call-1", "alice@example.com", "bob@example.com", nil)
	if err != nil {
		t.Fatalf("create peer connection failed: %v", err)
	}
	if pc.State != CallStateOffering {
		t.Fatalf("unexpected state: %v", pc.State)
	}

	got, err := engine.GetPeerConnection("call-1", "alice@example.com", "bob@example.com")
	if err != nil || got != pc {
		t.Fatalf("unexpected get result: %v %v", got, err)
	}

	list := engine.ListPeerConnections()
	if len(list) != 1 {
		t.Fatalf("unexpected list size: %d", len(list))
	}

	if err := engine.ClosePeerConnection("call-1", "alice@example.com", "bob@example.com"); err != nil {
		t.Fatalf("close peer connection failed: %v", err)
	}

	if _, err := engine.GetPeerConnection("call-1", "alice@example.com", "bob@example.com"); err == nil {
		t.Fatal("expected missing peer connection")
	}

	if err := engine.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown failed: %v", err)
	}
}

func TestEngineCloseUserSessions(t *testing.T) {
	engine, err := NewEngine(zerolog.Nop(), &Config{WebRTCConfig: webrtc.Configuration{}})
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}

	if _, err := engine.CreatePeerConnection(context.Background(), "call-1", "alice@example.com", "bob@example.com", nil); err != nil {
		t.Fatalf("create peer connection failed: %v", err)
	}
	if _, err := engine.CreatePeerConnection(context.Background(), "call-2", "carol@example.com", "alice@example.com", nil); err != nil {
		t.Fatalf("create peer connection failed: %v", err)
	}

	engine.CloseUserSessions("alice@example.com")
	if len(engine.ListPeerConnections()) != 0 {
		t.Fatal("expected sessions to be closed")
	}
}
