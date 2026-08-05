package media

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media/oggwriter"
	"github.com/rs/zerolog"

	"github.com/allcallall/backend/internal/media/sfu"
)

// candidateQueueSize bounds the trickle ICE dispatch queue. Candidates are
// produced on Pion's ICE agent goroutine, which must never block on the
// application transport, so a full queue drops the oldest entries instead.
const candidateQueueSize = 512

// maxRoomCandidateBuffers caps how many distinct participants may hold a
// pre-offer candidate buffer in a single room.
const maxRoomCandidateBuffers = 64

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

	sinkMu         sync.RWMutex
	candidateSink  sfu.CandidateSink
	candidateQueue chan sfu.LocalCandidate
	dispatchOnce   sync.Once
	closeOnce      sync.Once
	closed         chan struct{}
	droppedTrickle uint64

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

func newRoomEngine(logger zerolog.Logger, cfg webrtc.Configuration, api *webrtc.API) *RoomEngine {
	return &RoomEngine{
		logger:         logger.With().Str("component", "room_engine").Logger(),
		defaultConfig:  cfg,
		api:            api,
		gatherPolicy:   sfu.DefaultGatherPolicy(),
		keyframes:      sfu.NewKeyframeRequester(sfu.DefaultKeyframeInterval),
		candidateQueue: make(chan sfu.LocalCandidate, candidateQueueSize),
		closed:         make(chan struct{}),
		rooms:          make(map[string]*mediaRoom),
	}
}

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

// Close stops the trickle dispatch goroutine.
func (r *RoomEngine) Close() {
	r.closeOnce.Do(func() { close(r.closed) })
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

func (r *RoomEngine) HandleOffer(roomID, participantID, sdp string) (string, error) {
	if strings.TrimSpace(roomID) == "" || strings.TrimSpace(participantID) == "" {
		return "", fmt.Errorf("room_id and participant_id are required")
	}

	r.mu.Lock()
	room := r.ensureRoomLocked(roomID)
	participant, err := r.ensureParticipantLocked(room, participantID)
	if err != nil {
		r.mu.Unlock()
		return "", err
	}
	candidateBuffer, err := r.ensureCandidateBufferLocked(room, participantID)
	if err != nil {
		r.mu.Unlock()
		return "", err
	}
	for _, track := range room.tracks {
		if track.participantID == participantID {
			continue
		}
		r.attachTrackLocked(room, participant, track)
	}
	r.mu.Unlock()

	if err := participant.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return "", err
	}

	// Candidates trickled in before the offer reached us could not be applied
	// yet; replay them now that a remote description exists.
	for _, pending := range candidateBuffer.MarkRemoteReady() {
		if addErr := participant.pc.AddICECandidate(pending); addErr != nil {
			r.logger.Warn().Err(addErr).
				Str("room_id", roomID).
				Str("participant_id", participantID).
				Msg("failed to apply buffered ice candidate")
		}
	}

	answer, err := participant.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}

	gatherComplete := webrtc.GatheringCompletePromise(participant.pc)
	if err := participant.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}

	// Waiting for full gathering costs the slowest STUN/TURN probe in the ICE
	// server list. When a trickle transport is wired we only wait long enough
	// to pick up host candidates and deliver the rest asynchronously.
	trickle := r.trickleEnabled()
	if !sfu.WaitGather(gatherComplete, r.gatherTimeout(trickle)) {
		r.logger.Debug().
			Str("room_id", roomID).
			Str("participant_id", participantID).
			Bool("trickle", trickle).
			Msg("returning sdp answer before ice gathering completed")
	}

	// Inject Opus DTX (Discontinuous Transmission) to save bandwidth during silence
	// This is a commercial-grade optimization (Pillar A)
	finalSDP := participant.pc.LocalDescription().SDP
	finalSDP = strings.ReplaceAll(finalSDP, "useinbandfec=1", "useinbandfec=1;usedtx=1")

	return finalSDP, nil
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

func (r *RoomEngine) LeaveParticipant(roomID, participantID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	room, ok := r.rooms[roomID]
	if !ok {
		return nil
	}
	r.removeParticipantLocked(room, participantID)
	if len(room.participants) == 0 {
		delete(r.rooms, roomID)
	}
	return nil
}

func (r *RoomEngine) StartRecording(roomID, baseDir string) error {
	if strings.TrimSpace(roomID) == "" {
		return fmt.Errorf("room_id is required")
	}
	if strings.TrimSpace(baseDir) == "" {
		return fmt.Errorf("recording base dir is required")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	room := r.ensureRoomLocked(roomID)
	room.recording = &roomRecording{
		baseDir:   baseDir,
		startedAt: time.Now(),
		artifacts: make(map[string]*trackRecordingArtifact),
	}
	return nil
}

func (r *RoomEngine) StopRecording(roomID string) ([]RecordingArtifact, error) {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok || room.recording == nil {
		r.mu.Unlock()
		return nil, nil
	}
	recording := room.recording
	room.recording = nil
	r.mu.Unlock()

	recording.mu.Lock()
	defer recording.mu.Unlock()

	artifacts := make([]RecordingArtifact, 0, len(recording.artifacts))
	for key, artifact := range recording.artifacts {
		if artifact.writer != nil {
			if err := artifact.writer.Close(); err != nil {
				r.logger.Warn().Err(err).Str("path", artifact.path).Msg("failed to close recording writer")
			}
		}
		artifacts = append(artifacts, RecordingArtifact{
			ObjectKey:       artifact.path,
			ContentType:     artifact.contentType,
			DurationSeconds: int64(time.Since(artifact.startedAt).Seconds()),
			MetadataJSON:    fmt.Sprintf(`{"track_key":"%s"}`, key),
		})
	}

	return artifacts, nil
}

func (r *RoomEngine) ensureRoomLocked(roomID string) *mediaRoom {
	room, ok := r.rooms[roomID]
	if ok {
		return room
	}
	room = &mediaRoom{
		id:           roomID,
		participants: make(map[string]*roomParticipant),
		tracks:       make(map[string]*publishedTrack),
		candidates:   make(map[string]*sfu.CandidateBuffer),
	}
	r.rooms[roomID] = room
	return room
}

func (r *RoomEngine) ensureParticipantLocked(room *mediaRoom, participantID string) (*roomParticipant, error) {
	if participant, ok := room.participants[participantID]; ok {
		return participant, nil
	}

	var pc *webrtc.PeerConnection
	var err error
	if r.api != nil {
		pc, err = r.api.NewPeerConnection(r.defaultConfig)
	} else {
		pc, err = webrtc.NewPeerConnection(r.defaultConfig)
	}
	if err != nil {
		return nil, err
	}

	participant := &roomParticipant{
		id:      participantID,
		pc:      pc,
		senders: make(map[string]*webrtc.RTPSender),
	}

	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			r.emitCandidate(sfu.LocalCandidate{
				RoomID:          room.id,
				ParticipantID:   participantID,
				EndOfCandidates: true,
			})
			return
		}
		init := candidate.ToJSON()
		r.emitCandidate(sfu.LocalCandidate{
			RoomID:           room.id,
			ParticipantID:    participantID,
			Candidate:        init.Candidate,
			SDPMid:           init.SDPMid,
			SDPMLineIndex:    init.SDPMLineIndex,
			UsernameFragment: init.UsernameFragment,
		})
	})

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		if state == webrtc.PeerConnectionStateConnected {
			// A freshly connected subscriber cannot decode anything until the
			// publishers emit an IDR frame, so ask for one right away instead
			// of showing a black tile until the next natural keyframe.
			go r.requestKeyframesForSubscriber(room.id, participantID)
			return
		}
		if state == webrtc.PeerConnectionStateClosed || state == webrtc.PeerConnectionStateFailed {
			r.mu.Lock()
			defer r.mu.Unlock()
			if roomRef, ok := r.rooms[room.id]; ok {
				r.removeParticipantLocked(roomRef, participantID)
				if len(roomRef.participants) == 0 {
					delete(r.rooms, room.id)
				}
			}
		}
	})

	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		r.handleRemoteTrack(room.id, participantID, track)
	})

	room.participants[participantID] = participant
	return participant, nil
}

// attachTrackLocked subscribes a participant to a published track. The caller
// must hold r.mu.
func (r *RoomEngine) attachTrackLocked(room *mediaRoom, subscriber *roomParticipant, published *publishedTrack) bool {
	if subscriber == nil || published == nil || subscriber.id == published.participantID {
		return false
	}
	if _, ok := subscriber.senders[published.key]; ok {
		return false
	}
	sender, err := subscriber.pc.AddTrack(published.local)
	if err != nil {
		r.logger.Warn().Err(err).
			Str("room_id", room.id).
			Str("participant_id", subscriber.id).
			Str("track_key", published.key).
			Msg("failed to attach relay track to participant")
		return false
	}
	subscriber.senders[published.key] = sender
	go r.forwardSubscriberFeedback(room.id, published.key, sender)
	return true
}

// forwardSubscriberFeedback drains RTCP coming back from a subscriber and
// relays keyframe requests to the publisher. Draining is mandatory even when
// nothing is forwarded: an unread sender stalls the interceptor chain.
func (r *RoomEngine) forwardSubscriberFeedback(roomID, trackKey string, sender *webrtc.RTPSender) {
	for {
		packets, _, err := sender.ReadRTCP()
		if err != nil {
			return
		}
		if !sfu.ContainsKeyframeRequest(packets) {
			continue
		}
		r.requestKeyframeForTrack(roomID, trackKey)
	}
}

type keyframeTarget struct {
	pc   *webrtc.PeerConnection
	ssrc uint32
}

func (r *RoomEngine) requestKeyframesForSubscriber(roomID, subscriberID string) {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok {
		r.mu.Unlock()
		return
	}
	targets := make([]keyframeTarget, 0, len(room.tracks))
	for _, track := range room.tracks {
		if track.participantID == subscriberID || track.kind != webrtc.RTPCodecTypeVideo {
			continue
		}
		publisher, ok := room.participants[track.participantID]
		if !ok {
			continue
		}
		targets = append(targets, keyframeTarget{pc: publisher.pc, ssrc: track.ssrc})
	}
	r.mu.Unlock()

	r.sendKeyframeRequests(roomID, targets)
}

func (r *RoomEngine) requestKeyframeForTrack(roomID, trackKey string) {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok {
		r.mu.Unlock()
		return
	}
	track, ok := room.tracks[trackKey]
	if !ok || track.kind != webrtc.RTPCodecTypeVideo {
		r.mu.Unlock()
		return
	}
	publisher, ok := room.participants[track.participantID]
	if !ok {
		r.mu.Unlock()
		return
	}
	target := keyframeTarget{pc: publisher.pc, ssrc: track.ssrc}
	r.mu.Unlock()

	r.sendKeyframeRequests(roomID, []keyframeTarget{target})
}

func (r *RoomEngine) sendKeyframeRequests(roomID string, targets []keyframeTarget) {
	for _, target := range targets {
		if _, err := r.keyframes.Request(target.pc, target.ssrc); err != nil {
			r.logger.Debug().Err(err).
				Str("room_id", roomID).
				Uint32("ssrc", target.ssrc).
				Msg("failed to send keyframe request")
		}
	}
}

func (r *RoomEngine) removeParticipantLocked(room *mediaRoom, participantID string) {
	participant, ok := room.participants[participantID]
	if !ok {
		return
	}
	if err := participant.pc.Close(); err != nil {
		r.logger.Warn().Err(err).Str("participant_id", participantID).Msg("failed to close peer connection on participant removal")
	}
	delete(room.participants, participantID)
	delete(room.candidates, participantID)

	for key, track := range room.tracks {
		if track.participantID != participantID {
			continue
		}
		for _, other := range room.participants {
			if sender, ok := other.senders[key]; ok {
				if err := other.pc.RemoveTrack(sender); err != nil {
					r.logger.Warn().Err(err).Str("room_id", room.id).Str("participant_id", other.id).Msg("failed to remove track from participant")
				}
				delete(other.senders, key)
			}
		}
		delete(room.tracks, key)
	}
}

func (r *RoomEngine) handleRemoteTrack(roomID, participantID string, track *webrtc.TrackRemote) {
	relayTrackID := fmt.Sprintf("participant-%s-%s-%s", participantID, track.Kind().String(), sanitizeFilePart(track.ID()))
	relayStreamID := fmt.Sprintf("participant-%s", participantID)
	localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, relayTrackID, relayStreamID)
	if err != nil {
		r.logger.Warn().Err(err).Str("room_id", roomID).Msg("failed to create local relay track")
		return
	}

	rid := track.RID()
	key := fmt.Sprintf("%s:%s:%s", participantID, track.Kind().String(), track.ID())
	if rid != "" {
		key = fmt.Sprintf("%s:%s:%s:%s", participantID, track.Kind().String(), track.ID(), rid)
	}

	ssrc := uint32(track.SSRC())

	r.mu.Lock()
	room := r.ensureRoomLocked(roomID)
	published := &publishedTrack{
		key:           key,
		participantID: participantID,
		track:         track,
		local:         localTrack,
		kind:          track.Kind(),
		ssrc:          ssrc,
	}
	room.tracks[key] = published
	subscribers := 0
	for otherID, other := range room.participants {
		if otherID == participantID {
			continue
		}
		if r.attachTrackLocked(room, other, published) {
			subscribers++
		}
	}
	r.mu.Unlock()

	// Subscribers that were already in the room joined mid-stream and need an
	// IDR frame before they can render this track.
	if subscribers > 0 && track.Kind() == webrtc.RTPCodecTypeVideo {
		r.requestKeyframeForTrack(roomID, key)
	}

	for {
		packet, _, readErr := track.ReadRTP()
		if readErr != nil {
			break
		}
		if writeErr := localTrack.WriteRTP(packet); writeErr != nil {
			r.logger.Debug().Err(writeErr).Str("room_id", roomID).Msg("failed to forward room rtp packet")
			continue
		}
		r.writeRecordingPacket(roomID, participantID, track, packet)
	}

	r.mu.Lock()
	if roomRef, ok := r.rooms[roomID]; ok {
		delete(roomRef.tracks, key)
		for _, other := range roomRef.participants {
			if sender, ok := other.senders[key]; ok {
				if err := other.pc.RemoveTrack(sender); err != nil {
					r.logger.Warn().Err(err).Str("room_id", roomID).Str("participant_id", other.id).Msg("failed to remove track from participant")
				}
				delete(other.senders, key)
			}
		}
	}
	r.mu.Unlock()
	r.keyframes.Forget(ssrc)
}

func (r *RoomEngine) writeRecordingPacket(roomID, participantID string, track *webrtc.TrackRemote, packet *rtp.Packet) {
	if track.Kind() != webrtc.RTPCodecTypeAudio {
		return
	}

	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok || room.recording == nil {
		r.mu.Unlock()
		return
	}
	recording := room.recording
	r.mu.Unlock()

	recording.mu.Lock()
	defer recording.mu.Unlock()

	trackKey := fmt.Sprintf("%s:%s", participantID, track.ID())
	artifact, ok := recording.artifacts[trackKey]
	if !ok {
		ext := ".audio"
		contentType := "audio/octet-stream"
		clockRate := track.Codec().ClockRate
		channels := track.Codec().Channels
		if channels == 0 {
			channels = 2
		}
		if strings.Contains(strings.ToLower(track.Codec().MimeType), "opus") {
			ext = ".ogg"
			contentType = "audio/ogg"
		}
		path := filepath.Join(recording.baseDir, fmt.Sprintf("participant-%s-track-%s%s", participantID, sanitizeFilePart(track.ID()), ext))
		artifact = &trackRecordingArtifact{
			path:        path,
			contentType: contentType,
			startedAt:   time.Now(),
		}
		if ext == ".ogg" {
			writer, err := oggwriter.New(path, track.Codec().ClockRate, channels)
			if err != nil {
				r.logger.Warn().Err(err).Str("room_id", roomID).Msg("failed to create ogg writer")
				return
			}
			artifact.writer = writer
		} else {
			clockRate = 0
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				r.logger.Warn().Err(err).Str("room_id", roomID).Msg("failed to create raw recording file")
				return
			}
		}
		recording.artifacts[trackKey] = artifact
		_ = clockRate
	}

	if artifact.writer != nil {
		if err := artifact.writer.WriteRTP(packet); err != nil {
			r.logger.Debug().Err(err).Str("room_id", roomID).Msg("failed to write recording packet")
		}
	}
}

func sanitizeFilePart(input string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "-")
	return replacer.Replace(strings.TrimSpace(input))
}

func stringPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
