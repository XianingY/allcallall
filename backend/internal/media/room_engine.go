package media

import (
	"github.com/allcallall/backend/internal/media/sfu"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

// candidateQueueSize bounds the trickle ICE dispatch queue. Candidates are
// produced on Pion's ICE agent goroutine, which must never block on the
// application transport, so a full queue drops the oldest entries instead.
const candidateQueueSize = 512

// maxRoomCandidateBuffers caps how many distinct participants may hold a
// pre-offer candidate buffer in a single room.
const maxRoomCandidateBuffers = 64

// bandwidthSampleWindow is how often a published track's send bitrate is
// re-measured from RTP traffic to feed the bandwidth manager.
const bandwidthSampleWindow = 2 * time.Second

// rtpHeaderSize is the fixed RTP header size used when estimating send bitrate.
const rtpHeaderSize = 12

// RenegotiationOffer is delivered to a single meeting participant when the
// server must renegotiate an established peer connection (for example a new
// publisher joined and the subscriber needs to receive the new track). The
// server acts as the offerer; the client answers and returns the answer over
// the renegotiation HTTP endpoint.
type RenegotiationOffer struct {
	RoomID        string
	ParticipantID string
	SDP           string
}

// RenegotiationSink receives server-initiated renegotiation offers so the
// collaboration layer can forward them to the participant as a realtime event.
type RenegotiationSink func(RenegotiationOffer)

// renegotiationQueueSize bounds the server-initiated renegotiation dispatch
// queue. Renegotiation offers are rare (only on membership or track changes)
// but each one is mandatory for the affected subscriber, so unlike trickle ICE
// the queue only drops the oldest entry as a last resort.
const renegotiationQueueSize = 64

// negotiationTracker serialises renegotiation per participant. Only one offer
// may be in flight at a time; further changes observed while an offer is
// outstanding are recorded as queued and replayed once the answer lands. It is
// pure logic and unit tested without a live PeerConnection.
type negotiationTracker struct {
	mu      sync.Mutex
	pending map[string]bool
	queued  map[string]bool
}

func newNegotiationTracker() *negotiationTracker {
	return &negotiationTracker{pending: make(map[string]bool), queued: make(map[string]bool)}
}

// request marks the participant as having an in-flight negotiation. It returns
// true when a renegotiation should be started now; if one is already pending it
// records the change as queued and returns false.
func (t *negotiationTracker) request(id string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pending[id] {
		t.queued[id] = true
		return false
	}
	t.pending[id] = true
	return true
}

// complete clears the in-flight flag and reports whether another negotiation
// was queued while it was outstanding.
func (t *negotiationTracker) complete(id string) (queued bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	queued = t.queued[id]
	delete(t.pending, id)
	delete(t.queued, id)
	return queued
}

type RecordingArtifact struct {
	ObjectKey       string
	ContentType     string
	DurationSeconds int64
	MetadataJSON    string
}

type RoomEngine struct {
	logger        zerolog.Logger
	defaultConfig webrtc.Configuration
	api           *webrtc.API

	gatherPolicy sfu.GatherPolicy
	keyframes    *sfu.KeyframeRequester
	bw           *sfu.BandwidthController

	sinkMu         sync.RWMutex
	candidateSink  sfu.CandidateSink
	candidateQueue chan sfu.LocalCandidate
	dispatchOnce   sync.Once
	closeOnce      sync.Once
	closed         chan struct{}
	droppedTrickle uint64

	renegotiationSink  RenegotiationSink
	renegotiationQueue chan RenegotiationOffer
	negDispatchOnce    sync.Once
	negClosed          chan struct{}
	neg                *negotiationTracker

	mu    sync.Mutex
	rooms map[string]*mediaRoom
}

type mediaRoom struct {
	id           string
	participants map[string]*roomParticipant
	tracks       map[string]*publishedTrack
	// candidates buffers remote ICE candidates per participant. It is keyed
	// separately from participants because with trickle ICE the first
	// candidate regularly overtakes the offer that creates the participant.
	candidates map[string]*sfu.CandidateBuffer
	recording  *roomRecording
}

type roomParticipant struct {
	id      string
	pc      *webrtc.PeerConnection
	senders map[string]*webrtc.RTPSender
}

type publishedTrack struct {
	key           string
	participantID string
	track         *webrtc.TrackRemote
	local         *webrtc.TrackLocalStaticRTP
	kind          webrtc.RTPCodecType
	ssrc          uint32
}

type roomRecording struct {
	baseDir   string
	startedAt time.Time

	mu        sync.Mutex
	artifacts map[string]*trackRecordingArtifact
}

type trackRecordingArtifact struct {
	path        string
	contentType string
	startedAt   time.Time
	writer      *oggwriter.OggWriter
}

func newRoomEngine(logger zerolog.Logger, cfg webrtc.Configuration, api *webrtc.API, bw *sfu.BandwidthController) *RoomEngine {
	return &RoomEngine{
		logger:             logger.With().Str("component", "room_engine").Logger(),
		defaultConfig:      cfg,
		api:                api,
		gatherPolicy:       sfu.DefaultGatherPolicy(),
		keyframes:          sfu.NewKeyframeRequester(sfu.DefaultKeyframeInterval),
		bw:                 bw,
		candidateQueue:     make(chan sfu.LocalCandidate, candidateQueueSize),
		closed:             make(chan struct{}),
		renegotiationQueue: make(chan RenegotiationOffer, renegotiationQueueSize),
		negClosed:          make(chan struct{}),
		neg:                newNegotiationTracker(),
		rooms:              make(map[string]*mediaRoom),
	}
}
