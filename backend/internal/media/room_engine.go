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
)

type RecordingArtifact struct {
	ObjectKey       string
	ContentType     string
	DurationSeconds int64
	MetadataJSON    string
}

type RoomEngine struct {
	logger        zerolog.Logger
	defaultConfig webrtc.Configuration

	mu    sync.Mutex
	rooms map[string]*mediaRoom
}

type mediaRoom struct {
	id           string
	participants map[string]*roomParticipant
	tracks       map[string]*publishedTrack
	recording    *roomRecording
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

func newRoomEngine(logger zerolog.Logger, cfg webrtc.Configuration) *RoomEngine {
	return &RoomEngine{
		logger:        logger.With().Str("component", "room_media_engine").Logger(),
		defaultConfig: cfg,
		rooms:         make(map[string]*mediaRoom),
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
	for key, track := range room.tracks {
		if track.participantID == participantID {
			continue
		}
		if _, ok := participant.senders[key]; ok {
			continue
		}
		sender, addErr := participant.pc.AddTrack(track.local)
		if addErr != nil {
			r.logger.Warn().Err(addErr).Str("room_id", roomID).Str("participant_id", participantID).Msg("failed to add published track to participant")
			continue
		}
		participant.senders[key] = sender
	}
	r.mu.Unlock()

	if err := participant.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}); err != nil {
		return "", err
	}

	answer, err := participant.pc.CreateAnswer(nil)
	if err != nil {
		return "", err
	}
	gatherComplete := webrtc.GatheringCompletePromise(participant.pc)
	if err := participant.pc.SetLocalDescription(answer); err != nil {
		return "", err
	}
	<-gatherComplete

	local := participant.pc.LocalDescription()
	if local == nil {
		return "", fmt.Errorf("local description not available")
	}

	return local.SDP, nil
}

func (r *RoomEngine) AddICECandidate(roomID, participantID string, candidate webrtc.ICECandidateInit) error {
	r.mu.Lock()
	room, ok := r.rooms[roomID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("room not found")
	}
	participant, ok := room.participants[participantID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("participant not found")
	}
	return participant.pc.AddICECandidate(candidate)
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
			_ = artifact.writer.Close()
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
	}
	r.rooms[roomID] = room
	return room
}

func (r *RoomEngine) ensureParticipantLocked(room *mediaRoom, participantID string) (*roomParticipant, error) {
	if participant, ok := room.participants[participantID]; ok {
		return participant, nil
	}

	pc, err := webrtc.NewPeerConnection(r.defaultConfig)
	if err != nil {
		return nil, err
	}

	participant := &roomParticipant{
		id:      participantID,
		pc:      pc,
		senders: make(map[string]*webrtc.RTPSender),
	}

	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
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

func (r *RoomEngine) removeParticipantLocked(room *mediaRoom, participantID string) {
	participant, ok := room.participants[participantID]
	if !ok {
		return
	}
	_ = participant.pc.Close()
	delete(room.participants, participantID)

	for key, track := range room.tracks {
		if track.participantID != participantID {
			continue
		}
		for _, other := range room.participants {
			if sender, ok := other.senders[key]; ok {
				_ = other.pc.RemoveTrack(sender)
				delete(other.senders, key)
			}
		}
		delete(room.tracks, key)
	}
}

func (r *RoomEngine) handleRemoteTrack(roomID, participantID string, track *webrtc.TrackRemote) {
	localTrack, err := webrtc.NewTrackLocalStaticRTP(track.Codec().RTPCodecCapability, track.ID(), track.StreamID())
	if err != nil {
		r.logger.Warn().Err(err).Str("room_id", roomID).Msg("failed to create local relay track")
		return
	}

	key := fmt.Sprintf("%s:%s:%s", participantID, track.Kind().String(), track.ID())

	r.mu.Lock()
	room := r.ensureRoomLocked(roomID)
	room.tracks[key] = &publishedTrack{
		key:           key,
		participantID: participantID,
		track:         track,
		local:         localTrack,
	}
	for otherID, other := range room.participants {
		if otherID == participantID {
			continue
		}
		if _, ok := other.senders[key]; ok {
			continue
		}
		sender, addErr := other.pc.AddTrack(localTrack)
		if addErr != nil {
			r.logger.Warn().Err(addErr).Str("room_id", roomID).Str("participant_id", otherID).Msg("failed to attach relay track to participant")
			continue
		}
		other.senders[key] = sender
	}
	r.mu.Unlock()

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
				_ = other.pc.RemoveTrack(sender)
				delete(other.senders, key)
			}
		}
	}
	r.mu.Unlock()
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
