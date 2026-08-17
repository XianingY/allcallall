package media

import (
	"fmt"
	"github.com/pion/webrtc/v4"
)

// SetRenegotiationSink installs the delivery transport for server-initiated
// renegotiation offers and starts the dispatch goroutine on first use.
func (r *RoomEngine) SetRenegotiationSink(sink RenegotiationSink) {
	r.sinkMu.Lock()
	r.renegotiationSink = sink
	r.sinkMu.Unlock()

	if sink == nil {
		return
	}
	r.negDispatchOnce.Do(func() { go r.dispatchRenegotiation() })
}

func (r *RoomEngine) renegotiationSinkLocked() RenegotiationSink {
	r.sinkMu.RLock()
	defer r.sinkMu.RUnlock()
	return r.renegotiationSink
}

func (r *RoomEngine) emitRenegotiationOffer(offer RenegotiationOffer) {
	select {
	case r.renegotiationQueue <- offer:
	default:
		// Queue saturated: drop the oldest pending offer rather than blocking
		// the renegotiation goroutine. A lost offer stalls one participant's
		// view until the next membership change re-triggers it.
		select {
		case <-r.renegotiationQueue:
		default:
		}
		select {
		case r.renegotiationQueue <- offer:
		default:
		}
	}
}

func (r *RoomEngine) dispatchRenegotiation() {
	for {
		select {
		case <-r.negClosed:
			return
		case offer := <-r.renegotiationQueue:
			if sink := r.renegotiationSinkLocked(); sink != nil {
				sink(offer)
			}
		}
	}
}

// requestRenegotiation asks the given participant's peer connection to produce
// a new offer so the client can receive recently added or removed tracks. It is
// safe to call while the caller already holds r.mu because it only touches the
// negotiation tracker and the supplied peer connection; the blocking
// CreateOffer runs in a separate goroutine. The tracker coalesces bursts of
// track changes into a single renegotiation, replaying any change that arrives
// while one is in flight once the outstanding answer is applied.
func (r *RoomEngine) requestRenegotiation(roomID, participantID string, pc *webrtc.PeerConnection) {
	if pc == nil {
		return
	}
	if !r.neg.request(participantID) {
		return
	}
	go r.performRenegotiation(roomID, participantID, pc)
}

func (r *RoomEngine) performRenegotiation(roomID, participantID string, pc *webrtc.PeerConnection) {
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		r.neg.complete(participantID)
		r.logger.Warn().Err(err).Str("room_id", roomID).Str("participant_id", participantID).Msg("failed to create renegotiation offer")
		return
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		r.neg.complete(participantID)
		r.logger.Warn().Err(err).Str("room_id", roomID).Str("participant_id", participantID).Msg("failed to set local renegotiation offer")
		return
	}
	r.emitRenegotiationOffer(RenegotiationOffer{
		RoomID:        roomID,
		ParticipantID: participantID,
		SDP:           offer.SDP,
	})
}

// SetRenegotiationAnswer applies a client's answer to a server-initiated
// renegotiation offer and clears the in-flight flag. If further track changes
// were observed while the offer was outstanding, another renegotiation is
// scheduled immediately.
func (r *RoomEngine) SetRenegotiationAnswer(roomID, participantID, sdp string) error {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	var pc *webrtc.PeerConnection
	if ok {
		if p := room.participants[participantID]; p != nil {
			pc = p.pc
		}
	}
	r.mu.Unlock()
	if pc == nil {
		return fmt.Errorf("participant not found")
	}
	if err := pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}); err != nil {
		r.neg.complete(participantID)
		return err
	}
	if queued := r.neg.complete(participantID); queued {
		r.requestRenegotiation(roomID, participantID, pc)
	}
	return nil
}
