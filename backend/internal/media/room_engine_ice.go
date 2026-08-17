package media

import (
	"fmt"
	"github.com/allcallall/backend/internal/media/sfu"
	"github.com/pion/webrtc/v4"
	"time"
)

// SetICECandidateSink installs the trickle transport and starts the dispatch
// goroutine on first use.
func (r *RoomEngine) SetICECandidateSink(sink sfu.CandidateSink) {
	r.sinkMu.Lock()
	r.candidateSink = sink
	r.sinkMu.Unlock()

	if sink == nil {
		return
	}
	r.dispatchOnce.Do(func() { go r.dispatchCandidates() })
}

// SetGatherPolicy overrides the ICE gathering deadlines. Zero valued fields
// keep their defaults.
func (r *RoomEngine) SetGatherPolicy(policy sfu.GatherPolicy) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if policy.TrickleTimeout > 0 {
		r.gatherPolicy.TrickleTimeout = policy.TrickleTimeout
	}
	if policy.BlockingTimeout > 0 {
		r.gatherPolicy.BlockingTimeout = policy.BlockingTimeout
	}
}

func (r *RoomEngine) iceCandidateSink() sfu.CandidateSink {
	r.sinkMu.RLock()
	defer r.sinkMu.RUnlock()
	return r.candidateSink
}

func (r *RoomEngine) trickleEnabled() bool {
	return r.iceCandidateSink() != nil
}

// emitCandidate hands a locally gathered candidate to the dispatch queue. It
// never blocks: Pion invokes OnICECandidate from the ICE agent goroutine and a
// slow transport there would stall connectivity checks for every participant.
func (r *RoomEngine) emitCandidate(candidate sfu.LocalCandidate) {
	if !r.trickleEnabled() {
		return
	}
	select {
	case r.candidateQueue <- candidate:
	default:
		r.sinkMu.Lock()
		r.droppedTrickle++
		dropped := r.droppedTrickle
		r.sinkMu.Unlock()
		r.logger.Warn().
			Str("room_id", candidate.RoomID).
			Str("participant_id", candidate.ParticipantID).
			Uint64("dropped_total", dropped).
			Msg("trickle ice queue saturated, dropping candidate")
	}
}

func (r *RoomEngine) dispatchCandidates() {
	for {
		select {
		case <-r.closed:
			return
		case candidate := <-r.candidateQueue:
			sink := r.iceCandidateSink()
			if sink == nil {
				continue
			}
			sink(candidate)
		}
	}
}

func (r *RoomEngine) gatherTimeout(trickle bool) time.Duration {
	r.mu.Lock()
	policy := r.gatherPolicy
	r.mu.Unlock()
	return policy.Timeout(trickle)
}

func (r *RoomEngine) AddICECandidate(roomID, participantID string, candidate webrtc.ICECandidateInit) error {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("room not found")
	}
	// With trickle ICE the first candidates frequently overtake the offer, so
	// buffer them rather than rejecting them. No peer connection is created
	// here: a stray candidate must not be able to allocate media resources.
	buffer, err := r.ensureCandidateBufferLocked(room, participantID)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	participant := room.participants[participantID]
	r.mu.Unlock()

	if !buffer.Accept(candidate) {
		return nil
	}
	if participant == nil {
		return fmt.Errorf("participant not found")
	}
	return participant.pc.AddICECandidate(candidate)
}

func (r *RoomEngine) ensureCandidateBufferLocked(room *mediaRoom, participantID string) (*sfu.CandidateBuffer, error) {
	if buffer, ok := room.candidates[participantID]; ok {
		return buffer, nil
	}
	if len(room.candidates) >= maxRoomCandidateBuffers {
		return nil, fmt.Errorf("too many pending ice candidate buffers for room")
	}
	buffer := sfu.NewCandidateBuffer(sfu.DefaultMaxPendingCandidates)
	room.candidates[participantID] = buffer
	return buffer, nil
}
