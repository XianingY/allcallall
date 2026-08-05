package collaboration

import (
	"context"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/media"
	"github.com/allcallall/backend/internal/media/sfu"
)

type capturingPublisher struct {
	mu     sync.Mutex
	events []RealtimeEventRecord
}

func (p *capturingPublisher) PublishToUser(_ context.Context, event RealtimeEventRecord) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, event)
	return nil
}

func (p *capturingPublisher) byEvent(name string) []RealtimeEventRecord {
	p.mu.Lock()
	defer p.mu.Unlock()
	matched := make([]RealtimeEventRecord, 0, len(p.events))
	for _, event := range p.events {
		if event.Event == name {
			matched = append(matched, event)
		}
	}
	return matched
}

func TestRoomOrgRegistry(t *testing.T) {
	registry := newRoomOrgRegistry()

	if _, ok := registry.lookup(1); ok {
		t.Fatal("expected an empty registry to miss")
	}

	registry.remember(1, 42)
	got, ok := registry.lookup(1)
	if !ok || got != 42 {
		t.Fatalf("expected organization 42, got %d (ok=%v)", got, ok)
	}

	// Zero identifiers are meaningless and must not be stored.
	registry.remember(0, 42)
	registry.remember(2, 0)
	if registry.size() != 1 {
		t.Fatalf("expected zero ids to be rejected, size=%d", registry.size())
	}

	registry.forget(1)
	if _, ok := registry.lookup(1); ok {
		t.Fatal("expected the entry to be forgotten")
	}

	var nilRegistry *roomOrgRegistry
	nilRegistry.remember(1, 1)
	nilRegistry.forget(1)
	if _, ok := nilRegistry.lookup(1); ok || nilRegistry.size() != 0 {
		t.Fatal("expected a nil registry to degrade gracefully")
	}
}

func TestPublishRoomICECandidateRoutesToParticipant(t *testing.T) {
	svc, _, _ := newServiceTestEnv(t)
	publisher := &capturingPublisher{}
	svc.WithPublisher(publisher)
	svc.roomOrgs.remember(77, 9)

	mid := "0"
	index := uint16(0)
	ufrag := "abcd"
	svc.publishRoomICECandidate(sfu.LocalCandidate{
		RoomID:           "77",
		ParticipantID:    "5",
		Candidate:        "candidate:1 1 udp 2130706431 127.0.0.1 50000 typ host",
		SDPMid:           &mid,
		SDPMLineIndex:    &index,
		UsernameFragment: &ufrag,
	})

	events := publisher.byEvent(RoomICECandidateEvent)
	if len(events) != 1 {
		t.Fatalf("expected 1 candidate event, got %d", len(events))
	}
	if events[0].UserID != 5 {
		t.Fatalf("expected the candidate to target user 5, got %d", events[0].UserID)
	}
	if events[0].OrganizationID != 9 {
		t.Fatalf("expected organization 9, got %d", events[0].OrganizationID)
	}
}

func TestPublishRoomICECandidateDropsUnknownRoutes(t *testing.T) {
	svc, _, _ := newServiceTestEnv(t)
	publisher := &capturingPublisher{}
	svc.WithPublisher(publisher)

	// Unknown room: the peer connection is gone, dropping is correct.
	svc.publishRoomICECandidate(sfu.LocalCandidate{RoomID: "1", ParticipantID: "2", Candidate: "c"})
	// Non numeric identifiers can never map to a user or room.
	svc.roomOrgs.remember(1, 1)
	svc.publishRoomICECandidate(sfu.LocalCandidate{RoomID: "not-a-room", ParticipantID: "2"})
	svc.publishRoomICECandidate(sfu.LocalCandidate{RoomID: "1", ParticipantID: "not-a-user"})

	if got := len(publisher.byEvent(RoomICECandidateEvent)); got != 0 {
		t.Fatalf("expected unroutable candidates to be dropped, got %d events", got)
	}
}

func TestWithTrickleICEWiresSinkAndAnswerFlag(t *testing.T) {
	svc, db, _ := newServiceTestEnv(t)
	ctx := context.Background()
	publisher := &capturingPublisher{}
	svc.WithPublisher(publisher)

	if svc.TrickleICEEnabled() {
		t.Fatal("expected trickle ice to be disabled by default")
	}

	engine, err := media.NewEngine(zerolog.Nop(), &media.Config{WebRTCConfig: webrtc.Configuration{}})
	if err != nil {
		t.Fatalf("create media engine failed: %v", err)
	}
	t.Cleanup(func() { _ = engine.Shutdown(ctx) })

	svc.WithTrickleICE(true)
	svc.WithMediaEngine(engine)
	if !svc.TrickleICEEnabled() {
		t.Fatal("expected trickle ice to be enabled")
	}

	owner := createTestUser(t, db, "trickle-owner@example.com", "Owner")
	org, err := svc.CreateOrganization(ctx, owner.ID, "Workspace")
	if err != nil {
		t.Fatalf("create organization failed: %v", err)
	}
	roomState, err := svc.CreateRoom(ctx, org.ID, owner.ID, CreateRoomInput{Title: "Standup"})
	if err != nil {
		t.Fatalf("create room failed: %v", err)
	}

	clientPC, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("create client peer connection failed: %v", err)
	}
	defer func() { _ = clientPC.Close() }()
	if _, err := clientPC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio); err != nil {
		t.Fatalf("add transceiver failed: %v", err)
	}
	offer, err := clientPC.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer failed: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(clientPC)
	if err := clientPC.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description failed: %v", err)
	}
	<-gatherComplete

	result, err := svc.HandleRoomOffer(ctx, org.ID, owner.ID, roomState.Room.ID, clientPC.LocalDescription().SDP)
	if err != nil {
		t.Fatalf("handle room offer failed: %v", err)
	}
	if !result.TrickleICE {
		t.Fatal("expected the answer to advertise trickle ice")
	}
	if result.Answer.SDP == "" {
		t.Fatal("expected a non-empty answer sdp")
	}
	if got, ok := svc.roomOrgs.lookup(roomState.Room.ID); !ok || got != org.ID {
		t.Fatalf("expected the room to be mapped to organization %d, got %d (ok=%v)", org.ID, got, ok)
	}
}

func TestParseBoolEnv(t *testing.T) {
	cases := map[string]bool{"1": true, "true": true, "YES": true, "on": true, "0": false, "false": false, "off": false}
	for raw, want := range cases {
		t.Setenv("ROOM_TRICKLE_ICE_TEST", raw)
		if got := parseBoolEnv("ROOM_TRICKLE_ICE_TEST", !want); got != want {
			t.Fatalf("parseBoolEnv(%q) = %v, want %v", raw, got, want)
		}
	}

	t.Setenv("ROOM_TRICKLE_ICE_TEST", "maybe")
	if !parseBoolEnv("ROOM_TRICKLE_ICE_TEST", true) {
		t.Fatal("expected an unparsable value to fall back to the default")
	}
	if parseBoolEnv("ROOM_TRICKLE_ICE_ABSENT", false) {
		t.Fatal("expected a missing variable to fall back to the default")
	}
}
