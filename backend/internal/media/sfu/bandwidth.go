package sfu

import (
	"sync"

	"github.com/pion/interceptor/pkg/cc"
	"github.com/pion/interceptor/pkg/gcc"
)

// BandwidthController couples the Pion GCC bandwidth estimator of every peer
// connection to a BandwidthManager. Pion v4 invokes the interceptor factory
// synchronously while a new PeerConnection is being constructed (it calls
// api.interceptorRegistry.Build("") inside webrtc.NewPeerConnection), so we
// capture the per-connection estimator by stashing the participant id that is
// about to be created (see RoomEngine.ensureParticipantLocked) and binding it
// the moment the estimator is produced.
type BandwidthController struct {
	mu sync.Mutex
	// pending is the participant id whose PeerConnection is currently being
	// constructed. It is consumed by the next estimator Factory call.
	pending string
	// manager holds the pure forwarding/budget logic.
	manager *BandwidthManager
	// estimators keeps the live GCC estimators keyed by participant id so they
	// can be queried or closed on departure.
	estimators map[string]cc.BandwidthEstimator
}

// NewBandwidthController creates a controller backed by a fresh manager.
func NewBandwidthController() *BandwidthController {
	return &BandwidthController{
		manager:    NewBandwidthManager(),
		estimators: make(map[string]cc.BandwidthEstimator),
	}
}

// Manager returns the underlying pure bandwidth manager.
func (c *BandwidthController) Manager() *BandwidthManager {
	if c == nil {
		return nil
	}
	return c.manager
}

// SetPending records the participant id whose PeerConnection is about to be
// constructed. The next estimator produced by Factory is bound to it.
func (c *BandwidthController) SetPending(participantID string) {
	c.mu.Lock()
	c.pending = participantID
	c.mu.Unlock()
}

// ClearPending forgets any pending participant binding. It is safe to call even
// when no estimator was produced (e.g. the API had no GCC interceptor).
func (c *BandwidthController) ClearPending() {
	c.mu.Lock()
	c.pending = ""
	c.mu.Unlock()
}

// Factory returns the BandwidthEstimatorFactory used by the GCC interceptor.
// Each invocation builds a SendSideBWE estimator and, if a participant is
// pending, binds it to the manager so its target-bitrate updates flow in.
func (c *BandwidthController) Factory() cc.BandwidthEstimatorFactory {
	return func() (cc.BandwidthEstimator, error) {
		est, err := gcc.NewSendSideBWE()
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		pid := c.pending
		c.pending = ""
		c.mu.Unlock()
		if pid != "" {
			c.registerEstimator(pid, est)
		}
		return est, nil
	}
}

func (c *BandwidthController) registerEstimator(participantID string, est cc.BandwidthEstimator) {
	c.mu.Lock()
	c.estimators[participantID] = est
	c.mu.Unlock()
	pid := participantID
	est.OnTargetBitrateChange(func(bitrate int) {
		c.manager.SetDownlink(pid, bitrate)
	})
}

// ForgetParticipant drops all bandwidth state for a participant: its estimator,
// downlink estimate and forwarding bookkeeping.
func (c *BandwidthController) ForgetParticipant(participantID string) {
	c.mu.Lock()
	delete(c.estimators, participantID)
	c.mu.Unlock()
	c.manager.ForgetParticipant(participantID)
}

// GetTargetBitrate returns the latest estimated downlink bitrate (bps) for a
// participant, or 0 if none is known yet.
func (c *BandwidthController) GetTargetBitrate(participantID string) int {
	return c.manager.GetDownlink(participantID)
}

// BandwidthManager tracks per-participant downlink estimates produced by GCC and
// per-track nominal send bitrates, and decides whether a published track should
// be forwarded to a subscriber given the available bandwidth budget. It is
// intentionally free of any Pion dependency so it can be unit tested in
// isolation and reused by higher level forwarding policies.
type BandwidthManager struct {
	mu sync.RWMutex

	// downlink is the latest estimated available downlink (server -> client)
	// bitrate in bits per second for each subscriber.
	downlink map[string]int

	// trackBitrate is the measured/declared send bitrate (bps) of each
	// published track, keyed by track key.
	trackBitrate map[string]int

	// forwarded maps a subscriber id to the set of track keys currently
	// forwarded to it.
	forwarded map[string]map[string]struct{}

	// estimates keeps a short history of recent downlink estimates per
	// participant for observability.
	estimates map[string][]int

	// throttled counts how many track-forwarding decisions were denied because
	// the subscriber budget was exceeded.
	throttled int

	// headroomPct is the fraction of the estimated downlink reserved as a
	// safety margin (0-100).
	headroomPct int

	// minForwardBitrate is the floor (bps) below which a track is always
	// forwarded even if it would exceed the budget (e.g. audio).
	minForwardBitrate int
}

// DefaultHeadroomPct is the default safety margin applied to downlink estimates.
const DefaultHeadroomPct = 15

// DefaultMinForwardBitrate is the default floor (bps, ~80 kbps) below which a
// track is always forwarded regardless of budget.
const DefaultMinForwardBitrate = 80_000

// NewBandwidthManager creates a manager with sensible defaults.
func NewBandwidthManager() *BandwidthManager {
	return &BandwidthManager{
		downlink:          make(map[string]int),
		trackBitrate:      make(map[string]int),
		forwarded:         make(map[string]map[string]struct{}),
		estimates:         make(map[string][]int),
		headroomPct:       DefaultHeadroomPct,
		minForwardBitrate: DefaultMinForwardBitrate,
	}
}

// RegisterTrack records the current send bitrate (bps) of a published track.
func (m *BandwidthManager) RegisterTrack(trackKey string, bps int) {
	m.mu.Lock()
	m.trackBitrate[trackKey] = bps
	m.mu.Unlock()
}

// ForgetTrack removes a published track from bookkeeping and from every
// subscriber's forwarded set.
func (m *BandwidthManager) ForgetTrack(trackKey string) {
	m.mu.Lock()
	delete(m.trackBitrate, trackKey)
	for sub, set := range m.forwarded {
		delete(set, trackKey)
		if len(set) == 0 {
			delete(m.forwarded, sub)
		}
	}
	m.mu.Unlock()
}

// SetDownlink records the latest estimated downlink bitrate (bps) for a
// subscriber and appends to its estimate history (capped).
func (m *BandwidthManager) SetDownlink(participantID string, bps int) {
	m.mu.Lock()
	m.downlink[participantID] = bps
	hist := m.estimates[participantID]
	hist = append(hist, bps)
	if len(hist) > 10 {
		hist = hist[len(hist)-10:]
	}
	m.estimates[participantID] = hist
	m.mu.Unlock()
}

// GetDownlink returns the latest estimated downlink (bps), or 0 if unknown.
func (m *BandwidthManager) GetDownlink(participantID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.downlink[participantID]
}

// MarkForwarded records that a track is being forwarded to a subscriber.
func (m *BandwidthManager) MarkForwarded(subscriberID, trackKey string) {
	m.mu.Lock()
	set := m.forwarded[subscriberID]
	if set == nil {
		set = make(map[string]struct{})
		m.forwarded[subscriberID] = set
	}
	set[trackKey] = struct{}{}
	m.mu.Unlock()
}

// UnmarkForwarded forgets a single forwarded track for a subscriber.
func (m *BandwidthManager) UnmarkForwarded(subscriberID, trackKey string) {
	m.mu.Lock()
	set := m.forwarded[subscriberID]
	if set != nil {
		delete(set, trackKey)
		if len(set) == 0 {
			delete(m.forwarded, subscriberID)
		}
	}
	m.mu.Unlock()
}

// ShouldForward decides whether a track may be forwarded to a subscriber given
// its downlink budget. Tracks at or below the minimum forward bitrate (e.g.
// audio) are always allowed; unknown budgets are treated optimistically (allow)
// so that connectivity is never degraded before an estimate arrives.
func (m *BandwidthManager) ShouldForward(subscriberID, trackKey string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	downlink := m.downlink[subscriberID]
	if downlink <= 0 {
		return true
	}
	trackBr := m.trackBitrate[trackKey]
	if trackBr <= 0 || trackBr <= m.minForwardBitrate {
		return true
	}

	used := 0
	for tk := range m.forwarded[subscriberID] {
		used += m.trackBitrate[tk]
	}
	available := downlink - downlink*m.headroomPct/100
	return used+trackBr <= available
}

// ForwardBudget returns the remaining downlink budget (bps) for a subscriber
// after accounting for already-forwarded tracks and the safety headroom.
func (m *BandwidthManager) ForwardBudget(subscriberID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	downlink := m.downlink[subscriberID]
	if downlink <= 0 {
		return 0
	}
	used := 0
	for tk := range m.forwarded[subscriberID] {
		used += m.trackBitrate[tk]
	}
	available := downlink - downlink*m.headroomPct/100
	left := available - used
	if left < 0 {
		return 0
	}
	return left
}

// RecordThrottled increments the counter of denied forwarding decisions.
func (m *BandwidthManager) RecordThrottled() {
	m.mu.Lock()
	m.throttled++
	m.mu.Unlock()
}

// ForgetParticipant removes all state for a participant.
func (m *BandwidthManager) ForgetParticipant(participantID string) {
	m.mu.Lock()
	delete(m.downlink, participantID)
	delete(m.estimates, participantID)
	delete(m.forwarded, participantID)
	m.mu.Unlock()
}

// BandwidthStats is a snapshot of the manager for observability.
type BandwidthStats struct {
	Enabled      bool
	Participants int
	Tracks       int
	Forwarded    int
	Throttled    int
	DownlinkBps  map[string]int
	TrackBps     map[string]int
}

// Stats returns an observability snapshot.
func (m *BandwidthManager) Stats() BandwidthStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	downlink := make(map[string]int, len(m.downlink))
	for k, v := range m.downlink {
		downlink[k] = v
	}
	trackBps := make(map[string]int, len(m.trackBitrate))
	for k, v := range m.trackBitrate {
		trackBps[k] = v
	}
	forwarded := 0
	for _, set := range m.forwarded {
		forwarded += len(set)
	}
	return BandwidthStats{
		Enabled:      true,
		Participants: len(m.downlink),
		Tracks:       len(m.trackBitrate),
		Forwarded:    forwarded,
		Throttled:    m.throttled,
		DownlinkBps:  downlink,
		TrackBps:     trackBps,
	}
}
