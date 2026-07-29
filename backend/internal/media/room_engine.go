package media

import (
	"context"
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

	"github.com/allcallall/backend/internal/storage"
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
	api           *webrtc.API

	// recordingUploader 为可选的对象存储客户端（复用 storage.RecordingStorage）。
	// 仅在底层为 S3 时，StopRecording 才会将本地录制文件异步上传，未配置则保持纯本地盘行为。
	recordingUploader storage.RecordingStorage

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

func newRoomEngine(logger zerolog.Logger, cfg webrtc.Configuration, api *webrtc.API, uploader storage.RecordingStorage) *RoomEngine {
	return &RoomEngine{
		logger:            logger.With().Str("component", "room_engine").Logger(),
		defaultConfig:     cfg,
		api:               api,
		recordingUploader: uploader,
		rooms:             make(map[string]*mediaRoom),
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

	// Inject Opus DTX (Discontinuous Transmission) to save bandwidth during silence
	// This is a commercial-grade optimization (Pillar A)
	finalSDP := participant.pc.LocalDescription().SDP
	finalSDP = strings.ReplaceAll(finalSDP, "useinbandfec=1", "useinbandfec=1;usedtx=1")

	return finalSDP, nil
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

	// 最小安全增量：若配置了 S3 对象存储，将已关闭的本地录制文件异步上传到对象存储，
	// 避免多副本部署下录制文件随 Pod 销毁丢失。上传失败仅告警，绝不阻断会话结束流程；
	// 未配置 S3（默认本地盘）时此处完全不执行，行为不变。
	if r.recordingUploader != nil && r.recordingUploader.Driver() == storage.DriverS3 {
		baseDir := recording.baseDir
		r.uploadRecordingsAsync(roomID, baseDir, artifacts)
	}
	return artifacts, nil
}

// uploadRecordingsAsync 在后台将本地录制文件复制到对象存储。任何错误只记录告警，不影响主流程。
func (r *RoomEngine) uploadRecordingsAsync(roomID, baseDir string, artifacts []RecordingArtifact) {
	uploader := r.recordingUploader
	go func() {
		ctx := context.Background()
		for _, artifact := range artifacts {
			src := strings.TrimSpace(artifact.ObjectKey)
			if src == "" {
				continue
			}
			if info, err := os.Stat(src); err != nil || info.IsDir() {
				r.logger.Warn().Str("path", src).Msg("recording upload skipped: local file not found")
				continue
			}
			// 用相对 baseDir 的路径作为对象键，保留目录结构；回退到文件名。
			key := filepath.ToSlash(filepath.Join("recordings", roomID, filepath.Base(src)))
			if rel, err := filepath.Rel(baseDir, src); err == nil && strings.TrimSpace(rel) != "" && !strings.HasPrefix(rel, "..") {
				key = filepath.ToSlash(filepath.Join("recordings", roomID, rel))
			}
			if _, err := uploader.SaveFile(ctx, src, key, artifact.ContentType); err != nil {
				r.logger.Warn().Err(err).Str("room_id", roomID).Str("key", key).Msg("failed to upload recording to object storage")
				continue
			}
			r.logger.Info().Str("room_id", roomID).Str("key", key).Msg("recording uploaded to object storage")
		}
	}()
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
	if err := participant.pc.Close(); err != nil {
		r.logger.Warn().Err(err).Str("participant_id", participantID).Msg("failed to close peer connection on participant removal")
	}
	delete(room.participants, participantID)

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
				if err := other.pc.RemoveTrack(sender); err != nil {
					r.logger.Warn().Err(err).Str("room_id", roomID).Str("participant_id", other.id).Msg("failed to remove track from participant")
				}
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
