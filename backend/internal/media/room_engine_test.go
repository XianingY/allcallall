package media

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/media/sfu"
)

func newTestRoomEngine(t *testing.T) *RoomEngine {
	t.Helper()
	engine, err := NewEngine(zerolog.Nop(), &Config{WebRTCConfig: webrtc.Configuration{}})
	if err != nil {
		t.Fatalf("new engine failed: %v", err)
	}
	t.Cleanup(func() { _ = engine.Shutdown(context.Background()) })
	return engine.roomEngine
}

// clientOffer produces a fully gathered offer from a throwaway peer connection.
func clientOffer(t *testing.T, kind webrtc.RTPCodecType) string {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		t.Fatalf("create client peer connection failed: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	if _, err := pc.AddTransceiverFromKind(kind); err != nil {
		t.Fatalf("add transceiver failed: %v", err)
	}
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		t.Fatalf("create offer failed: %v", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(offer); err != nil {
		t.Fatalf("set local description failed: %v", err)
	}
	<-gatherComplete
	return pc.LocalDescription().SDP
}

func TestRoomEngineHandleOfferRequiresIdentifiers(t *testing.T) {
	engine := newTestRoomEngine(t)

	if _, err := engine.HandleOffer("", "participant-1", "sdp"); err == nil {
		t.Fatal("expected an error when the room id is empty")
	}
	if _, err := engine.HandleOffer("room-1", "  ", "sdp"); err == nil {
		t.Fatal("expected an error when the participant id is empty")
	}
}

func TestRoomEngineBuffersCandidatesArrivingBeforeOffer(t *testing.T) {
	engine := newTestRoomEngine(t)

	// A candidate for an unknown room is still rejected: nothing to buffer for.
	if err := engine.AddICECandidate("room-unknown", "p1", webrtc.ICECandidateInit{Candidate: "candidate:1 1 udp"}); err == nil {
		t.Fatal("expected an error for an unknown room")
	}

	engine.mu.Lock()
	engine.ensureRoomLocked("room-1")
	engine.mu.Unlock()

	// The participant has not offered yet, so the candidate must be buffered
	// instead of rejected, and no peer connection may be allocated for it.
	if err := engine.AddICECandidate("room-1", "p1", webrtc.ICECandidateInit{Candidate: "candidate:1 1 udp"}); err != nil {
		t.Fatalf("expected early candidate to be buffered, got %v", err)
	}

	engine.mu.Lock()
	room := engine.rooms["room-1"]
	participants := len(room.participants)
	buffered := room.candidates["p1"].Pending()
	engine.mu.Unlock()

	if participants != 0 {
		t.Fatalf("expected no peer connection to be created by a stray candidate, got %d", participants)
	}
	if buffered != 1 {
		t.Fatalf("expected 1 buffered candidate, got %d", buffered)
	}
}

func TestRoomEngineCandidateBufferIsBounded(t *testing.T) {
	engine := newTestRoomEngine(t)
	engine.mu.Lock()
	engine.ensureRoomLocked("room-1")
	engine.mu.Unlock()

	for i := 0; i < maxRoomCandidateBuffers; i++ {
		participantID := "p" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		if err := engine.AddICECandidate("room-1", participantID, webrtc.ICECandidateInit{Candidate: "c"}); err != nil {
			t.Fatalf("unexpected error at participant %d: %v", i, err)
		}
	}
	if err := engine.AddICECandidate("room-1", "overflow", webrtc.ICECandidateInit{Candidate: "c"}); err == nil {
		t.Fatal("expected the candidate buffer table to be capped")
	}
}

func TestRoomEngineFlushesBufferedCandidatesOnOffer(t *testing.T) {
	engine := newTestRoomEngine(t)
	engine.mu.Lock()
	engine.ensureRoomLocked("room-1")
	engine.mu.Unlock()

	if err := engine.AddICECandidate("room-1", "p1", webrtc.ICECandidateInit{Candidate: "candidate:1 1 udp 2130706431 127.0.0.1 50000 typ host"}); err != nil {
		t.Fatalf("buffering candidate failed: %v", err)
	}

	if _, err := engine.HandleOffer("room-1", "p1", clientOffer(t, webrtc.RTPCodecTypeAudio)); err != nil {
		t.Fatalf("handle offer failed: %v", err)
	}

	engine.mu.Lock()
	buffer := engine.rooms["room-1"].candidates["p1"]
	engine.mu.Unlock()

	if !buffer.RemoteReady() {
		t.Fatal("expected the buffer to switch to pass-through after the offer")
	}
	if buffer.Pending() != 0 {
		t.Fatalf("expected buffered candidates to be flushed, %d left", buffer.Pending())
	}
}

func TestRoomEngineTrickleModeReturnsAnswerEarly(t *testing.T) {
	engine := newTestRoomEngine(t)

	var mu sync.Mutex
	var received []sfu.LocalCandidate
	done := make(chan struct{})
	var closeOnce sync.Once

	engine.SetICECandidateSink(func(candidate sfu.LocalCandidate) {
		mu.Lock()
		received = append(received, candidate)
		mu.Unlock()
		if candidate.EndOfCandidates {
			closeOnce.Do(func() { close(done) })
		}
	})
	engine.SetGatherPolicy(sfu.GatherPolicy{TrickleTimeout: time.Millisecond})

	if !engine.trickleEnabled() {
		t.Fatal("expected trickle mode to be enabled once a sink is installed")
	}

	answer, err := engine.HandleOffer("room-1", "p1", clientOffer(t, webrtc.RTPCodecTypeAudio))
	if err != nil {
		t.Fatalf("handle offer failed: %v", err)
	}
	if answer == "" {
		t.Fatal("expected a non-empty answer")
	}

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for the end-of-candidates signal")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) == 0 {
		t.Fatal("expected at least the terminal candidate to be trickled")
	}
	last := received[len(received)-1]
	if !last.EndOfCandidates {
		t.Fatalf("expected the last signal to terminate gathering, got %+v", last)
	}
	for _, candidate := range received {
		if candidate.RoomID != "room-1" || candidate.ParticipantID != "p1" {
			t.Fatalf("candidate routed to the wrong peer: %+v", candidate)
		}
	}
}

func TestRoomEngineBlockingModeIsBounded(t *testing.T) {
	engine := newTestRoomEngine(t)
	engine.SetGatherPolicy(sfu.GatherPolicy{BlockingTimeout: 50 * time.Millisecond})

	if engine.trickleEnabled() {
		t.Fatal("expected trickle to stay disabled without a sink")
	}
	if got := engine.gatherTimeout(false); got != 50*time.Millisecond {
		t.Fatalf("expected the configured blocking timeout, got %s", got)
	}

	start := time.Now()
	if _, err := engine.HandleOffer("room-1", "p1", clientOffer(t, webrtc.RTPCodecTypeAudio)); err != nil {
		t.Fatalf("handle offer failed: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("expected the gather wait to be bounded, took %s", elapsed)
	}
}

func TestRoomEngineDropsCandidatesWhenQueueSaturated(t *testing.T) {
	engine := newTestRoomEngine(t)

	release := make(chan struct{})
	engine.SetICECandidateSink(func(sfu.LocalCandidate) { <-release })
	defer close(release)

	// Fill the queue plus one so at least a single candidate is dropped rather
	// than blocking the ICE agent goroutine.
	for i := 0; i < candidateQueueSize+8; i++ {
		engine.emitCandidate(sfu.LocalCandidate{RoomID: "room-1", ParticipantID: "p1"})
	}

	if engine.DroppedTrickleCandidates() == 0 {
		t.Fatal("expected saturated dispatch queue to drop candidates instead of blocking")
	}
}

func TestRoomEngineLeaveParticipantClearsCandidateBuffer(t *testing.T) {
	engine := newTestRoomEngine(t)

	if _, err := engine.HandleOffer("room-1", "p1", clientOffer(t, webrtc.RTPCodecTypeAudio)); err != nil {
		t.Fatalf("handle offer failed: %v", err)
	}
	engine.mu.Lock()
	_, hasBuffer := engine.rooms["room-1"].candidates["p1"]
	engine.mu.Unlock()
	if !hasBuffer {
		t.Fatal("expected a candidate buffer to exist after the offer")
	}

	if err := engine.LeaveParticipant("room-1", "p1"); err != nil {
		t.Fatalf("leave participant failed: %v", err)
	}
	engine.mu.Lock()
	_, roomStillThere := engine.rooms["room-1"]
	engine.mu.Unlock()
	if roomStillThere {
		t.Fatal("expected the empty room to be reclaimed")
	}
}

func TestRoomEngineKeyframeStatsExposed(t *testing.T) {
	engine := newTestRoomEngine(t)

	if sent, throttled := engine.KeyframeStats(); sent != 0 || throttled != 0 {
		t.Fatalf("expected zeroed keyframe counters, got %d / %d", sent, throttled)
	}

	// Requesting a keyframe for an unknown room or track must be a no-op
	// rather than a panic: tracks disappear concurrently with feedback.
	engine.requestKeyframeForTrack("room-missing", "track-missing")
	engine.requestKeyframesForSubscriber("room-missing", "p1")

	if sent, _ := engine.KeyframeStats(); sent != 0 {
		t.Fatalf("expected no keyframe requests to be emitted, got %d", sent)
	}
}
