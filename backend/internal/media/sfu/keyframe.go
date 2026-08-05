package sfu

import (
	"sync"
	"time"

	"github.com/pion/rtcp"
)

// DefaultKeyframeInterval is the minimum spacing between two keyframe requests
// targeting the same media source.
//
// Every subscriber joining a room needs an IDR frame before it can decode
// anything, otherwise it stares at a black tile until the publisher happens to
// emit its next keyframe (often several seconds). Requesting one is therefore
// mandatory, but an unthrottled request per subscriber collapses the
// publisher's encoder bitrate, so requests are coalesced.
const DefaultKeyframeInterval = 500 * time.Millisecond

// RTCPWriter is the subset of *webrtc.PeerConnection used to emit feedback.
type RTCPWriter interface {
	WriteRTCP(packets []rtcp.Packet) error
}

// KeyframeRequester emits rate limited Picture Loss Indications towards
// publishers. It is safe for concurrent use.
type KeyframeRequester struct {
	mu        sync.Mutex
	interval  time.Duration
	last      map[uint32]time.Time
	now       func() time.Time
	sent      uint64
	throttled uint64
}

// NewKeyframeRequester creates a requester coalescing requests per SSRC.
// Non positive intervals fall back to DefaultKeyframeInterval.
func NewKeyframeRequester(interval time.Duration) *KeyframeRequester {
	if interval <= 0 {
		interval = DefaultKeyframeInterval
	}
	return &KeyframeRequester{
		interval: interval,
		last:     make(map[uint32]time.Time),
		now:      time.Now,
	}
}

// allow reports whether a request for ssrc is due, recording the decision.
func (k *KeyframeRequester) allow(ssrc uint32) bool {
	k.mu.Lock()
	defer k.mu.Unlock()

	now := k.now()
	if last, ok := k.last[ssrc]; ok && now.Sub(last) < k.interval {
		k.throttled++
		return false
	}
	k.last[ssrc] = now
	k.sent++
	return true
}

// Request sends a PLI for ssrc unless one was sent too recently. It reports
// whether a packet was actually written.
func (k *KeyframeRequester) Request(writer RTCPWriter, ssrc uint32) (bool, error) {
	if writer == nil || ssrc == 0 {
		return false, nil
	}
	if !k.allow(ssrc) {
		return false, nil
	}
	if err := writer.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{MediaSSRC: ssrc},
	}); err != nil {
		return false, err
	}
	return true, nil
}

// Forget drops the bookkeeping for a source that went away so the map does not
// grow for the lifetime of the process.
func (k *KeyframeRequester) Forget(ssrc uint32) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.last, ssrc)
}

// Stats returns how many requests were emitted and how many were coalesced.
func (k *KeyframeRequester) Stats() (sent uint64, throttled uint64) {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.sent, k.throttled
}

// Tracked returns the number of sources currently held in the rate limiter.
func (k *KeyframeRequester) Tracked() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.last)
}

// IsKeyframeRequest reports whether an RTCP packet received from a subscriber
// asks for a fresh keyframe.
func IsKeyframeRequest(packet rtcp.Packet) bool {
	switch packet.(type) {
	case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
		return true
	default:
		return false
	}
}

// ContainsKeyframeRequest reports whether any packet in the compound asks for a
// keyframe.
func ContainsKeyframeRequest(packets []rtcp.Packet) bool {
	for _, packet := range packets {
		if IsKeyframeRequest(packet) {
			return true
		}
	}
	return false
}
