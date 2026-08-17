package media

import (
	"github.com/allcallall/backend/internal/media/sfu"
	"github.com/pion/webrtc/v4"
)

// SetRoomICECandidateSink wires the transport used to trickle server side ICE
// candidates to clients. Passing a non nil sink also switches HandleRoomOffer
// into trickle mode: the answer is returned as soon as the local description
// is set instead of waiting for gathering to finish.
func (e *Engine) SetRoomICECandidateSink(sink sfu.CandidateSink) {
	e.roomEngine.SetICECandidateSink(sink)
}

// RoomKeyframeStats exposes emitted/coalesced keyframe request counters.
func (e *Engine) RoomKeyframeStats() (sent uint64, throttled uint64) {
	return e.roomEngine.KeyframeStats()
}

// RoomBandwidthStats exposes per-participant downlink estimates and forwarding
// decisions collected by the GCC bandwidth estimator. Enabled is false when
// bandwidth estimation was not initialised.
func (e *Engine) RoomBandwidthStats() sfu.BandwidthStats {
	return e.roomEngine.BandwidthStats()
}

// SetRenegotiationSink installs the delivery transport for server-initiated
// renegotiation offers (used for multi-party meetings when tracks are added or
// removed after the initial offer/answer).
func (e *Engine) SetRenegotiationSink(sink RenegotiationSink) {
	e.roomEngine.SetRenegotiationSink(sink)
}

// HandleRenegotiationAnswer applies a client's answer to a server-initiated
// renegotiation offer for the given room participant.
func (e *Engine) HandleRenegotiationAnswer(roomID, participantID, sdp string) error {
	return e.roomEngine.SetRenegotiationAnswer(roomID, participantID, sdp)
}

func (e *Engine) HandleRoomOffer(roomID, participantID, sdp string) (string, error) {
	return e.roomEngine.HandleOffer(roomID, participantID, sdp)
}

func (e *Engine) AddRoomICECandidate(roomID, participantID string, candidate ICECandidateInit) error {
	return e.roomEngine.AddICECandidate(roomID, participantID, webrtc.ICECandidateInit{
		Candidate:        candidate.Candidate,
		SDPMLineIndex:    candidate.SDPMLineIndex,
		SDPMid:           candidate.SDPMid,
		UsernameFragment: stringPtrOrNil(candidate.UsernameFragment),
	})
}

func (e *Engine) LeaveRoomParticipant(roomID, participantID string) error {
	return e.roomEngine.LeaveParticipant(roomID, participantID)
}

func (e *Engine) StartRoomRecording(roomID, baseDir string) error {
	return e.roomEngine.StartRecording(roomID, baseDir)
}

func (e *Engine) StopRoomRecording(roomID string) ([]RecordingArtifact, error) {
	return e.roomEngine.StopRecording(roomID)
}
