// Package sfu contains transport level helpers used by the room media engine
// (a selective forwarding unit): trickle ICE bookkeeping and keyframe
// (PLI/FIR) management.
//
// The helpers deliberately avoid depending on the collaboration/service layer
// so they stay unit testable without spinning up real peer connections.
package sfu

import (
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// DefaultMaxPendingCandidates bounds how many remote ICE candidates are kept
// while the remote description has not been applied yet. A misbehaving or
// malicious client must not be able to grow this buffer without limit.
const DefaultMaxPendingCandidates = 128

// LocalCandidate describes an ICE candidate gathered by the server side peer
// connection that has to be trickled to the remote peer out of band (the SDP
// answer has already been returned by then).
type LocalCandidate struct {
	RoomID           string  `json:"room_id"`
	ParticipantID    string  `json:"participant_id"`
	Candidate        string  `json:"candidate"`
	SDPMid           *string `json:"sdpMid,omitempty"`
	SDPMLineIndex    *uint16 `json:"sdpMLineIndex,omitempty"`
	UsernameFragment *string `json:"usernameFragment,omitempty"`
	// EndOfCandidates marks the terminal signal emitted once gathering
	// finished. Clients use it to stop waiting for further candidates.
	EndOfCandidates bool `json:"end_of_candidates"`
}

// CandidateSink receives locally gathered candidates. Implementations must not
// block: the room engine dispatches through a bounded queue, but a permanently
// stuck sink still causes candidates to be dropped.
type CandidateSink func(LocalCandidate)

// CandidateBuffer holds remote ICE candidates that arrive before the remote
// description has been negotiated.
//
// With trickle ICE a client starts pushing candidates as soon as it created
// its offer, which regularly beats the offer HTTP request to the server. Pion
// rejects AddICECandidate in that window, so without buffering those early
// (and usually best, because host candidates are gathered first) candidates
// are lost and connection setup falls back to slower reflexive/relay paths.
type CandidateBuffer struct {
	mu          sync.Mutex
	maxPending  int
	remoteReady bool
	pending     []webrtc.ICECandidateInit
	dropped     int
}

// NewCandidateBuffer creates a buffer holding at most maxPending candidates.
// Non positive values fall back to DefaultMaxPendingCandidates.
func NewCandidateBuffer(maxPending int) *CandidateBuffer {
	if maxPending <= 0 {
		maxPending = DefaultMaxPendingCandidates
	}
	return &CandidateBuffer{maxPending: maxPending}
}

// Accept reports whether the candidate may be applied to the peer connection
// right away. When the remote description is still missing the candidate is
// buffered instead and false is returned.
func (b *CandidateBuffer) Accept(candidate webrtc.ICECandidateInit) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.remoteReady {
		return true
	}
	if len(b.pending) >= b.maxPending {
		// Keep the newest candidates: they are the ones the peer still
		// considers viable.
		b.dropped++
		copy(b.pending, b.pending[1:])
		b.pending[len(b.pending)-1] = candidate
		return false
	}
	b.pending = append(b.pending, candidate)
	return false
}

// MarkRemoteReady switches the buffer into pass through mode and returns every
// candidate buffered so far, in arrival order, so the caller can flush them.
// Calling it more than once is safe and returns nil after the first flush.
func (b *CandidateBuffer) MarkRemoteReady() []webrtc.ICECandidateInit {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.remoteReady = true
	if len(b.pending) == 0 {
		return nil
	}
	flushed := b.pending
	b.pending = nil
	return flushed
}

// Pending returns the number of buffered candidates.
func (b *CandidateBuffer) Pending() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.pending)
}

// Dropped returns how many candidates were discarded because the buffer was
// full. Exposed for metrics and troubleshooting.
func (b *CandidateBuffer) Dropped() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// RemoteReady reports whether the remote description has been applied.
func (b *CandidateBuffer) RemoteReady() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.remoteReady
}

// GatherPolicy decides how long answering an offer may block while the local
// ICE agent gathers candidates.
//
// Waiting for full gathering ("vanilla ICE") is simple but costs the caller
// the slowest STUN/TURN probe in the list, which is seconds when a relay is
// unreachable. Bounding the wait turns that worst case into a fixed cost.
type GatherPolicy struct {
	// TrickleTimeout applies when candidates can still be delivered out of
	// band after the answer was returned.
	TrickleTimeout time.Duration
	// BlockingTimeout applies when no trickle transport is wired, so the
	// answer must carry as many candidates as it can get. It exists purely to
	// stop a wedged ICE agent from hanging the HTTP request forever.
	BlockingTimeout time.Duration
}

// DefaultGatherPolicy returns the production defaults.
func DefaultGatherPolicy() GatherPolicy {
	return GatherPolicy{
		TrickleTimeout:  300 * time.Millisecond,
		BlockingTimeout: 10 * time.Second,
	}
}

// Timeout returns the deadline to apply for the given trickle capability.
func (p GatherPolicy) Timeout(trickle bool) time.Duration {
	if trickle {
		if p.TrickleTimeout <= 0 {
			return DefaultGatherPolicy().TrickleTimeout
		}
		return p.TrickleTimeout
	}
	if p.BlockingTimeout <= 0 {
		return DefaultGatherPolicy().BlockingTimeout
	}
	return p.BlockingTimeout
}

// WaitGather blocks until gathering completed or the timeout elapsed and
// reports whether gathering actually completed.
func WaitGather(done <-chan struct{}, timeout time.Duration) bool {
	if done == nil {
		return false
	}
	if timeout <= 0 {
		select {
		case <-done:
			return true
		default:
			return false
		}
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}
