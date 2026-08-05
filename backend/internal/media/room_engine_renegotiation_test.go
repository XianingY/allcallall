package media

import (
	"testing"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/rs/zerolog"
)

func TestNegotiationTrackerCoalescing(t *testing.T) {
	tracker := newNegotiationTracker()

	if !tracker.request("a") {
		t.Fatal("first request for a participant should start a negotiation")
	}
	// While one is in flight, further requests must be coalesced as queued.
	if tracker.request("a") {
		t.Fatal("second request while pending must not start a new negotiation")
	}
	if tracker.request("a") {
		t.Fatal("third request while pending must not start a new negotiation")
	}

	// Completing reports the queued change and clears state; the caller is
	// then expected to re-arm with another request() (which starts again).
	if queued := tracker.complete("a"); !queued {
		t.Fatal("complete should report the queued change")
	}
	if !tracker.request("a") {
		t.Fatal("after complete, the caller re-arm should start a new negotiation")
	}
	if queued := tracker.complete("a"); queued {
		t.Fatal("second complete should not report another queued change")
	}
	if !tracker.request("a") {
		t.Fatal("after a clean complete a fresh request should start")
	}
}

func TestNegotiationTrackerIndependentParticipants(t *testing.T) {
	tracker := newNegotiationTracker()
	if !tracker.request("a") || !tracker.request("b") {
		t.Fatal("different participants negotiate independently")
	}
	if queued := tracker.complete("a"); queued {
		t.Fatal("a had no queued change")
	}
	if !tracker.request("a") {
		t.Fatal("a can negotiate again after completion")
	}
}

func TestRoomEngineSetRenegotiationAnswerUnknown(t *testing.T) {
	logger := zerolog.Nop()
	e := newRoomEngine(logger, webrtc.Configuration{}, webrtc.NewAPI(), nil)
	if err := e.SetRenegotiationAnswer("999", "1", "v=0\r\n"); err == nil {
		t.Fatal("expected error for unknown room/participant")
	}
}

// TestRoomEngineRequestRenegotiationEmitsOffer verifies the full server-offerer
// path: a track change triggers CreateOffer + SetLocalDescription and the
// resulting offer is delivered through the configured sink.
func TestRoomEngineRequestRenegotiationEmitsOffer(t *testing.T) {
	logger := zerolog.Nop()
	e := newRoomEngine(logger, webrtc.Configuration{}, webrtc.NewAPI(), nil)

	offers := make(chan RenegotiationOffer, 1)
	e.SetRenegotiationSink(func(o RenegotiationOffer) { offers <- o })

	room := e.ensureRoomLocked("1")
	participant, err := e.ensureParticipantLocked(room, "42")
	if err != nil {
		t.Fatalf("ensureParticipantLocked: %v", err)
	}
	// Give the peer connection at least one transceiver so the offer has
	// content; no network is required for CreateOffer offline.
	if _, err := participant.pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	}); err != nil {
		t.Fatalf("AddTransceiverFromKind: %v", err)
	}

	e.requestRenegotiation("1", "42", participant.pc)

	select {
	case offer := <-offers:
		if offer.RoomID != "1" || offer.ParticipantID != "42" {
			t.Fatalf("unexpected offer routing: %+v", offer)
		}
		if offer.SDP == "" {
			t.Fatal("renegotiation offer SDP must not be empty")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for renegotiation offer delivery")
	}
}
