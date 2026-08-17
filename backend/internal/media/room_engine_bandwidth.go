package media

import (
	"github.com/allcallall/backend/internal/media/sfu"
)

// KeyframeStats exposes emitted/coalesced keyframe request counters.
func (r *RoomEngine) KeyframeStats() (sent uint64, throttled uint64) {
	return r.keyframes.Stats()
}

// DroppedTrickleCandidates reports how many locally gathered candidates were
// discarded because the dispatch queue was saturated.
func (r *RoomEngine) DroppedTrickleCandidates() uint64 {
	r.sinkMu.RLock()
	defer r.sinkMu.RUnlock()
	return r.droppedTrickle
}

// Close stops the trickle dispatch goroutine and the renegotiation dispatcher.
func (r *RoomEngine) Close() {
	r.closeOnce.Do(func() {
		close(r.closed)
		close(r.negClosed)
	})
}

// BandwidthStats returns the bandwidth manager snapshot, or a disabled snapshot
// when estimation is not configured.
func (r *RoomEngine) BandwidthStats() sfu.BandwidthStats {
	if r.bw == nil {
		return sfu.BandwidthStats{Enabled: false}
	}
	return r.bw.Manager().Stats()
}

// The following helpers are nil-safe so the forwarding/hot paths can call them
// unconditionally; they are no-ops when bandwidth estimation is disabled.

func (r *RoomEngine) bwUnmarkForwarded(subscriberID, trackKey string) {
	if r.bw == nil {
		return
	}
	r.bw.Manager().UnmarkForwarded(subscriberID, trackKey)
}

func (r *RoomEngine) bwForgetParticipant(participantID string) {
	if r.bw == nil {
		return
	}
	r.bw.ForgetParticipant(participantID)
}

func (r *RoomEngine) bwForgetTrack(trackKey string) {
	if r.bw == nil {
		return
	}
	r.bw.Manager().ForgetTrack(trackKey)
}

func (r *RoomEngine) bwRegisterTrack(trackKey string, bps int) {
	if r.bw == nil {
		return
	}
	r.bw.Manager().RegisterTrack(trackKey, bps)
}
